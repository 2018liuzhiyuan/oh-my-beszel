package hub

import (
	"context"
	"errors"
	"fmt"
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
	app         core.App
	ctx         context.Context
	cancel      context.CancelFunc
	deploy      func(context.Context, agentDeploymentTarget) error
	publicKey   func() string
	retryDelays []time.Duration
	mu          sync.Mutex
	running     map[string]struct{}
	wg          sync.WaitGroup
}

const agentDeploymentAttemptTimeout = 2 * time.Minute

func newAgentDeploymentManager(app core.App, publicKey func() string) *agentDeploymentManager {
	ctx, cancel := context.WithCancel(context.Background())
	manager := &agentDeploymentManager{
		app:         app,
		ctx:         ctx,
		cancel:      cancel,
		publicKey:   publicKey,
		retryDelays: []time.Duration{time.Minute, 5 * time.Minute, 30 * time.Minute},
		running:     make(map[string]struct{}),
	}
	manager.deploy = func(ctx context.Context, target agentDeploymentTarget) error {
		return deployAgentOverSSH(ctx, app, target)
	}
	return manager
}

func newAgentDeploymentManagerForTest(deploy func(context.Context, agentDeploymentTarget) error) *agentDeploymentManager {
	ctx, cancel := context.WithCancel(context.Background())
	return &agentDeploymentManager{
		ctx:     ctx,
		cancel:  cancel,
		deploy:  deploy,
		running: make(map[string]struct{}),
	}
}

func (manager *agentDeploymentManager) bind() {
	manager.app.OnRecordAfterCreateSuccess("systems").BindFunc(manager.onSystemChanged)
	manager.app.OnRecordAfterUpdateSuccess("systems").BindFunc(manager.onSystemChanged)
	manager.app.OnTerminate().BindFunc(func(event *core.TerminateEvent) error {
		manager.stop()
		return event.Next()
	})
}

func (manager *agentDeploymentManager) onSystemChanged(event *core.RecordEvent) error {
	if !shouldScheduleAgentDeployment(event.Record.GetString("status"), event.Record.GetString("ssh_config")) {
		return event.Next()
	}
	target, err := manager.targetFromRecord(event.Record)
	if err != nil {
		event.App.Logger().Warn("Agent auto-deploy skipped", "system", event.Record.Id, "err", err)
		return event.Next()
	}
	manager.schedule(target)
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
			manager.app.Logger().Warn("Agent auto-deploy skipped", "system", record.Id, "err", err)
			continue
		}
		manager.schedule(target)
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

func (manager *agentDeploymentManager) schedule(target agentDeploymentTarget) {
	manager.mu.Lock()
	if _, exists := manager.running[target.id]; exists || manager.ctx.Err() != nil {
		manager.mu.Unlock()
		return
	}
	manager.running[target.id] = struct{}{}
	manager.wg.Add(1)
	manager.mu.Unlock()

	go manager.run(target)
}

func (manager *agentDeploymentManager) run(target agentDeploymentTarget) {
	defer manager.wg.Done()
	defer func() {
		manager.mu.Lock()
		delete(manager.running, target.id)
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
		if attempt >= len(manager.retryDelays) || !waitForAgentDeployment(manager.ctx, manager.retryDelays[attempt]) {
			return
		}
	}
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

func waitForAgentDeployment(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func (manager *agentDeploymentManager) stop() {
	manager.cancel()
	manager.wg.Wait()
}
