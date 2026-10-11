package hub

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"path"
	"strconv"
	"strings"

	"github.com/pocketbase/pocketbase/core"
)

type agentDeploymentIdentity struct {
	machineFingerprint string
	remoteUID          string
	installDir         string
}

func (identity agentDeploymentIdentity) instanceID() string {
	sum := sha256.Sum256([]byte(identity.machineFingerprint + "\x00" + identity.remoteUID + "\x00" + identity.installDir))
	return hex.EncodeToString(sum[:])
}

type agentDeploymentResult struct {
	port uint16
	agentDeploymentIdentity
}

func parseAgentDeploymentResult(output string) (agentDeploymentResult, error) {
	port, err := parseAgentInstallPort(output)
	if err != nil {
		return agentDeploymentResult{}, err
	}
	result := agentDeploymentResult{port: port}
	for line := range strings.Lines(output) {
		key, value, _ := strings.Cut(strings.TrimSpace(line), "=")
		switch key {
		case "agent_machine":
			result.machineFingerprint = value
		case "agent_uid":
			result.remoteUID = value
		case "agent_dir_b64":
			decoded, err := base64.StdEncoding.DecodeString(value)
			if err != nil {
				return agentDeploymentResult{}, fmt.Errorf("invalid agent installation directory: %w", err)
			}
			result.installDir = string(decoded)
		}
	}
	uid, err := strconv.ParseUint(result.remoteUID, 10, 32)
	if err != nil || strconv.FormatUint(uid, 10) != result.remoteUID ||
		result.machineFingerprint == "" || len(result.machineFingerprint) > 256 ||
		strings.ContainsAny(result.machineFingerprint, "\x00\r\n") ||
		!path.IsAbs(result.installDir) || path.Clean(result.installDir) != result.installDir ||
		strings.ContainsAny(result.installDir, "\x00\r\n") {
		return agentDeploymentResult{}, fmt.Errorf("invalid or missing remote agent identity")
	}
	return result, nil
}

func persistAgentDeployment(app core.App, target agentDeploymentTarget, result agentDeploymentResult) error {
	record, err := app.FindRecordById("systems", target.id)
	if err != nil {
		return fmt.Errorf("load system after agent deployment: %w", err)
	}
	if strings.TrimSpace(record.GetString("host")) != target.host ||
		strings.TrimSpace(record.GetString("ssh_config")) != target.sshConfig ||
		(record.GetString("port") != strconv.Itoa(int(target.port)) && record.GetString("port") != strconv.Itoa(int(result.port))) {
		return fmt.Errorf("system SSH target changed during deployment")
	}
	record.Set("port", strconv.Itoa(int(result.port)))
	record.Set("agent_machine_fingerprint", result.machineFingerprint)
	record.Set("agent_remote_uid", result.remoteUID)
	record.Set("agent_install_dir", result.installDir)
	record.Set("agent_instance_id", result.instanceID())
	if err := app.SaveNoValidate(record); err != nil {
		return fmt.Errorf("save agent deployment identity and port: %w", err)
	}
	if result.port != target.port {
		app.Logger().Info("Agent port conflict resolved automatically", "system", target.id, "host", target.host, "previousPort", target.port, "port", result.port)
	}
	return nil
}

func protectAgentDeploymentIdentity(event *core.RecordRequestEvent) error {
	info, err := event.RequestInfo()
	if err != nil {
		return err
	}
	for _, name := range []string{"agent_machine_fingerprint", "agent_remote_uid", "agent_install_dir", "agent_instance_id"} {
		if _, supplied := info.Body[name]; supplied {
			return event.BadRequestError("Agent identity is managed by the Hub.", nil)
		}
	}
	return event.Next()
}
