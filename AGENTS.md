# AGENTS.md — sshui

Guidance for humans and coding agents working on this repository.

## Project summary

**sshui** is a terminal UI for editing a **single** OpenSSH client `ssh_config` file. It parses the file into an in-memory model, lets the user edit hosts and directives in a Bubble Tea UI, then serializes the model back to disk. Saving **reformats** the entire file (deterministic layout); it does not preserve arbitrary comments except sshclick-style group metadata.

- **Upstream:** https://github.com/sacckth/sshui  
- **Go module:** `github.com/sacckth/sshui`  
- **Language / version:** Go 1.25+  
- **License:** BSD 3-Clause (`LICENSE`)

## Repository layout

| Path | Role |
|------|------|
| `cmd/sshui/main.go` | CLI entry: Cobra, config path resolution, parse file, start `tea.Program` |
| `internal/config/` | Domain types, **custom** line-oriented parser, **custom** writer (`Write` / `String`) |
| `internal/appcfg/` | Optional TOML: `~/.config/sshui/config.toml` (see README for macOS path) |
| `internal/sshkeywords/` | Static OpenSSH client keyword catalog for picker UX (not authoritative for OpenSSH) |
| `internal/tui/` | Bubble Tea model: tree, host detail, directive picker, inputs, raw editor, save, themes |
| `Makefile` | `build`, `test`, `dist`, `packages` (linux: deb/rpm/apk via nfpm; darwin-arm64 tarball) |
| `nfpm.yaml` | Linux package metadata for [nfpm](https://github.com/goreleaser/nfpm); version injected by Makefile |
| `.github/workflows/release.yml` | On push of tag `v*`, verifies tag matches `cmd/sshui/main.go` version, runs `make packages`, uploads deb/rpm/apk/tarballs + `SHA256SUMS` to GitHub Releases |
| `README.md` | User-facing install/run and config table |

**Imports:** Always use the module path `github.com/sacckth/sshui/...` for internal packages.

## Architecture (data flow)

```text
Read file → config.Parse → *Config in TUI Model
                ↓
User edits (lists, text inputs, raw editor)
                ↓
Save: backup prior bytes → config.Write → same path
Reload: ReadFile + Parse (discards unsaved model)
```

- **Parser** (`internal/config/parse.go`): Builds `DefaultHosts` + `Groups` with `#@group:`, `#@desc:`, `#@info:` on groups and `#@host:` lines attached to the **following** `Host` stanza (`HostBlock.HostComments`). Sets `HasInclude` if any `Include` directive appears. **`MergeIncludes`** (`include.go`) appends synthetic `include:<basename>` groups for merged browse when the TUI is read-only.
- **Writer** (`internal/config/write.go`): Emits `Host` blocks and group banners; stable spacing; ends with newline.
- **TUI** (`internal/tui/app.go` + `raweditor.go` + `theme.go`): Pointer receiver `*Model`; `Options` carries theme + default editor hint from appcfg.

## Critical behaviors (do not break without intent)

1. **Single-file contract:** Only the resolved absolute path opened at startup is read/written. Included files are not merged.
2. **Save backup:** Before overwriting the main file, if it already exists, its **previous** bytes are written to a hidden sibling file: `hiddenBackupPath` → `.<basename>.bkp` in the same directory (e.g. `~/.ssh/config` → `~/.ssh/.config.bkp`). Mode `0600`. If backup write fails, save aborts.
3. **New file:** If the config path does not exist yet, no backup is created; save creates the file.
4. **Raw editor (`v`):** Serializes **in-memory** `*Config` to a temp file, runs `sh -c '$editor $quotedPath'` (editor from appcfg → `VISUAL` → `EDITOR` → `vi`), then replaces the model only if `Parse` succeeds (`tea.ExecProcess`).
5. **Dirty flag:** Cleared on successful save and on reload from disk.

## CLI and configuration precedence

Order for SSH config path:

1. `--config`  
2. `$SSH_CONFIG`  
3. `ssh_config` in app TOML (`internal/appcfg`)  
4. `~/.ssh/config`  

App settings file path: `internal/appcfg.FilePath()` → `os.UserConfigDir()/sshui/config.toml`.

## Dependencies (direct)

- `github.com/charmbracelet/bubbletea` — TUI runtime  
- `github.com/charmbracelet/bubbles` — list, textinput, key helpers  
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

After substantive edits, run `go test ./...` at minimum (`internal/config` and `internal/appcfg` have tests; `internal/tui` includes `save_test.go`).

## Coding conventions

- **Scope:** Prefer minimal diffs; match existing naming and package boundaries (`internal/*` is not importable by external modules).
- **Config layer:** Parser and writer live together; extend the model in `model.go`, then parse + write + golden/round-trip tests in `config_test.go`.
- **TUI layer:** New screens or modes need updates to `Update`, `View`, and the `?` help string in `app.go`. Use existing Lip Gloss style vars after `applyTheme`.
- **Errors:** Return wrapped errors from save/load paths; surface user-readable strings via `errStyle` in the TUI status line.
- **Version:** CLI version string is in `cmd/sshui/main.go` (`version` var).

## Testing strategy

- **config:** Round-trip `Parse` → `String` → `Parse`; `Include` sets `HasInclude`.  
- **appcfg:** Missing file yields empty config; valid TOML populates fields (tests set `HOME` / `XDG_CONFIG_HOME` appropriately for OS).  
- **tui:** `hiddenBackupPath` unit test; interactive behavior is manual.

## Known limitations / future work

- **Include:** Write-back still targets only the primary file; merged included files are read-only in the UI.  
- **Comments:** Non-metadata `#` lines are not preserved through parse/write.  
- **Host ref validity:** After raw editor or destructive edits, `ValidateRef` may force navigation back to tree.

## Commit and PR expectations

- Use clear commit messages (full sentences for non-trivial changes).  
- Keep README user-focused; put agent/architecture detail here in `AGENTS.md`.

## Security note

The app edits SSH config and runs a user-configurable shell editor command. Treat app TOML `editor` and file paths as trusted user input.
