# Branding assets

- `oh-my-beszel.ico` — multi-size Windows icon (16–256 px) generated from the
  squared logo `docs/assets/oh-my-beszel-logo.png`.
- The compiled binaries embed this icon via the committed resource files
  `internal/cmd/monitor/rsrc_windows_amd64.syso` and
  `internal/cmd/hub/rsrc_windows_amd64.syso`; the `_windows_amd64` suffix keeps
  them out of non-Windows builds. No loose logo/ico file ships in the package.
- The web UI favicon is `internal/site/public/static/icon.png` (512×512,
  also generated from the squared logo).

To regenerate after replacing the logo (square canvas, full-bleed):

```powershell
python -c "from PIL import Image; img = Image.open('docs/assets/oh-my-beszel-logo.png').convert('RGBA'); img.resize((512,512), Image.LANCZOS).save('internal/site/public/static/icon.png'); img.save('deploy/windows/branding/oh-my-beszel.ico', sizes=[(16,16),(24,24),(32,32),(48,48),(64,64),(128,128),(256,256)])"
go run github.com/akavel/rsrc@latest -ico deploy/windows/branding/oh-my-beszel.ico -o internal/cmd/monitor/rsrc_windows_amd64.syso
Copy-Item internal/cmd/monitor/rsrc_windows_amd64.syso internal/cmd/hub/rsrc_windows_amd64.syso
```
