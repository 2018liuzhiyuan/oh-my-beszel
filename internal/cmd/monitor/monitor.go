//go:build windows

// Monitor is the local deployment helper for the Beszel hub. It replaces the
// previous C# launcher, task runner, and BeszelLauncher with one static binary
// and has two modes:
//
//	Monitor.exe              start tasks, wait for the dashboard, open browser
//	Monitor.exe task <ps1>   run a PowerShell script hidden (used by tasks)
//
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
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const mutexName = `Local\BeszelMonitorLauncher`

type HubConfig struct {
	UserEmail    string `json:"userEmail"`
	UserPassword string `json:"userPassword"`
	AutoLogin    string `json:"autoLogin"`
	CheckUpdates *bool  `json:"checkUpdates"`
}

type Config struct {
	Host                  string    `json:"host"`
	Port                  int       `json:"port"`
	SSHConfigPath         string    `json:"sshConfigPath"`
	OpenBrowser           *bool     `json:"openBrowser"`
	StartupTimeoutSeconds int       `json:"startupTimeoutSeconds"`
	Tasks                 []string  `json:"tasks"`
	Hub                   HubConfig `json:"hub"`
}

func loadConfig() (*Config, error) {
	cfg := &Config{
		Host:                  "127.0.0.1",
		Port:                  8090,
		OpenBrowser:           new(bool),
		StartupTimeoutSeconds: 45,
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
	return cfg, nil
}

func main() {
	if len(os.Args) == 3 && os.Args[1] == "task" {
		os.Exit(runTask(os.Args[2]))
	}
	if err := launch(); err != nil {
		appendLog(err)
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func launch() error {
	if !acquireSingleInstanceLock() {
		return nil // another instance is already bringing up the dashboard
	}
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	for _, task := range cfg.Tasks {
		if err := startScheduledTask(task); err != nil {
			return fmt.Errorf("start scheduled task %q: %w", task, err)
		}
	}
	url := fmt.Sprintf("http://%s:%d", cfg.Host, cfg.Port)
	if !waitUntilReady(url, time.Duration(cfg.StartupTimeoutSeconds)*time.Second) {
		return fmt.Errorf(
			"dashboard at %s did not become ready within %ds; check the scheduled tasks and local hub.log",
			url, cfg.StartupTimeoutSeconds,
		)
	}
	if *cfg.OpenBrowser {
		openBrowser(url)
	}
	return nil
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
	schtasks := filepath.Join(os.Getenv("WINDIR"), "System32", "schtasks.exe")
	cmd := exec.Command(schtasks, "/Run", "/TN", name)
	hideWindow(cmd)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%w: %s", err, out)
	}
	return nil
}

func waitUntilReady(url string, timeout time.Duration) bool {
	client := &http.Client{
		Timeout:   time.Second,
		Transport: &http.Transport{Proxy: nil, DialContext: (&net.Dialer{Timeout: time.Second}).DialContext},
	}
	deadline := time.Now().Add(timeout)
	for {
		if resp, err := client.Get(url); err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return true
			}
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
	cmd := exec.Command(shell, "-NoLogo", "-NoProfile", "-NonInteractive", "-File", scriptPath)
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
