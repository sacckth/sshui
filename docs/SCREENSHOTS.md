# README screenshots

The PNGs under `docs/screenshots/` are **illustrative** (e.g. AI-generated or hand-tuned mockups). They are **not** guaranteed to match pixel-perfect layout of the current TUI. The real interface is defined by the running app (title line, list chrome, filter bar, footer hints).

## Capturing real screenshots (synthetic config)

1. Build or install sshui (`go build -o sshui ./cmd/sshui` or `go install ./cmd/sshui@latest`).
2. From the repo root, run with the **fiction-only** fixture (RFC 5737-style addresses in `docs/readme-demo.conf`):

   ```bash
   sshui --config "$(pwd)/docs/readme-demo.conf"
   ```

3. Resize your terminal to a comfortable width (~100–120 columns) and consistent font.
4. Capture **browse** (default split view), **detail** (Enter on a host, optional tab to focus panes), and **filter** (press `/`, type a substring matching several hosts).
5. Save PNGs as `docs/screenshots/browse.png`, `detail.png`, `filter.png` (overwrite existing).
6. Skim captures for anything that looks like a real hostname, user, or IP; only ship obfuscated content.

Do not use your personal `~/.ssh/config` for repo screenshots.
