# sshui

Keyboard-driven TUI for editing a single OpenSSH client config file (default: `~/.ssh/config` or `SSH_CONFIG`). Built with Go, Bubble Tea, and Bubbles.

**Back up your config before relying on save** — the app rewrites the file with stable formatting.

## Features

- Grouped tree (sshclick-style `#@group:` / `#@desc:` metadata)
- Per-host directives: add (catalog picker or custom key), edit value, delete
- Duplicate / delete host, new host in the default section
- `Include` is detected; only the opened file is written (Phase 1 per plan)
- Directive catalog for discovery (unknown keys can still be entered with `k`)

## Build

```bash
go build -o sshui ./cmd/sshui/
```

Cross-compile: `make dist` (Darwin arm64 + Linux amd64).

## Run

```bash
./sshui
./sshui --config /path/to/config
```

Press `?` in the app for keybindings.

## Help

```bash
./sshui --help
```
