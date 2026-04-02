# Configuration reference

## App settings (`config.toml`)

Path: `~/.config/sshui/config.toml` (macOS: `~/Library/Application Support/sshui/config.toml`).

Created automatically on first run with sensible defaults. Edit with `$` in the TUI or any text editor.

### Top-level keys

| Key | Default | Purpose |
|-----|---------|---------|
| `ssh_config` | `~/.ssh/config` | Main OpenSSH client config path |
| `editor` | `$VISUAL` / `$EDITOR` / `vi` | Shell command for raw edit (`sh -c '$editor $tmpfile'`) |
| `theme` | `default` | TUI theme: `default`, `warm`, or `muted` |
| `ssh_config_git_mirror` | _(empty)_ | Copy saved config here after each save (e.g. `~/dotfiles/ssh/config`) |

### `[hosts]` section

| Key | Default | Purpose |
|-----|---------|---------|
| `ssh_hosts_path` | `~/.config/sshui/ssh_hosts` | OpenSSH-format file sshui edits |
| `password_overlay_path` | `~/.config/sshui/password_hosts.toml` | Password overlay (TOML) |
| `ensure_include` | `true` | Append `Include` to main ssh_config on startup if missing |
| `browse_mode` | `merged` | Initial tree view: `merged`, `openssh`, or `password` |
| `first_run_openssh_export_wizard` | `true` | Offer to import hosts from main ssh_config |
| `first_run_openssh_export_done` | `false` | Set after the wizard completes or is skipped |

### Example

```toml
editor = "nvim"
theme = "warm"

[hosts]
ssh_hosts_path = "~/.config/sshui/ssh_hosts"
password_overlay_path = "~/.config/sshui/password_hosts.toml"
browse_mode = "merged"
```

## SSH config path resolution

Order of precedence:

1. `--config` flag
2. `$SSH_CONFIG` environment variable
3. `ssh_config` in `config.toml`
4. `~/.ssh/config`

## File layout

```
~/.config/sshui/
  config.toml            # app settings
  ssh_hosts              # OpenSSH host definitions (primary edit target)
  password_hosts.toml    # password overlay (SSH_ASKPASS entries)

~/.ssh/
  config                 # main ssh_config (Include ssh_hosts appended here)
```

## Include behavior

If `ensure_include` is `true` (default), sshui appends an `Include` directive pointing to `ssh_hosts` at the top of your main `~/.ssh/config` on startup, with a backup. This lets `ssh <alias>` work from the shell without sshui.

If the opened file contains `Include`, sshui starts in **read-only merged mode**: it loads the main file plus every included file and shows them in one tree. Save is disabled because one write cannot safely update multiple files. Press `W` to switch to a writable view of only the main file. Press `W` again or `r` to return to merged browse.

## Password overlay (`password_hosts.toml`)

Each entry maps Host patterns to an askpass script for password-based SSH authentication.

```toml
version = 1

[[password_host]]
patterns = ["legacy-box", "*.pw.example.com"]
hostname = "192.168.50.10"
user = "admin"
port = 22
askpass = "~/.local/bin/ssh-askpass-pass"
askpass_require = "force"

[[password_host]]
patterns = ["router-lan"]
hostname = "192.168.0.1"
user = "root"
port = 22
askpass = "~/.local/bin/ssh-askpass-op"
```

| Field | Required | Purpose |
|-------|----------|---------|
| `patterns` | yes | OpenSSH Host-style patterns for matching |
| `hostname` | yes | DNS name or IP to connect to |
| `user` | no | SSH username |
| `port` | no | SSH port (default: 22) |
| `askpass` | yes | Path to askpass script (prints password to stdout) |
| `askpass_require` | no | `SSH_ASKPASS_REQUIRE` value (recommended: `force`) |
| `display` | no | `DISPLAY` override for older ssh builds |

See [ASKPASS.md](ASKPASS.md) for recipes with `pass`, `gopass`, KeePassXC, `secret-tool`, and `age`.

## Browse modes

Toggle with `B` in the TUI. The last choice is persisted in `config.toml`.

| Mode | Content |
|------|---------|
| `merged` | OpenSSH hosts + password overlay hosts in one tree |
| `openssh` | Only hosts from `ssh_hosts` |
| `password` | Only password overlay entries |

## Edit mode and locking

sshui starts in **read-only** mode. Actions that modify files (add/edit/delete hosts, directives, groups, save) automatically enter edit mode and acquire a `.sshui.swp` lock file adjacent to the data file. This prevents concurrent sshui instances from overwriting each other's changes. `Esc` on a host detail view exits edit mode first (releasing the lock), then exits the view.

## Environment variables

| Variable | Purpose |
|----------|---------|
| `SSH_CONFIG` | Override SSH config path (see resolution above) |
| `NO_COLOR=1` | Disable ANSI colors in the TUI |
| `VISUAL` / `EDITOR` | Editor for raw edit and app config edit |
