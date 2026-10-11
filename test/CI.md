# Automatic regression coverage

`.github/workflows/regression.yml` runs on pull requests, main pushes and manual
requests. A disposable Linux container has no real user SSH configuration,
launchd or user systemd session. HOME and XDG configuration are temporary.
The root container permits switching to UID 65534; it does not use host sudo.

The workflow runs all Go tests with the `testing` tag, and Agent WebSocket tests
again with the race detector. These include TLS trust/hostname rejection,
authentication challenges, reconnect authentication and concurrent lifecycle
handling. Linux E2E builds a real Agent and executes extracted install/probe
scripts locally: no SSH server or real remote account is contacted.

E2E covers instance reuse, deployment locking, foreign listeners, managed port
recovery, corrupted uploads, different UID isolation, bind races, mocked NFS
host namespaces and port exhaustion. The NFS test overrides stat/hostname;
it does not mount NFS or verify real network filesystem locking semantics.
Reports and installer failure logs are uploaded even on failure.

Cleanup tests inject an assertion and verify that fixture executables stop,
unrelated processes survive, and temporary files disappear. E2E cleanup scans
fixture executable paths rather than trusting stale PID files, including
partial installs without PID metadata. Upload stages are removed in finally.
GitHub cancellation/timeout also destroys the disposable container.

The backend uses a placeholder embedded frontend to compile. This does not
verify frontend rendering/builds or the script's Windows live-Hub HTTP entry.
Real Windows PowerShell/WSL, macOS launchd and package installation, user systemd
integration, real SSH deployment and real NFS mounts still require platform
validation. Passing Linux CI must not be reported as passing these scenarios.

Local equivalent (root in a disposable Linux environment, Go from go.mod):

```sh
mkdir -p internal/site/dist
printf 'CI placeholder\n' > internal/site/dist/index.html
GOEXPERIMENT=nojsonv2 go test -tags testing -count=1 ./...
GOEXPERIMENT=nojsonv2 go test -race -tags testing ./agent -run 'Test(WebSocketClient|NewWebSocketClient)'
GOEXPERIMENT=nojsonv2 go build -o /tmp/beszel-ci-agent ./internal/cmd/agent
python3 -m unittest discover -s test -p 'test_agent_identity_cleanup.py' -v
python3 test/run_agent_identity_ci.py --linux --agent /tmp/beszel-ci-agent --output test/artifacts/ci/report.json
```
