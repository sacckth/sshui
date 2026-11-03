# AGENTS.md — sshui

Guidance for humans and coding agents working on this repository.

## Project summary

**sshui** is a terminal UI for managing OpenSSH client host definitions. It stores managed hosts in a dedicated `ssh_hosts` file and optionally displays read-only hosts from the user's main `ssh_config` alongside them (dual tree). Saving **reformats** the managed file (deterministic layout); it does not preserve arbitrary comments except sshclick-style group metadata.

- **Upstream:** https://github.com/sacckth/sshui  
- **Go module:** `github.com/sacckth/sshui`  
- **Language / version:** Go 1.25+  
- **License:** BSD 3-Clause (`LICENSE`)

## Repository layout

| Path | Role |
|------|------|
| `cmd/sshui/main.go` | CLI entry: Cobra, path resolution, startup Include setup, parse files, start `tea.Program`; subcommands `list`, `show`, `dump`, `completion` |
| `internal/config/` | Domain types, **custom** line-oriented parser, **custom** writer (`Write` / `String`); Include handling (`include.go`, `include_helper.go`); text-level stanza removal (`remove.go`) |
| `internal/appcfg/` | Optional TOML: `~/.config/sshui/config.toml` (see README for macOS path) |
| `internal/overlay/` | `password_hosts.toml`: password hosts + groups, SSH_ASKPASS env (not OpenSSH config) |
| `internal/lockfile/` | `.sshui.swp` lock files beside edited files (PID-based, stale locks reclaimed) |
| `internal/sshkeywords/` | Static OpenSSH client keyword catalog for picker UX (not authoritative for OpenSSH) |
| `internal/tui/` | Bubble Tea model (`app.go`): tree, host detail, pickers, inputs, wizard, save; `host_edit.go` (new-host flow, duplicate-pattern note), `effective.go` (`ssh -G` checks), `raweditor.go`, `theme.go` |
| `docs/` | User docs: `CONFIGURATION.md`, `ASKPASS.md`, `RELEASING.md`, screenshots |
| `Makefile` | `build`, `test`, `dist`, `packages` (linux: deb/rpm/apk via nfpm; darwin-arm64 tarball) |
| `nfpm.yaml` | Linux package metadata for [nfpm](https://github.com/goreleaser/nfpm); version injected by Makefile |
| `.github/workflows/release.yml` | On push of tag `v*`, verifies tag matches `cmd/sshui/main.go` version, runs `make packages`, uploads deb/rpm/apk/tarballs + `SHA256SUMS` to GitHub Releases |
| `README.md` | User-facing install/run and config table |

**Imports:** Always use the module path `github.com/sacckth/sshui/...` for internal packages.

## Architecture (data flow)

```text
Startup:
  config.toml missing → setup wizard (writes config.toml, ssh_hosts, main ssh_config)
  ensure_include (default on) → AppendInclude: "#sshui-managed / Include <ssh_hosts>" at top of main ssh_config
  ssh_hosts (managed) → config.Parse → m.cfg   (editable; HasInclude → MergeIncludes, read-only)
  ssh_config (parent) → config.Parse → StripBridgeIncludes → m.mainCfg (read-only snapshot)
  password_hosts.toml → overlay.Load → m.overlayData

User edits (managed hosts only):
  lists, text inputs, raw editor → m.cfg mutations → dirty flag
  first mutation → edit mode: lockfile.Acquire on ssh_hosts + overlay

Save (w):  ssh -G check (refuse once) → backup prior bytes → config.Write → ssh_hosts path only
Import from main: clone HostBlock → m.cfg → save ssh_hosts
                  optional: RemoveHostStanza cuts that stanza from the main file's text (with backup)
Reload: ReadFile + Parse both files (discards unsaved model)
```

- **Parser** (`internal/config/parse.go`): Builds `DefaultHosts` + `Groups` with `#@group:`, `#@desc:`, `#@info:` on groups and `#@host:` lines attached to the **following** `Host` stanza (`HostBlock.HostComments`, as in sshclick). Accepts `Key value` and `Key=value`. Records `HostBlock.SrcLine` (1-based line of the `Host` keyword; 0 for synthetic blocks). Directives that appear before any `Host` in a section are wrapped in a synthetic `Host *` block **in that section** (order preserved). `Match` lines are kept as directives of the preceding block. Sets `HasInclude` if any `Include` directive appears. `Parse` only fails on I/O/scanner errors, so it is not a validity check. **`MergeIncludes`** (`include.go`) appends synthetic `include:<basename>` groups for merged browse when the TUI is read-only.
- **Include resolution:** Relative `Include` paths resolve against `UserIncludeBaseDir()` (`~/.ssh`), for nested includes too, as in ssh_config(5), not against the including file's directory.
- **`StripBridgeIncludes`** (`include.go`): Removes `HostBlock`s whose only directives are `Include` pointing at a given absolute file (managed `ssh_hosts` and/or the main `ssh_config` when exporting). Callers pass `UserIncludeBaseDir()` as `resolveBaseDir`. The result is for display only: never serialize `m.mainCfg` back to disk (it lacks the bridge Include and all plain comments).
- **`RemoveHostStanza`** (`remove.go`): Removes one stanza (plus the `#@host:` lines above it) from raw file text by `SrcLine`, leaving every other byte unchanged; errors if that line no longer matches the expected patterns.
- **Writer** (`internal/config/write.go`): Emits `Host` blocks and group banners; stable spacing; ends with newline.
- **TUI** (`internal/tui/`): Pointer receiver `*Model` (except `View`); `Options` carries theme, editor, paths, overlay and main config from `cmd/sshui`.
- **ssh/sftp:** OpenSSH hosts run `ssh`/`sftp [-F main] <first pattern>` (`openSSHCommand`; `-F` only when the main config is not `~/.ssh/config`). Password-overlay hosts connect by overlay index with `SSH_ASKPASS` env; an overlay pattern matching an OpenSSH alias only adds that env.

### Dual-tree model

When `MainSSHConfigPath ≠ ssh_hosts_path` and the main file is readable, the TUI model holds **two** `*Config` values:

| Field | Source | Editable | Save target |
|-------|--------|----------|-------------|
| `m.cfg` | `ssh_hosts` | Yes | `ssh_hosts_path` |
| `m.mainCfg` | `ssh_config` (stripped) | No (import only) | `MainSSHConfigPath` (only during import-with-delete) |

`HostRef` carries a `FromMain bool` discriminator. The helper methods `resolveHostAt`, `resolveValidateRef`, and `hostConfig` on `*Model` dispatch to the correct `*Config`. All mutation paths check `mainRefBlocked()` (via `editModeBlocked`) and refuse edits on `FromMain` refs with a status message directing to the Import action.

## Critical behaviors (do not break without intent)

1. **Save contract:** Normal save (`w`) writes only `ssh_hosts_path` (plus `ssh_config_git_mirror` if set). Every other write, with backup-before-write where the file has content:
   - **Main `ssh_config`:** startup `AppendInclude` (when `ensure_include` is on and the `#sshui-managed` marker is missing); the setup wizard (`AppendInclude`, or `ReplaceMainSSHConfigWithManagedInclude` when moving all hosts); Import-with-delete after explicit confirmation, which edits text via `RemoveHostStanza` and must **never** re-serialize `m.mainCfg`.
   - **`password_hosts.toml`:** saved immediately on every overlay edit (no dirty flag).
   - **`config.toml`:** written by the wizard and by `B` (browse mode is persisted).
2. **Save backup:** Before overwriting any file, its **previous** bytes are written to a hidden sibling file: `hiddenBackupPath` → `.<basename>.bkp` in the same directory (e.g. `~/.ssh/config` → `~/.ssh/.config.bkp`). Mode `0600`. If backup write fails, save aborts. Only one backup generation is kept.
3. **New file:** If the config path does not exist yet, no backup is created; save creates the file.
4. **Save check:** Interactive saves go through `saveChecked`: `ssh -G -F <tmp>` must accept the serialized file, otherwise the save is refused once and the same content saves on the next `w` (`saveOverride`). Missing `ssh` or a 3s timeout never blocks saving. Programmatic saves (import) call `save` directly.
5. **Raw editor (`v`):** Serializes **in-memory** `*Config` to a temp file, runs `sh -c '$editor $quotedPath'` (editor from appcfg → `VISUAL` → `EDITOR` → `vi`), and replaces the model only if the editor exits 0 **and** the buffer changed (`tea.ExecProcess`).
6. **Dirty flag:** Cleared on successful save and on reload from disk. All quit paths (`q`, ctrl+c) go through `requestQuit`, which asks when dirty (`y`/second ctrl+c quit, `w` saves then quits) and releases locks.
7. **Main config hosts are read-only:** Mutations (delete, move, rename, edit directives, duplicate, reorder) are blocked on `FromMain` refs. The only way to adopt them is the Import action (A menu).
8. **Edit mode and locks:** sshui starts read-only; the first mutation calls `enterEditMode`, which takes `.sshui.swp` locks on `ssh_hosts` and the overlay. Esc in host detail leaves edit mode (releases locks) before leaving the view.

## CLI and configuration precedence

Order for the **main** SSH config path (`MainSSHConfigPath`, shown read-only beside the managed hosts):

1. `--config`  
2. `$SSH_CONFIG`  
3. `ssh_config` in app TOML (`internal/appcfg`)  
4. `~/.ssh/config`  

The **managed** file is `[hosts] ssh_hosts_path` (default `os.UserConfigDir()/sshui/ssh_hosts`); `--config` does not change it. The `list`, `show` and `dump` subcommands read only the managed file and ignore `--config`.

App settings file path: `internal/appcfg.FilePath()` → `os.UserConfigDir()/sshui/config.toml` (on macOS, `~/Library/Application Support/sshui/`).

## Dependencies (direct)

- `github.com/charmbracelet/bubbletea` — TUI runtime  
- `github.com/charmbracelet/bubbles` — list, textinput, textarea, viewport, key helpers  
- `github.com/atotto/clipboard` — "copy ssh command" action  
- `github.com/charmbracelet/lipgloss` — styles / themes  
- `github.com/spf13/cobra` — CLI  
- `github.com/pelletier/go-toml/v2` — app settings  

No CGO; cross-compilation is straightforward (`make dist`).

## Commands agents should run

```bash
go build -o sshui ./cmd/sshui/
go test ./...
go fmt ./...
```

After substantive edits, run `go test ./...` at minimum. There is no CI on pull requests (the only workflow runs on `v*` tags), so Dependabot PRs and branches are untested until you run the suite locally.

## Coding conventions

- **Scope:** Prefer minimal diffs; match existing naming and package boundaries (`internal/*` is not importable by external modules).
- **Config layer:** Parser and writer live together; extend the model in `model.go`, then parse + write + golden/round-trip tests in `config_test.go`. `StripBridgeIncludes` and related helpers live in `include.go`.
- **TUI layer:** New screens or modes need updates to `Update`, `View`, and the `?` help string in `app.go`. Use existing Lip Gloss style vars after `applyTheme`. Host access in the TUI should go through `resolveHostAt`/`resolveValidateRef` (not `m.cfg.HostAt` directly) to support the dual-config model.
- **Errors:** Return wrapped errors from save/load paths; surface user-readable strings via `errStyle` in the TUI status line.
- **Version:** CLI version string is in `cmd/sshui/main.go` (`version` var).

## Testing strategy

- **config:** Round-trip `Parse` → `String` → `Parse`; `Include` sets `HasInclude`; bridge stripping and Include cycles (`include_helper_test.go`, absolute paths only); relative Include resolution (`TestMergeIncludesRelativeToUserSSHDir`, sets `HOME` because relative paths resolve under `~/.ssh`); `RemoveHostStanza` golden tests (`remove_test.go`).  
- **appcfg:** Missing file yields empty config; valid TOML populates fields (tests set `HOME` / `XDG_CONFIG_HOME` appropriately for OS).  
- **overlay / lockfile / cmd/sshui:** unit tests per package.  
- **tui:** Drive the model with `m.Update(tea.KeyMsg{...})` against temp files (`model_test.go`); `effective_test.go` runs the real `ssh -G` and skips when `ssh` is missing. Never call `finishSSHCopy` or other clipboard paths from tests (they overwrite the user's clipboard); test `sshCopyCommand` instead.

## Known limitations / future work

- **Include:** Write-back still targets only the managed file; merged included files are read-only in the UI.
- **Comments:** Non-metadata `#` lines are not preserved through parse/write of the managed file.
- **Match blocks:** Stored as directives of the preceding `Host` block, so moving or deleting that host carries the `Match` section with it.
- **`ssh -G` side effects:** The Overview tab and save check run `ssh -G`, which evaluates `Match exec` commands in the user's config (bounded by a 3s timeout).
- **Writes:** Files are written in place (`os.WriteFile`), not via temp file + rename.
- **Host ref validity:** After raw editor or destructive edits, `ValidateRef` may force navigation back to tree.
- **Import granularity:** Import currently places the host into the managed default group; choosing a target group during import is not yet implemented.
- **Main config groups in tree:** Groups from the parent `ssh_config` are displayed with a 🔒 suffix in a dimmer color and cannot be renamed/deleted from within sshui.

## Commit and PR expectations

- Use clear commit messages (full sentences for non-trivial changes).  
- Keep README user-focused; put agent/architecture detail here in `AGENTS.md`.

## Security note

The app edits SSH config and runs a user-configurable shell editor command. Treat app TOML `editor` and file paths as trusted user input.
