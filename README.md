# sshui

Repository: [github.com/sacckth/sshui](https://github.com/sacckth/sshui).

Keyboard-driven TUI for editing a single OpenSSH client config file. Built with Go, Bubble Tea, and Bubbles.

**Back up your config before relying on save** — the app rewrites the file with stable formatting.

## Config resolution

1. `--config`  
2. `$SSH_CONFIG`  
3. `ssh_config` in app settings (below)  
4. `~/.ssh/config`

## App settings (`~/.config/sshui/config.toml`)

Optional TOML (macOS: `~/Library/Application Support/sshui/config.toml`). All keys optional.

| Key | Purpose |
|-----|---------|
| `ssh_config` | Default SSH client config path |
| `editor` | Shell command prefix for raw edit (e.g. `vim` or `code --wait`); runs as `sh -c '$editor $tmpfile'` |
| `theme` | `default`, `warm`, or `muted` (Lip Gloss accents) |
| `ssh_config_git_mirror` | After each successful save, write the same bytes here (e.g. `~/src/dotfiles/ssh/config`); parents are created; `0600` |

Set `NO_COLOR=1` to disable ANSI colors in the TUI.

## Features

- Split host detail: tree + Overview / all directives / Connectivity tabs; `tab` switches pane focus
- sshclick-style `#@group:` / `#@desc:` / `#@info:` / `#@host:` (per-host comments before `Host`)
- Actions menu (`A`): `ssh`, `sftp`, copy `ssh <alias>` (single non-wildcard pattern)
- `Include`: read-only merged browse of included files as `include:<filename>` groups
- Optional git/dotfiles mirror on save (`ssh_config_git_mirror`)
- CLI: `list`, `show`, `dump` (`--json`, `--check`), `completion bash|zsh|fish`

## Build

```bash
go build -o sshui ./cmd/sshui/
```

Cross-compile: `make dist` (Darwin arm64 + Linux amd64).

**Packages:** `make packages` builds a static **linux/amd64** binary and produces **`.deb`**, **`.rpm`**, and **`.apk`** under `dist/` (via [nfpm](https://github.com/goreleaser/nfpm)), plus **`sshui-<version>-darwin-arm64.tar.gz`** for macOS Apple Silicon. Override version with `make packages VERSION=1.0.0`. Requires Go only (nfpm is run with `go run`).

## Run

```bash
./sshui
./sshui --config /path/to/config
./sshui list
./sshui show myhost --json
./sshui dump --check
```

Press `?` in the TUI for keybindings.

## Help

```bash
./sshui --help
./sshui completion bash  # pipe to your shell rc
```
