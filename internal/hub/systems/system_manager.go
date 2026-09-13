package systems

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/henrygd/beszel/internal/hub/ws"

	"github.com/henrygd/beszel/internal/entities/system"
	"github.com/henrygd/beszel/internal/hub/expirymap"

	"github.com/henrygd/beszel/internal/common"

	"github.com/henrygd/beszel/internal/beszel"

	"github.com/blang/semver"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/store"
	"golang.org/x/crypto/ssh"
)

// System status constants
const (
	up      string = "up"      // System is online and responding
	down    string = "down"    // System is offline or not responding
	paused  string = "paused"  // System monitoring is paused
	pending string = "pending" // System is waiting on initial connection result

	// interval is the default update interval in milliseconds (60 seconds)
	interval int = 60_000
	// interval int = 10_000 // Debug interval for faster updates

	// sessionTimeout is the maximum time to wait for SSH connections
	sessionTimeout = 4 * time.Second
)

// errSystemExists is returned when attempting to add a system that already exists
var errSystemExists = errors.New("system exists")

// SystemManager manages a collection of monitored systems and their connections.
// It handles system lifecycle, status updates, and maintains both SSH and WebSocket connections.
type SystemManager struct {
	hub           hubLike                               // Hub interface for database and alert operations
	systems       *store.Store[string, *System]         // Thread-safe store of active systems
	sshConfig     *ssh.ClientConfig                     // SSH client configuration for system connections
	smartFetchMap *expirymap.ExpiryMap[smartFetchState] // Stores last SMART fetch time/result; TTL is only for cleanup
	ctx           context.Context
	cancel        context.CancelFunc
	samplerStop   chan struct{} // Stops the GpuMemoryFree sampler goroutine
	samplerOnce   sync.Once
	lifecycleMu   sync.Mutex
	updaterCount  int
	updatersDone  <-chan struct{}
	updatersClose chan struct{}
	stopped       bool
}

// hubLike defines the interface requirements for the hub dependency.
// It extends core.App with system-specific functionality.
type hubLike interface {
	core.App
	GetSSHKey(dataDir string) (ssh.Signer, error)
	HandleSystemAlerts(systemRecord *core.Record, data *system.CombinedData) error
	HandleStatusAlerts(status string, systemRecord *core.Record) error
	CancelPendingStatusAlerts(systemID string)
	SampleGpuFreeAlerts(systemID string, data *system.CombinedData)
}

// NewSystemManager creates a new SystemManager instance with the provided hub.
// The hub must implement the hubLike interface to provide database and alert functionality.
func NewSystemManager(hub hubLike) *SystemManager {
	updatersDone := make(chan struct{})
	close(updatersDone)
	sm := &SystemManager{
		systems:       store.New(map[string]*System{}),
		hub:           hub,
		smartFetchMap: expirymap.New[smartFetchState](time.Hour),
		samplerStop:   make(chan struct{}),
		updatersDone:  updatersDone,
	}
	sm.ctx, sm.cancel = context.WithCancel(context.Background())
	go sm.sampleGpuFree()
	return sm
}

// sampleGpuFree feeds every online system's latest data to the GpuMemoryFree
// sampler once per second; the sampler itself paces samples evenly.
func (sm *SystemManager) sampleGpuFree() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-sm.samplerStop:
			return
		case <-ticker.C:
			for _, sys := range sm.systems.Values() {
				if sys.statusValue() != up {
					continue
				}
				sm.hub.SampleGpuFreeAlerts(sys.Id, sys.dataSnapshot())
			}
		}
	}
}

// StopSampler stops the GpuMemoryFree sampling goroutine (used by tests).
func (sm *SystemManager) StopSampler() {
	sm.samplerOnce.Do(func() { close(sm.samplerStop) })
}

// GetSystem returns a system by ID from the store
func (sm *SystemManager) GetSystem(systemID string) (*System, error) {
	sys, ok := sm.systems.GetOk(systemID)
	if !ok {
		return nil, fmt.Errorf("system not found")
	}
	return sys, nil
}

// Initialize sets up the system manager by binding event hooks and starting existing systems.
// It configures SSH client settings and begins monitoring all non-paused systems from the database.
// Systems are started with staggered delays to prevent overwhelming the hub during startup.
func (sm *SystemManager) Initialize() error {
	sm.bindEventHooks()

	// Initialize SSH client configuration
	err := sm.createSSHClientConfig()
	if err != nil {
		return err
	}

	// Load existing systems from database (excluding paused ones)
	systems, err := loadSystemsForStartup(sm.hub.DB())
	if err != nil || len(systems) == 0 {
		return err
	}

	// Start systems in background with staggered timing
	go func() {
		// Calculate staggered delay between system starts (max 2 seconds per system)
		delta := interval / max(1, len(systems))
		delta = min(delta, 2_000)
		sleepTime := time.Duration(delta) * time.Millisecond

		for _, system := range systems {
			if !waitForContext(sm.ctx, sleepTime) {
				return
			}
			_ = sm.AddSystem(system)
		}
	}()
	return nil
}

// loadSystemsForStartup returns the systems that should be monitored after a
// hub start. ssh_config must be selected so systems with a custom SSH config
// keep dialing through `ssh -F` after a restart instead of silently falling
// back to direct connections.
func loadSystemsForStartup(db dbx.Builder) ([]*System, error) {
	var systems []*System
	err := db.NewQuery("SELECT id, host, port, ssh_config, status FROM systems WHERE status != 'paused'").All(&systems)
	return systems, err
}

// bindEventHooks registers event handlers for system and fingerprint record changes.
// These hooks ensure the system manager stays synchronized with database changes.
func (sm *SystemManager) bindEventHooks() {
	sm.hub.OnRecordCreate("systems").BindFunc(sm.onRecordCreate)
	sm.hub.OnRecordAfterCreateSuccess("systems").BindFunc(sm.onRecordAfterCreateSuccess)
	sm.hub.OnRecordUpdate("systems").BindFunc(sm.onRecordUpdate)
	sm.hub.OnRecordAfterUpdateSuccess("systems").BindFunc(sm.onRecordAfterUpdateSuccess)
	sm.hub.OnRecordAfterDeleteSuccess("systems").BindFunc(sm.onRecordAfterDeleteSuccess)
	sm.hub.OnRecordAfterUpdateSuccess("fingerprints").BindFunc(sm.onTokenRotated)
	sm.hub.OnRealtimeSubscribeRequest().BindFunc(sm.onRealtimeSubscribeRequest)
	sm.hub.OnRealtimeConnectRequest().BindFunc(sm.onRealtimeConnectRequest)
	sm.hub.OnTerminate().BindFunc(sm.onTerminate)
}

// onTerminate cancels SystemManager context on app shutdown
func (sm *SystemManager) onTerminate(e *core.TerminateEvent) error {
	sm.lifecycleMu.Lock()
	sm.stopped = true
	sm.cancel()
	done := sm.updatersDone
	sm.lifecycleMu.Unlock()
	<-done
	sm.StopSampler()
	return e.Next()
}

// onTokenRotated handles fingerprint token rotation events.
// When a system's authentication token is rotated, any existing WebSocket connection
// must be closed to force re-authentication with the new token.
func (sm *SystemManager) onTokenRotated(e *core.RecordEvent) error {
	systemID := e.Record.GetString("system")
	system, ok := sm.systems.GetOk(systemID)
	if !ok {
		return e.Next()
	}
	// No need to close connection if not connected via websocket
	if system.WsConn == nil {
		return e.Next()
	}
	system.setDown(nil)
	sm.RemoveSystem(systemID)
	return e.Next()
}

// onRecordCreate is called before a new system record is committed to the database.
// info must always default to a JSON object (a null breaks the dashboard's
// derived-field lookups); status keeps any caller-provided value so API
// created paused or seeded systems are not force-fed into polling.
func (sm *SystemManager) onRecordCreate(e *core.RecordEvent) error {
	e.Record.Set("info", system.Info{})
	if e.Record.GetString("status") == "" {
		e.Record.Set("status", pending)
	}
	return e.Next()
}

// onRecordAfterCreateSuccess is called after a new system record is successfully created.
// It adds the new system to the manager to begin monitoring.
func (sm *SystemManager) onRecordAfterCreateSuccess(e *core.RecordEvent) error {
	// paused systems are not monitored (mirrors the startup query in Initialize);
	// resuming flips the status to pending, which re-adds them via onRecordAfterUpdateSuccess
	if e.Record.GetString("status") == paused {
		return e.Next()
	}
	if err := sm.AddRecord(e.Record, nil); err != nil {
		e.App.Logger().Error("Error adding record", "err", err)
	}
	return e.Next()
}

// onRecordUpdate is called before a system record is updated in the database.
// It clears system info when the status is changed to paused.
func (sm *SystemManager) onRecordUpdate(e *core.RecordEvent) error {
	if e.Record.GetString("status") == paused {
		e.Record.Set("info", system.Info{})
		e.Record.Set("status_info", "")
	}
	return e.Next()
}

// onRecordAfterUpdateSuccess handles system record updates after they're committed to the database.
// It manages system lifecycle based on status changes and triggers appropriate alerts.
// Status transitions are handled as follows:
// - paused: Closes SSH connection and deactivates alerts
// - pending: Starts monitoring (reuses WebSocket if available)
// - up: Triggers system alerts
// - down: Triggers status change alerts
func (sm *SystemManager) onRecordAfterUpdateSuccess(e *core.RecordEvent) error {
	newStatus := e.Record.GetString("status")
	prevStatus := pending
	system, ok := sm.systems.GetOk(e.Record.Id)
	if ok {
		prevStatus = system.statusValue()
		system.setStatus(newStatus)
	}

	switch newStatus {
	case paused:
		if ok {
			// Pause monitoring but keep system in manager for potential resume
			system.closeSSHConnection()
		}
		_ = deactivateAlerts(e.App, e.Record.Id)
		sm.hub.CancelPendingStatusAlerts(e.Record.Id)
		return e.Next()
	case pending:
		// Resume monitoring, preferring existing WebSocket connection
		if ok && system.WsConn != nil {
			go system.update()
			return e.Next()
		}
		// Start new monitoring session
		if err := sm.AddRecord(e.Record, nil); err != nil {
			e.App.Logger().Error("Error adding record", "err", err)
		}
		_ = deactivateAlerts(e.App, e.Record.Id)
		return e.Next()
	}

	// Handle systems not in manager
	if !ok {
		return sm.AddRecord(e.Record, nil)
	}

	// Trigger system alerts when system comes online
	if newStatus == up {
		if err := sm.hub.HandleSystemAlerts(e.Record, system.dataSnapshot()); err != nil {
			e.App.Logger().Error("Error handling system alerts", "err", err)
		}
	}

	// Trigger status change alerts for up/down transitions
	if (newStatus == down && prevStatus == up) || (newStatus == up && prevStatus == down) {
		if err := sm.hub.HandleStatusAlerts(newStatus, e.Record); err != nil {
			e.App.Logger().Error("Error handling status alerts", "err", err)
		}
	}
	return e.Next()
}

// onRecordAfterDeleteSuccess is called after a system record is successfully deleted.
// It removes the system from the manager and cleans up all associated resources.
func (sm *SystemManager) onRecordAfterDeleteSuccess(e *core.RecordEvent) error {
	sm.RemoveSystem(e.Record.Id)
	return e.Next()
}

// AddSystem adds a system to the manager and starts monitoring it.
// It validates required fields, initializes the system context, and starts the update goroutine.
// Returns error if a system with the same ID already exists.
func (sm *SystemManager) AddSystem(sys *System) error {
	sm.lifecycleMu.Lock()
	defer sm.lifecycleMu.Unlock()
	if sm.stopped || sm.ctx.Err() != nil {
		return errors.New("system manager stopped")
	}
	if sm.systems.Has(sys.Id) {
		return errSystemExists
	}
	if sys.Id == "" || sys.Host == "" {
		return errors.New("system missing required fields")
	}

	// Initialize system for monitoring
	sys.manager = sm
	sys.ctx, sys.cancel = sys.getContext(sm.ctx)
	sys.setData(&system.CombinedData{})
	sm.systems.Set(sys.Id, sys)

	// Start monitoring in background
	if sm.updaterCount == 0 {
		sm.updatersClose = make(chan struct{})
		sm.updatersDone = sm.updatersClose
	}
	sm.updaterCount++
	done := make(chan struct{})
	sys.setUpdaterDone(done)
	go func() {
		defer sm.finishUpdater()
		defer close(done)
		sys.StartUpdater()
	}()
	return nil
}

func (sm *SystemManager) finishUpdater() {
	sm.lifecycleMu.Lock()
	sm.updaterCount--
	if sm.updaterCount == 0 {
		close(sm.updatersClose)
		sm.updatersClose = nil
	}
	sm.lifecycleMu.Unlock()
}

// RemoveSystem removes a system from the manager and cleans up all associated resources.
// It cancels the system's context, closes all connections, and removes it from the store.
// Returns an error if the system is not found.
func (sm *SystemManager) RemoveSystem(systemID string) error {
	system, ok := sm.systems.GetOk(systemID)
	if !ok {
		return errors.New("system not found")
	}

	// Stop the update goroutine
	if system.cancel != nil {
		system.cancel()
	}

	// Clean up all connections
	system.closeSSHConnection()
	system.closeWebSocketConnection()
	sm.systems.Remove(systemID)
	return nil
}

// AddRecord creates a System instance from a database record and adds it to the manager.
// If a system with the same ID already exists, it's removed first to ensure clean state.
// If no system instance is provided, a new one is created.
// This method is typically called when systems are created or their status changes to pending.
func (sm *SystemManager) AddRecord(record *core.Record, system *System) (err error) {
	// Remove existing system to ensure clean state
	if sm.systems.Has(record.Id) {
		_ = sm.RemoveSystem(record.Id)
	}

	// Create new system if none provided
	if system == nil {
		system = sm.NewSystem(record.Id)
	}

	// Populate system from record
	system.Status = record.GetString("status")
	system.Host = record.GetString("host")
	system.Port = record.GetString("port")
	system.SSHConfigPath = record.GetString("ssh_config")

	return sm.AddSystem(system)
}

// AddWebSocketSystem creates and adds a system with an established WebSocket connection.
// This method is called when an agent connects via WebSocket with valid authentication.
// The system is immediately added to monitoring with the provided connection and version info.
func (sm *SystemManager) AddWebSocketSystem(systemId string, agentVersion semver.Version, wsConn *ws.WsConn) error {
	systemRecord, err := sm.hub.FindRecordById("systems", systemId)
	if err != nil {
		return err
	}
	sm.resetFailedSmartFetchState(systemId)

	system := sm.NewSystem(systemId)
	system.WsConn = wsConn
	system.agentVersion = agentVersion

	if err := sm.AddRecord(systemRecord, system); err != nil {
		return err
	}
	return nil
}

// resetFailedSmartFetchState clears only failed SMART cooldown entries so a fresh
// agent reconnect retries SMART discovery immediately after configuration changes.
func (sm *SystemManager) resetFailedSmartFetchState(systemID string) {
	state, ok := sm.smartFetchMap.GetOk(systemID)
	if ok && !state.Successful {
		sm.smartFetchMap.Remove(systemID)
	}
}

// createSSHClientConfig initializes the SSH client configuration for connecting to an agent's server
func (sm *SystemManager) createSSHClientConfig() error {
	privateKey, err := sm.hub.GetSSHKey("")
	if err != nil {
		return err
	}

	sm.sshConfig = &ssh.ClientConfig{
		User: "u",
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(privateKey),
		},
		Config: ssh.Config{
			Ciphers:      common.DefaultCiphers,
			KeyExchanges: common.DefaultKeyExchanges,
			MACs:         common.DefaultMACs,
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		ClientVersion:   fmt.Sprintf("SSH-2.0-%s_%s", beszel.AppName, beszel.Version),
		Timeout:         sessionTimeout,
	}
	return nil
}

// deactivateAlerts finds all triggered alerts for a system and sets them to inactive.
// This is called when a system is paused or goes offline to prevent continued alerts.
func deactivateAlerts(app core.App, systemID string) error {
	// Note: Direct SQL updates don't trigger SSE, so we use the PocketBase API
	// _, err := app.DB().NewQuery(fmt.Sprintf("UPDATE alerts SET triggered = false WHERE system = '%s'", systemID)).Execute()

	alerts, err := app.FindRecordsByFilter("alerts", fmt.Sprintf("system = '%s' && triggered = 1", systemID), "", -1, 0)
	if err != nil {
		return err
	}

	for _, alert := range alerts {
		alert.Set("triggered", false)
		if err := app.SaveNoValidate(alert); err != nil {
			return err
		}
	}
	return nil
}

// waitForContext waits for delay or returns early when ctx is cancelled.
func waitForContext(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
