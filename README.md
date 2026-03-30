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

Raw edit (`v` in the TUI) writes the **current in-memory** serialized config to a temp file, runs `editor` then `VISUAL` then `EDITOR` then `vi`, and replaces the model if the result parses.

## Features

- Grouped tree (sshclick-style `#@group:` / `#@desc:` metadata)
- Per-host directives: add (catalog picker or custom key), edit value, delete
- Duplicate / delete host, new host in the default section
- Raw `$EDITOR` buffer (`v`) and optional app config / themes
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
