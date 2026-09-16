oh-my-beszel Linux build (amd64, static)

Quick start - hub:
  ./beszel serve --http 0.0.0.0:8090
  Open http://<host>:8090 - the first visit creates the admin account.
  Optional: USER_EMAIL=... USER_PASSWORD=... pre-create it on first start
  (only applies to an empty database). Data lives in ./beszel_data next
  to the binary; back that directory up to keep history and settings.

Quick start - agent (usually not needed: the hub deploys agents over
SSH by itself when you import systems from an SSH config):
  KEY='<hub public key from Settings>' LISTEN=127.0.0.1:45876 ./beszel-agent
  beszel-agent-glibc is the same agent with NVIDIA GPU metrics via NVML;
  it needs glibc (any mainstream distro qualifies, Alpine does not).

Autostart (systemd), hub example:
  sudo install -d /opt/beszel && sudo install -m 0755 beszel /opt/beszel/
  sudo tee /etc/systemd/system/beszel-hub.service >/dev/null <<'EOF'
  [Unit]
  Description=Beszel Hub
  After=network-online.target
  Wants=network-online.target
  [Service]
  WorkingDirectory=/opt/beszel
  ExecStart=/opt/beszel/beszel serve --http 0.0.0.0:8090
  Restart=always
  RestartSec=5
  [Install]
  WantedBy=multi-user.target
  EOF
  sudo systemctl enable --now beszel-hub

Verify the download: sha256sum --check sha256sums.txt
