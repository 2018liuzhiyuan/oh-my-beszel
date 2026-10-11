import argparse
import base64
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import signal
import socket
import struct
import subprocess
import tempfile
import secrets
import urllib.error
import urllib.request
import time
from concurrent.futures import ThreadPoolExecutor
from datetime import datetime, timezone

ROOT = Path(__file__).resolve().parents[2]


def scripts():
    source = (ROOT / "internal/hub/agent_deploy_ssh.go").read_text(encoding="utf-8")
    install = re.search(r"func agentInstallScript\(\) string \{\s*return `([\s\S]*?)`\s*\}", source).group(1)
    probe = re.search(r"func agentProbeCommand[\s\S]*?return fmt.Sprintf\(`([\s\S]*?)`, port, publicKeyBase64\)", source).group(1)
    return install, probe


def cleanup_fixture(fixture):
    """Stop only executables owned by this fixture, even after a partial install."""
    errors = []
    # Scan executables as well as PID files: an installer can fail before writing a PID.
    for entry in Path("/proc").iterdir():
        if not entry.name.isdigit():
            continue
        try:
            # Docker root lacks CAP_SYS_PTRACE for other UIDs. Read the
            # executable with its owner's filesystem credentials, then restore
            # root before signalling; never trust PID metadata.
            original_uid = os.geteuid()
            try:
                if original_uid == 0:
                    os.seteuid(entry.stat().st_uid)
                executable = (entry / "exe").readlink()
            finally:
                os.seteuid(original_uid)
            if not str(executable).startswith(str(fixture) + "/"):
                continue
            process_id = int(entry.name)
            os.kill(process_id, signal.SIGTERM)
            for _ in range(50):
                try:
                    if (entry / "stat").read_text().split()[2] == "Z":
                        break
                except FileNotFoundError:
                    break
                time.sleep(0.1)
            else:
                os.kill(process_id, signal.SIGKILL)
        except (FileNotFoundError, ProcessLookupError):
            pass
        except OSError as error:
            errors.append(str(error))
    try:
        shutil.rmtree(fixture)
    except OSError as error:
        errors.append(str(error))
    if errors:
        raise RuntimeError("fixture cleanup failed: " + "; ".join(errors))


def linux_run(agent, output):
    import fcntl
    if os.getuid() != 0:
        raise ValueError("Linux isolation tests require root for the different-UID fixture")
    agent = agent.resolve(strict=True)
    output = output.resolve()
    output.parent.mkdir(parents=True, exist_ok=True)
    install_script, probe_template = scripts()
    fixture = Path(tempfile.mkdtemp(prefix="beszel-agent-identity-"))
    fixture.chmod(0o755)
    homes = []
    checks = []
    key = base64.b64encode(struct.pack(">I", 11) + b"ssh-ed25519" + struct.pack(">I", 32) + bytes(range(32))).decode()
    public_key = "ssh-ed25519 " + key
    key_b64 = base64.b64encode(public_key.encode()).decode()
    data = agent.read_bytes()
    checksum = hashlib.sha256(data).hexdigest()

    def home(name, uid=None):
        directory = fixture / name
        directory.mkdir(mode=0o700)
        if uid is not None:
            os.chown(directory, uid, uid)
        homes.append(directory)
        return directory

    mock_tools = fixture / "mock-tools"
    mock_tools.mkdir(mode=0o755)
    (mock_tools / "stat").write_text('#!/bin/sh\nif [ "$1" = "-f" ]; then echo nfs; else exec /usr/bin/stat "$@"; fi\n')
    (mock_tools / "hostname").write_text('#!/bin/sh\nprintf \'%s\\n\' "$BESZEL_E2E_HOST"\n')
    for tool in mock_tools.iterdir():
        tool.chmod(0o755)

    def environment(directory, namespace=None):
        env = dict(os.environ)
        for name in ("LISTEN", "KEY", "KEY_FILE", "TOKEN", "HUB_URL", "DATA_DIR"):
            env.pop(name, None)
        env.update(HOME=str(directory), XDG_CONFIG_HOME=str(directory / ".config"),
                   XDG_RUNTIME_DIR=str(directory / "missing-runtime"))
        if namespace:
            env.update(PATH=str(mock_tools) + ":" + env["PATH"], BESZEL_E2E_HOST=namespace)
        return env

    def install(directory, port, first, last, uid=None, content=None, namespace=None):
        descriptor, stage_path = tempfile.mkstemp(prefix="beszel-agent-identity-stage-", dir="/tmp")
        os.close(descriptor)
        stage = Path(stage_path)
        stage.write_bytes(data if content is None else content)
        stage.chmod(0o700)
        if uid is not None:
            os.chown(stage, uid, uid)
        def switch_user():
            os.setgroups([])
            os.setgid(uid)
            os.setuid(uid)
        command = ["sh", "-s", "--", str(port), key_b64, stage.name, checksum, str(first), str(last)]
        try:
            return subprocess.run(command, input=install_script, text=True, capture_output=True,
                                  env=environment(directory, namespace), preexec_fn=switch_user if uid is not None else None, timeout=110)
        finally:
            for suffix in ("", ".env", ".run", ".service"):
                Path(str(stage) + suffix).unlink(missing_ok=True)

    def result(completed):
        if completed.returncode != 0:
            logs = {str(directory.name): (directory / "oh-my-beszel/logs/agent.log").read_text(errors="replace") for directory in homes if (directory / "oh-my-beszel/logs/agent.log").exists()}
            output.with_name("failure.json").write_text(json.dumps({"stderr": completed.stderr, "stdout": completed.stdout, "logs": logs}, indent=2), encoding="utf-8")
            raise AssertionError(completed.stderr + completed.stdout + json.dumps(logs))
        values = dict(line.split("=", 1) for line in completed.stdout.splitlines() if "=" in line)
        assert all(values.get(k) for k in ("agent_port", "agent_machine", "agent_uid", "agent_dir_b64")), values
        return values

    def pid(directory):
        return int((directory / "oh-my-beszel/data/agent.pid").read_text())

    def wait_dead(process_id):
        for _ in range(50):
            try:
                state = Path(f"/proc/{process_id}/stat").read_text().split()[2]
                if state == "Z":
                    return
            except FileNotFoundError:
                return
            time.sleep(0.1)
        raise AssertionError("fixture agent did not exit")

    occupied = socket.socket()
    occupied.bind(("127.0.0.1", 0))
    occupied.listen()
    requested = occupied.getsockname()[1]
    assert requested < 65520
    first, last = requested + 1, requested + 15
    try:
        primary = home("account-a")
        a = result(install(primary, requested, first, last))
        assert int(a["agent_port"]) != requested
        assert a["agent_uid"] == str(os.getuid())
        assert base64.b64decode(a["agent_dir_b64"]).decode() == str(primary / "oh-my-beszel")
        checks.append("fresh deployment avoids foreign listener and reports real machine/UID/directory")
        before = pid(primary)
        b = result(install(primary, requested + 14, first, last))
        assert a == b and pid(primary) == before
        checks.append("same account with a different requested port reuses the same PID and actual port")

        probe = probe_template.replace("%[1]d", str(requested)).replace("%[2]s", key_b64).replace("%%", "%")
        probed = subprocess.run(["sh", "-c", probe], text=True, capture_output=True, env=environment(primary), timeout=15)
        assert probed.returncode == 0, probed.stderr
        lines = probed.stdout.splitlines()
        assert lines[5:9] == ["healthy", "listening", "authorized", "managed"], lines
        assert "agent_port=" + a["agent_port"] in lines
        checks.append("probe finds the installed port instead of trusting the system record")

        concurrent = home("concurrent-account")
        lock_root = concurrent / "oh-my-beszel"
        lock_root.mkdir(mode=0o700)
        with (lock_root / ".deploy.lock").open("w") as lock, ThreadPoolExecutor(2) as executor:
            fcntl.flock(lock, fcntl.LOCK_EX)
            one = executor.submit(install, concurrent, requested, first, last)
            two = executor.submit(install, concurrent, requested + 13, first, last)
            time.sleep(0.5)
            assert not (lock_root / "bin/beszel-agent").exists()
            fcntl.flock(lock, fcntl.LOCK_UN)
            c, d = result(one.result()), result(two.result())
        assert c == d
        checks.append("concurrent installers wait on the remote lock and converge on one instance")

        old_pid = pid(primary)
        os.kill(old_pid, signal.SIGTERM)
        wait_dead(old_pid)
        health_env = environment(concurrent)
        health_env["DATA_DIR"] = str(concurrent / "oh-my-beszel/data")
        health = subprocess.run([str(concurrent / "oh-my-beszel/bin/beszel-agent"), "health"], env=health_env, capture_output=True, timeout=10)
        assert health.returncode == 0, health.stderr
        checks.append("stopping one instance leaves the other instance's health state intact")
        recovered = result(install(primary, requested, first, last))
        assert recovered == a and pid(primary) != old_pid
        checks.append("dead managed process recovers without changing identity or the selected port")

        bad = install(primary, requested, first, last, content=b"invalid artifact")
        assert bad.returncode != 0 and "checksum mismatch" in bad.stderr
        assert pid(primary) != old_pid and Path(f"/proc/{pid(primary)}/exe").exists()
        checks.append("bad uploaded artifact leaves the existing agent running")

        if os.getuid() == 0:
            separate = home("account-b", 65534)
            other = result(install(separate, int(a["agent_port"]), first, last, uid=65534))
            assert other["agent_machine"] == a["agent_machine"]
            assert other["agent_uid"] == "65534"
            assert other["agent_dir_b64"] != a["agent_dir_b64"]
            assert other["agent_port"] != a["agent_port"]
            assert (separate / "oh-my-beszel/config/env").stat().st_uid == 65534
            checks.append("different remote UIDs isolate directories/processes and avoid machine-wide port conflicts")
        else:
            raise AssertionError("different-UID scenario requires local root test fixture")

        racing = home("bind-race-account")
        with socket.socket() as free:
            free.bind(("127.0.0.1", 0))
            race_port = free.getsockname()[1]
        assert race_port < 65520
        with ThreadPoolExecutor(1) as executor, socket.socket() as competitor:
            pending = executor.submit(install, racing, race_port, race_port + 1, race_port + 15)
            for _ in range(200):
                if (racing / "oh-my-beszel/config/env").exists():
                    break
                time.sleep(0.02)
            competitor.bind(("127.0.0.1", race_port))
            competitor.listen()
            race_result = result(pending.result())
        assert int(race_result["agent_port"]) != race_port
        checks.append("a competitor binding after selection is rejected and triggers a new port")

        shared = home("shared-home")
        host_a = result(install(shared, requested, first, last, namespace="host-a"))
        host_b = result(install(shared, requested, first, last, namespace="host-b"))
        base_a = Path(base64.b64decode(host_a["agent_dir_b64"]).decode())
        base_b = Path(base64.b64decode(host_b["agent_dir_b64"]).decode())
        assert base_a != base_b and base_a.is_dir() and base_b.is_dir()
        assert int((base_a / "data/agent.pid").read_text()) != int((base_b / "data/agent.pid").read_text())
        host_a_probe = subprocess.run(["sh", "-c", probe], text=True, capture_output=True, env=environment(shared, "host-a"), timeout=15)
        assert host_a_probe.stdout.splitlines()[8] == "managed", host_a_probe.stdout
        assert "agent_port=" + host_a["agent_port"] in host_a_probe.stdout.splitlines()
        checks.append("NFS homes isolate each host's binary/config/data and the shared dispatcher selects the right instance")

        exhausted = home("exhausted-account")
        failure = install(exhausted, requested, requested, requested)
        assert failure.returncode == 24 and "BESZEL_AGENT_PORT_RANGE_EXHAUSTED" in failure.stderr
        checks.append("exhausted port range fails explicitly")
        output.write_text(json.dumps({"passed": True, "checks": checks}, ensure_ascii=False, indent=2), encoding="utf-8")
        print(json.dumps({"passed": True, "checks": checks}, ensure_ascii=False, indent=2))
    finally:
        occupied.close()
        cleanup_fixture(fixture)


def main():
    parser = argparse.ArgumentParser(description="Disposable Linux CI isolation checks (requires root)")
    parser.add_argument("--linux", action="store_true", required=True)
    parser.add_argument("--agent", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    linux_run(args.agent, args.output)


if __name__ == "__main__":
    main()
