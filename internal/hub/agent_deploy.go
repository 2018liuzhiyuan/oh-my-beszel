package hub

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"golang.org/x/crypto/ssh"
)

type agentDeploymentTarget struct {
	id        string
	host      string
	port      uint16
	publicKey string
	sshConfig string
}

type agentDeploymentManager struct {
	app          core.App
	ctx          context.Context
	cancel       context.CancelFunc
	deploy       func(context.Context, agentDeploymentTarget) error
	publicKey    func() string
	retryDelays  []time.Duration
	mu           sync.Mutex
	running      map[string]chan struct{} // per-system wake channel, present while a run is active
	lastFinished map[string]time.Time
	wg           sync.WaitGroup
}

const (
	agentDeploymentAttemptTimeout = 2 * time.Minute

	// agentDeployRescheduleCooldown throttles re-scheduling from record update
	// events. Without it, the down -> deploy -> pending -> down flap cycles a
	// new deployment run on every failed poll (observed every ~13s).
	agentDeployRescheduleCooldown = 2 * time.Minute
)

func newAgentDeploymentManager(app core.App, publicKey func() string) *agentDeploymentManager {
	ctx, cancel := context.WithCancel(context.Background())
	manager := &agentDeploymentManager{
		app:          app,
		ctx:          ctx,
		cancel:       cancel,
		publicKey:    publicKey,
		retryDelays:  []time.Duration{time.Minute, 5 * time.Minute, 30 * time.Minute},
		running:      make(map[string]chan struct{}),
		lastFinished: make(map[string]time.Time),
	}
	manager.deploy = func(ctx context.Context, target agentDeploymentTarget) error {
		return deployAgentOverSSH(ctx, app, target)
	}
	return manager
}

func newAgentDeploymentManagerForTest(deploy func(context.Context, agentDeploymentTarget) error) *agentDeploymentManager {
	ctx, cancel := context.WithCancel(context.Background())
	return &agentDeploymentManager{
		ctx:          ctx,
		cancel:       cancel,
		deploy:       deploy,
		running:      make(map[string]chan struct{}),
		lastFinished: make(map[string]time.Time),
	}
}

func (manager *agentDeploymentManager) bind() {
	manager.app.OnRecordAfterCreateSuccess("systems").BindFunc(func(event *core.RecordEvent) error {
		return manager.onSystemChanged(event, true)
	})
	manager.app.OnRecordAfterUpdateSuccess("systems").BindFunc(func(event *core.RecordEvent) error {
		return manager.onSystemChanged(event, false)
	})
	manager.app.OnTerminate().BindFunc(func(event *core.TerminateEvent) error {
		manager.stop()
		return event.Next()
	})
}

func (manager *agentDeploymentManager) onSystemChanged(event *core.RecordEvent, created bool) error {
	if !shouldScheduleAgentDeployment(event.Record.GetString("status"), event.Record.GetString("ssh_config")) {
		return event.Next()
	}
	target, err := manager.targetFromRecord(event.Record)
	if err != nil {
		event.App.Logger().Warn("Agent auto-deploy skipped", "system", event.Record.Id, "host", event.Record.GetString("host"), "err", err)
		return event.Next()
	}
	manager.schedule(target, created)
	return event.Next()
}

func (manager *agentDeploymentManager) startExisting() error {
	records, err := manager.app.FindAllRecords("systems")
	if err != nil {
		return fmt.Errorf("find systems for agent auto-deploy: %w", err)
	}
	for _, record := range records {
		if !shouldScheduleAgentDeployment(record.GetString("status"), record.GetString("ssh_config")) {
			continue
		}
		target, err := manager.targetFromRecord(record)
		if err != nil {
			manager.app.Logger().Warn("Agent auto-deploy skipped", "system", record.Id, "host", record.GetString("host"), "err", err)
			continue
		}
		manager.schedule(target, true)
	}
	return nil
}

func shouldScheduleAgentDeployment(status, sshConfig string) bool {
	return strings.TrimSpace(sshConfig) != "" && (status == "pending" || status == "down")
}

func (manager *agentDeploymentManager) targetFromRecord(record *core.Record) (agentDeploymentTarget, error) {
	port, err := strconv.ParseUint(record.GetString("port"), 10, 16)
	if err != nil || port == 0 {
		return agentDeploymentTarget{}, fmt.Errorf("invalid agent port")
	}
	host := strings.TrimSpace(record.GetString("host"))
	sshConfig := strings.TrimSpace(record.GetString("ssh_config"))
	if host == "" || strings.HasPrefix(host, "-") || strings.ContainsAny(host, "\r\n\x00") {
		return agentDeploymentTarget{}, fmt.Errorf("invalid SSH host")
	}
	configured, err := readSSHHosts(sshConfig)
	if err != nil {
		return agentDeploymentTarget{}, fmt.Errorf("read SSH config: %w", err)
	}
	found := false
	for _, candidate := range configured {
		if strings.EqualFold(candidate.Name, host) {
			found = true
			break
		}
	}
	if !found {
		return agentDeploymentTarget{}, fmt.Errorf("SSH host is not a concrete entry in the selected config")
	}
	if manager.publicKey == nil {
		return agentDeploymentTarget{}, fmt.Errorf("Hub public key is unavailable")
	}
	key, _, _, _, err := ssh.ParseAuthorizedKey([]byte(manager.publicKey()))
	if err != nil {
		return agentDeploymentTarget{}, fmt.Errorf("parse agent public key: %w", err)
	}
	publicKey := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(key)))
	return agentDeploymentTarget{
		id:        record.Id,
		host:      host,
		port:      uint16(port),
		publicKey: publicKey,
		sshConfig: sshConfig,
	}, nil
}

// schedule starts a deployment run for the target unless one is already
// active. When a run is active, an immediate request (record creation or hub
// startup) wakes it so a fresh attempt happens without waiting out the current
// backoff sleep. Non-immediate requests (poll-failure status updates) are
// throttled by agentDeployRescheduleCooldown once the previous run finished.
func (manager *agentDeploymentManager) schedule(target agentDeploymentTarget, immediate bool) {
	manager.mu.Lock()
	if manager.ctx.Err() != nil {
		manager.mu.Unlock()
		return
	}
	if wake, exists := manager.running[target.id]; exists {
		if immediate {
			select {
			case wake <- struct{}{}:
			default:
			}
		}
		manager.mu.Unlock()
		return
	}
	if !immediate {
		if last, ok := manager.lastFinished[target.id]; ok && time.Since(last) < agentDeployRescheduleCooldown {
			manager.mu.Unlock()
			return
		}
	}
	wake := make(chan struct{}, 1)
	manager.running[target.id] = wake
	manager.wg.Add(1)
	manager.mu.Unlock()

	go manager.run(target, wake)
}

func (manager *agentDeploymentManager) run(target agentDeploymentTarget, wake <-chan struct{}) {
	defer manager.wg.Done()
	defer func() {
		manager.mu.Lock()
		delete(manager.running, target.id)
		manager.lastFinished[target.id] = time.Now()
		manager.mu.Unlock()
	}()

	for attempt := 0; ; attempt++ {
		if manager.app != nil && attempt > 0 {
			record, err := manager.app.FindRecordById("systems", target.id)
			if err != nil || !shouldScheduleAgentDeployment(record.GetString("status"), record.GetString("ssh_config")) {
				return
			}
			target, err = manager.targetFromRecord(record)
			if err != nil {
				manager.app.Logger().Warn("Agent auto-deploy retry skipped", "system", target.id, "err", err)
				return
			}
		}
		attemptCtx, cancel := context.WithTimeout(manager.ctx, agentDeploymentAttemptTimeout)
		err := manager.deploy(attemptCtx, target)
		cancel()
		if err == nil {
			manager.markPending(target.id)
			return
		}
		if manager.app != nil {
			manager.app.Logger().Warn("Agent auto-deploy failed", "system", target.id, "host", target.host, "err", err)
		}
		// mirror to stderr so packaged hubs capture the reason in hub.log
		slog.Warn("Agent auto-deploy failed", "system", target.id, "host", target.host, "err", err)
		delay, ok := manager.retryDelay(attempt)
		if !ok || !waitForAgentDeployment(manager.ctx, delay, wake) {
			return
		}
	}
}

// retryDelay returns the backoff before the next attempt. Runs keep retrying
// at the slowest cadence for as long as the system stays pending or down, so a
// host that was unreachable at first attempt (rebooted, network flap) is
// recovered without a hub restart or manual record edit. A manager without
// configured delays (tests) performs a single attempt.
func (manager *agentDeploymentManager) retryDelay(attempt int) (time.Duration, bool) {
	if len(manager.retryDelays) == 0 {
		return 0, false
	}
	if attempt < len(manager.retryDelays) {
		return manager.retryDelays[attempt], true
	}
	return manager.retryDelays[len(manager.retryDelays)-1], true
}

func (manager *agentDeploymentManager) markPending(systemID string) {
	if manager.app == nil {
		return
	}
	record, err := manager.app.FindRecordById("systems", systemID)
	if err != nil || record.GetString("status") == "paused" {
		return
	}
	record.Set("status", "pending")
	if err := manager.app.SaveNoValidate(record); err != nil && !errors.Is(err, context.Canceled) {
		manager.app.Logger().Warn("Agent auto-deploy status update failed", "system", systemID, "err", err)
	}
}

// waitForAgentDeployment waits for delay, returning early when the context is
// cancelled or a new scheduling request wakes the run (so an edited system is
// retried immediately instead of after the current backoff sleep).
func waitForAgentDeployment(ctx context.Context, delay time.Duration, wake <-chan struct{}) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-wake:
		return true
	case <-timer.C:
		return true
	}
}

func (manager *agentDeploymentManager) stop() {
	manager.cancel()
	manager.wg.Wait()
}
