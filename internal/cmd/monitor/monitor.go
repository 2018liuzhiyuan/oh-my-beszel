//go:build windows

// Monitor is the local deployment helper for the Beszel hub. It replaces the
// previous C# launcher, task runner, and BeszelLauncher with one static binary
// and has these modes:
//
//	Monitor.exe              start tasks, wait for the dashboard, open browser
//	Monitor.exe task <ps1>   run a PowerShell script hidden (used by tasks)
//	Monitor.exe install-task [name]    register + start the logon task
//	Monitor.exe uninstall-task [name]  stop + remove the logon task
//
// The task subcommands use schtasks.exe directly instead of a .ps1 wrapper so
// they work on machines where the execution policy blocks downloaded scripts.
// Both modes read config.json next to the executable; see loadConfig for the
// supported options and their defaults.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const mutexName = `Local\BeszelMonitorLauncher`

// hubTaskName is the scheduled task that starts run-hub.ps1 at logon. The
// optional CLI argument of install-task/uninstall-task overrides it, and
// BESZEL_MONITOR_TASK_NAME lets tests exercise the real registration path
// against a scratch task instead of the production entry.
const hubTaskName = "Beszel Hub"

func autostartTaskName() string {
	if name := strings.TrimSpace(os.Getenv("BESZEL_MONITOR_TASK_NAME")); name != "" {
		return name
	}
	return hubTaskName
}

type HubConfig struct {
	UserEmail    string `json:"userEmail"`
	UserPassword string `json:"userPassword"`
	AutoLogin    string `json:"autoLogin"`
	CheckUpdates *bool  `json:"checkUpdates"`
	LogLevel     string `json:"logLevel"`
}

type Config struct {
	Host                  string    `json:"host"`
	Port                  int       `json:"port"`
	SSHConfigPath         string    `json:"sshConfigPath"`
	OpenBrowser           *bool     `json:"openBrowser"`
	StartupTimeoutSeconds int       `json:"startupTimeoutSeconds"`
	Tasks                 []string  `json:"tasks"`
	HubScript             string    `json:"hubScript"`
	Hub                   HubConfig `json:"hub"`
}

func loadConfig() (*Config, error) {
	cfg := &Config{
		Host:                  "127.0.0.1",
		Port:                  8090,
		OpenBrowser:           new(bool),
		StartupTimeoutSeconds: 45,
		HubScript:             `app\run-hub.ps1`,
	}
	*cfg.OpenBrowser = true

	exePath, err := os.Executable()
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(exePath), "config.json"))
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(raw, cfg); err != nil {
		return nil, fmt.Errorf("parse config.json: %w", err)
	}
	if cfg.Host == "" {
		cfg.Host = "127.0.0.1"
	}
	if cfg.Port <= 0 {
		cfg.Port = 8090
	}
	if cfg.OpenBrowser == nil {
		cfg.OpenBrowser = new(bool)
		*cfg.OpenBrowser = true
	}
	if cfg.StartupTimeoutSeconds <= 0 {
		cfg.StartupTimeoutSeconds = 45
	}
	if strings.TrimSpace(cfg.HubScript) == "" {
		cfg.HubScript = `app\run-hub.ps1`
	}
	return cfg, nil
}

func main() {
	if len(os.Args) >= 3 && os.Args[1] == "task" {
		os.Exit(runTask(os.Args[2]))
	}
	if len(os.Args) >= 2 && os.Args[1] == "install-task" {
		name := autostartTaskName()
		if len(os.Args) >= 3 && os.Args[2] != "" {
			name = os.Args[2]
		}
		if err := installTask(name, true); err != nil {
			reportTaskResult("Beszel autostart install failed", err.Error(), true)
			os.Exit(1)
		}
		reportTaskResult("Beszel autostart installed", fmt.Sprintf(
			"Scheduled task %q registered and started. The hub now starts at logon.", name), false)
		return
	}
	if len(os.Args) >= 2 && os.Args[1] == "uninstall-task" {
		name := autostartTaskName()
		if len(os.Args) >= 3 && os.Args[2] != "" {
			name = os.Args[2]
		}
		if err := uninstallTask(name); err != nil {
			reportTaskResult("Beszel autostart removal failed", err.Error(), true)
			os.Exit(1)
		}
		reportTaskResult("Beszel autostart removed", fmt.Sprintf(
			"Scheduled task %q stopped and removed.", name), false)
		return
	}
	if err := launch(); err != nil {
		appendLog(err)
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// reportTaskResult surfaces install/uninstall outcomes to double-click users:
// the binary is built with -H windowsgui, so there is no console to read and
// a message box is the only visible channel. BESZEL_MONITOR_NO_MSGBOX=1 keeps
// automation and tests non-interactive (result goes to stdout/stderr instead).
func reportTaskResult(title, message string, isError bool) {
	if os.Getenv("BESZEL_MONITOR_NO_MSGBOX") == "1" {
		if isError {
			fmt.Fprintln(os.Stderr, title+": "+message)
			return
		}
		fmt.Println(title + ": " + message)
		return
	}
	caption, _ := windows.UTF16PtrFromString(title)
	text, _ := windows.UTF16PtrFromString(message)
	icon := uint32(windows.MB_ICONINFORMATION)
	if isError {
		icon = windows.MB_ICONERROR
	}
	windows.MessageBox(0, text, caption, icon|windows.MB_SETFOREGROUND)
}

func launch() error {
	if !acquireSingleInstanceLock() {
		return nil // another instance is already bringing up the dashboard
	}
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	url := fmt.Sprintf("http://%s:%d", cfg.Host, cfg.Port)
	if !isReady(url) {
		missingTask := false
		for _, task := range cfg.Tasks {
			if err := startScheduledTask(task); err != nil {
				if taskExists(task) {
					return fmt.Errorf("start scheduled task %q: %w", task, err)
				}
				// fresh unzip without install-task: note it and bring the hub
				// up directly below instead of failing with a cryptic
				// "file not found" from schtasks
				appendLog(fmt.Errorf("scheduled task %q is not registered; starting the hub directly", task))
				missingTask = true
			}
		}
		if missingTask || len(cfg.Tasks) == 0 {
			if err := startHubScript(cfg.HubScript); err != nil {
				return err
			}
			registerAutostart()
		}
		if !waitUntilReady(url, time.Duration(cfg.StartupTimeoutSeconds)*time.Second) {
			return fmt.Errorf(
				"dashboard at %s did not become ready within %ds; check the scheduled tasks and local hub.log",
				url, cfg.StartupTimeoutSeconds,
			)
		}
	}
	if *cfg.OpenBrowser {
		openBrowser(url)
	}
	return nil
}

// resolveHubScript finds the hub start script next to the executable: the
// configured hubScript (package layout: app\run-hub.ps1), falling back to a
// script beside the exe (flat layout used by earlier deployments).
func resolveHubScript(rel string) (string, error) {
	exePath, err := os.Executable()
	if err != nil {
		return "", err
	}
	dir := filepath.Dir(exePath)
	for _, candidate := range []string{rel, "run-hub.ps1"} {
		path, err := filepath.Abs(filepath.Join(dir, filepath.FromSlash(candidate)))
		if err != nil {
			continue
		}
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("hub script not found beside Monitor.exe (tried %s and run-hub.ps1)", rel)
}

// startHubScript starts the hub without a scheduled task by spawning a
// detached runner (`Monitor.exe task <script>`). The child outlives this
// launcher, so a fresh unzip works with a plain double-click.
func startHubScript(rel string) error {
	script, err := resolveHubScript(rel)
	if err != nil {
		return err
	}
	exePath, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(exePath, "task", script)
	cmd.Dir = filepath.Dir(script)
	hideWindow(cmd)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start hub script %s: %w", script, err)
	}
	// deliberately not waited on: the runner owns the hub for the session
	return nil
}

// registerAutostart installs the logon task on first use, so a portable
// install gains autostart without any extra step. Best-effort: failures are
// logged, never fatal. BESZEL_MONITOR_NO_AUTOREG=1 keeps automation and the
// E2E gate (which must not clobber a developer machine's real task) away
// from the scheduler.
func registerAutostart() {
	if os.Getenv("BESZEL_MONITOR_NO_AUTOREG") == "1" {
		return
	}
	name := autostartTaskName()
	if taskExists(name) {
		return
	}
	if err := installTask(name, false); err != nil {
		appendLog(fmt.Errorf("autostart registration failed: %w", err))
		return
	}
	appendLog(fmt.Errorf("registered logon task %q; run %q to remove it", name, "Monitor.exe uninstall-task"))
}

func schtasksPath() string {
	return filepath.Join(os.Getenv("WINDIR"), "System32", "schtasks.exe")
}

// taskExists reports whether the scheduled task is registered. A query
// failure for any other reason is also treated as missing, which only ever
// results in the direct-start fallback, never a wrong success.
func taskExists(name string) bool {
	cmd := exec.Command(schtasksPath(), "/Query", "/TN", name)
	hideWindow(cmd)
	return cmd.Run() == nil
}

// installTask registers the logon task that starts the hub script via this
// binary's task-runner mode; with start it also runs it immediately.
// Equivalent to the previous install-task.ps1 (RestartCount 10 / no time
// limit / hidden) but immune to PowerShell execution policies. The autostart
// registration passes start=false because the hub is already running from the
// direct start; running the task then would just lose a port-bind race and
// churn through the task's failure-restart budget.
func installTask(name string, start bool) error {
	exePath, err := os.Executable()
	if err != nil {
		return err
	}
	script, err := resolveHubScript(`app\run-hub.ps1`)
	if err != nil {
		return err
	}
	userID := os.Getenv("USERNAME")
	if domain := os.Getenv("USERDOMAIN"); domain != "" {
		userID = domain + "\\" + userID
	}
	xml := taskXML(exePath, script, filepath.Dir(exePath), userID)
	xmlPath := filepath.Join(os.TempDir(), "beszel-task-"+name+".xml")
	// schtasks /XML requires UTF-16 with a BOM
	if err := writeUTF16(xmlPath, xml); err != nil {
		return err
	}
	defer os.Remove(xmlPath)

	cmd := exec.Command(schtasksPath(), "/Create", "/TN", name, "/XML", xmlPath, "/F")
	hideWindow(cmd)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("register task: %w: %s", err, out)
	}
	if start {
		cmd = exec.Command(schtasksPath(), "/Run", "/TN", name)
		hideWindow(cmd)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("start task: %w: %s", err, out)
		}
	}
	return nil
}

func uninstallTask(name string) error {
	// ending a stopped or missing task fails harmlessly; deletion is the goal
	cmd := exec.Command(schtasksPath(), "/End", "/TN", name)
	hideWindow(cmd)
	_ = cmd.Run()
	cmd = exec.Command(schtasksPath(), "/Delete", "/TN", name, "/F")
	hideWindow(cmd)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("delete task: %w: %s", err, out)
	}
	return nil
}

// taskXML builds the Task Scheduler definition; schtasks requires UTF-16, so
// non-ASCII installation paths and user names are safe.
func taskXML(exe, script, workingDir, userID string) string {
	escape := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-16"?>
<Task version="1.2" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <RegistrationInfo>
    <Description>Run the local Beszel monitoring hub.</Description>
  </RegistrationInfo>
  <Triggers>
    <LogonTrigger>
      <Enabled>true</Enabled>
      <UserId>%s</UserId>
    </LogonTrigger>
  </Triggers>
  <Settings>
    <MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>
    <DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>
    <StopIfGoingOnBatteries>false</StopIfGoingOnBatteries>
    <StartWhenAvailable>true</StartWhenAvailable>
    <RestartOnFailure>
      <Count>10</Count>
      <Interval>PT1M</Interval>
    </RestartOnFailure>
    <ExecutionTimeLimit>PT0S</ExecutionTimeLimit>
    <Enabled>true</Enabled>
    <Hidden>true</Hidden>
  </Settings>
  <Actions Context="Author">
    <Exec>
      <Command>%s</Command>
      <Arguments>task "%s"</Arguments>
      <WorkingDirectory>%s</WorkingDirectory>
    </Exec>
  </Actions>
</Task>
`, escape.Replace(userID), escape.Replace(exe), escape.Replace(script), escape.Replace(workingDir))
}

func writeUTF16(path, text string) error {
	encoded, err := windows.UTF16FromString(text)
	if err != nil {
		return err
	}
	buf := make([]byte, 0, 2+2*len(encoded))
	buf = append(buf, 0xFF, 0xFE) // little-endian BOM
	for _, u := range encoded {
		buf = append(buf, byte(u), byte(u>>8))
	}
	return os.WriteFile(path, buf, 0o600)
}

func acquireSingleInstanceLock() bool {
	name, err := windows.UTF16PtrFromString(mutexName)
	if err != nil {
		return true
	}
	handle, err := windows.CreateMutex(nil, true, name)
	if err == windows.ERROR_ALREADY_EXISTS {
		if handle != 0 {
			windows.CloseHandle(handle)
		}
		return false
	}
	// keep the mutex handle open for the lifetime of the process
	return handle != 0
}

func startScheduledTask(name string) error {
	cmd := exec.Command(schtasksPath(), "/Run", "/TN", name)
	hideWindow(cmd)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%w: %s", err, out)
	}
	return nil
}

// probeClient never uses a proxy and times out fast: readiness of the local
// dashboard is all it measures.
var probeClient = &http.Client{
	Timeout:   time.Second,
	Transport: &http.Transport{Proxy: nil, DialContext: (&net.Dialer{Timeout: time.Second}).DialContext},
}

// isReady does a single probe of the dashboard, so a double-click while the
// hub is already up just opens the browser instead of re-running tasks.
func isReady(url string) bool {
	resp, err := probeClient.Get(url)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func waitUntilReady(url string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		if isReady(url) {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func openBrowser(url string) {
	cmd := exec.Command(
		filepath.Join(os.Getenv("WINDIR"), "System32", "rundll32.exe"),
		"url.dll,FileProtocolHandler", url,
	)
	hideWindow(cmd)
	_ = cmd.Start()
}

// runTask executes a PowerShell script with no window and returns its exit
// code. The script's process tree is tied to a kill-on-close job object so no
// orphans survive if this runner is terminated.
func runTask(scriptPath string) int {
	scriptPath, err := filepath.Abs(scriptPath)
	if err != nil {
		return 2
	}
	if _, err := os.Stat(scriptPath); err != nil {
		return 2
	}
	shell, err := findPowerShell()
	if err != nil {
		return 3
	}
	// Bypass keeps the runner working on machines whose execution policy is
	// Restricted or RemoteSigned: scripts downloaded inside the release zip
	// carry the mark-of-the-web and would otherwise be refused.
	cmd := exec.Command(shell, "-NoLogo", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", scriptPath)
	cmd.Dir = filepath.Dir(scriptPath)
	hideWindow(cmd)
	if err := cmd.Start(); err != nil {
		return 4
	}
	job, err := assignKillOnCloseJob(cmd.Process)
	if err != nil {
		_ = cmd.Process.Kill()
		return 5
	}
	defer windows.CloseHandle(job)

	if err := cmd.Wait(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitCode()
		}
		return 1
	}
	return 0
}

// findPowerShell prefers PowerShell 7, falling back to the built-in PowerShell.
func findPowerShell() (string, error) {
	candidates := []string{
		filepath.Join(os.Getenv("ProgramW6432"), `PowerShell\7\pwsh.exe`),
		filepath.Join(os.Getenv("ProgramFiles"), `PowerShell\7\pwsh.exe`),
	}
	for _, candidate := range candidates {
		if candidate != "" {
			if _, err := os.Stat(candidate); err == nil {
				return candidate, nil
			}
		}
	}
	return exec.LookPath("powershell.exe")
}

func assignKillOnCloseJob(proc *os.Process) (windows.Handle, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return 0, err
	}
	var info jobObjectExtendedLimitInformation
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if err := setInformationJobObject(
		job, jobObjectExtendedLimitInformationClass, unsafe.Pointer(&info), uint32(unsafe.Sizeof(info)),
	); err != nil {
		windows.CloseHandle(job)
		return 0, err
	}
	handle, err := windows.OpenProcess(
		windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(proc.Pid),
	)
	if err != nil {
		windows.CloseHandle(job)
		return 0, err
	}
	defer windows.CloseHandle(handle)
	if err := windows.AssignProcessToJobObject(job, handle); err != nil {
		windows.CloseHandle(job)
		return 0, err
	}
	return job, nil
}

// job object structs and syscall: x/sys/windows does not export them.
const jobObjectExtendedLimitInformationClass = 9

type jobObjectBasicLimitInformation struct {
	PerProcessUserTimeLimit int64
	PerJobUserTimeLimit     int64
	LimitFlags              uint32
	MinimumWorkingSetSize   uintptr
	MaximumWorkingSetSize   uintptr
	ActiveProcessLimit      uint32
	Affinity                uintptr
	PriorityClass           uint32
	SchedulingClass         uint32
}

type ioCounters struct {
	ReadOperationCount  uint64
	WriteOperationCount uint64
	OtherOperationCount uint64
	ReadTransferCount   uint64
	WriteTransferCount  uint64
	OtherTransferCount  uint64
}

type jobObjectExtendedLimitInformation struct {
	BasicLimitInformation jobObjectBasicLimitInformation
	IoInfo                ioCounters
	ProcessMemoryLimit    uintptr
	JobMemoryLimit        uintptr
	PeakProcessMemoryUsed uintptr
	PeakJobMemoryUsed     uintptr
}

var procSetInformationJobObject = windows.NewLazySystemDLL("kernel32.dll").
	NewProc("SetInformationJobObject")

func setInformationJobObject(
	job windows.Handle, infoClass uintptr, info unsafe.Pointer, infoLen uint32,
) error {
	ret, _, err := procSetInformationJobObject.Call(
		uintptr(job), infoClass, uintptr(info), uintptr(infoLen),
	)
	if ret == 0 {
		return err
	}
	return nil
}

func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &windows.SysProcAttr{
		HideWindow:    true,
		CreationFlags: windows.CREATE_NO_WINDOW,
	}
}

func appendLog(err error) {
	exePath, pathErr := os.Executable()
	if pathErr != nil {
		return
	}
	logPath := filepath.Join(filepath.Dir(exePath), "launcher.log")
	file, openErr := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if openErr != nil {
		return
	}
	defer file.Close()
	fmt.Fprintf(file, "%s %s\r\n", time.Now().Format(time.RFC3339), err)
}
