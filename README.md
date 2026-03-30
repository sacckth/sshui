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

Set `NO_COLOR=1` to disable ANSI colors in the TUI (pane backgrounds and list chrome are subdued).

In the TUI, **`$`** opens this file in `$EDITOR` and **`&`** shows it in a read-only scroller (after editing, theme/editor/mirror settings reload when the editor exits).

## Features

- Split panes with distinct backgrounds, a vertical separator, and a footer rule; group headers use bold+italic styling; **`z`** folds a group when its header row is selected (filtering temporarily shows hosts under folded groups)
- Split host detail: tree + Overview / all directives / Connectivity tabs; `tab` switches pane focus
- sshclick-style `#@group:` / `#@desc:` / `#@info:` / `#@host:` (per-host comments before `Host`)
- Actions menu (`A`): `ssh`, `sftp`, copy `ssh <alias>` (single non-wildcard pattern)
- **`Include`:** If the opened file contains `Include`, sshui starts in **read-only merged** mode: it loads that file plus every matched include and shows extra `include:<filename>` groups so you can browse everything in one tree. **Saving is disabled** (one save could not update every file). Press **`W`** in the TUI to switch to a **writable** view of **only** the main file (included hosts are hidden); **`s`** still writes that path only. Press **`W`** again (no unsaved changes) or **`r`** (reload) to return to merged read-only browse if `Include` is still there. See **`?`** in the TUI for the full note.
- Optional git/dotfiles mirror on save (`ssh_config_git_mirror`)
- CLI: `list`, `show`, `dump` (`--json`, `--check`), `completion bash|zsh|fish`

## Build

```bash
go build -o sshui ./cmd/sshui/
```

To install on your `PATH` (Go 1.17+): `go install github.com/sacckth/sshui/cmd/sshui@latest` (or the same path from a local clone).

**Makefile:** run `make` or `make help` for targets (`build`, `test`, `dist`, `packages`, `tag-push`, …).

Cross-compile: `make dist` (Darwin arm64 + Linux amd64).

**Packages:** `make packages` builds a static **linux/amd64** binary and produces **`.deb`**, **`.rpm`**, and **`.apk`** under `dist/` (via [nfpm](https://github.com/goreleaser/nfpm)), plus **`sshui-<version>-linux-amd64.tar.gz`** and **`sshui-<version>-darwin-arm64.tar.gz`**. Override version with `make packages VERSION=1.0.0`. Requires Go only (nfpm is run with `go run`).

### GitHub Releases (where builds appear)

Official binaries are **not** uploaded by `make` alone; they show up under the repo’s **Releases** tab after CI runs.

1. Set `version = "x.y.z"` in `cmd/sshui/main.go` and commit (this string must match the tag, see below).
2. Push to `main` (or your default branch) so the commit is on GitHub.
3. Run **`make tag-push`** (creates annotated tag `vx.y.z` and `git push origin vx.y.z`). Requires a clean working tree unless `ALLOW_DIRTY=1`.
4. The [Release workflow](.github/workflows/release.yml) starts on **`push` of tags `v*`**. It checks that **`vx.y.z` without the `v`** equals `version` in `cmd/sshui/main.go`, runs tests, runs **`make packages`**, writes **`dist/SHA256SUMS`**, and publishes a **GitHub Release** with the `.deb`, `.rpm`, `.apk`, tarballs, and checksum file.

If the workflow fails the version check, fix `main.go` or delete the bad tag and tag again. After success, open **[github.com/sacckth/sshui/releases](https://github.com/sacckth/sshui/releases)** and pick the new tag to download assets.

## Run

Examples assume `sshui` is on your `PATH` (after `go install`, a package install, or `cp`/`mv` from `go build -o sshui`).

```bash
sshui
sshui --config /path/to/config
sshui list
sshui show myhost --json
sshui dump --check
```

Press `?` in the TUI for keybindings.

## Screenshots

Illustrative renders (fictional hosts/users/addresses). To try the real TUI with the same synthetic file used for docs:

```bash
sshui --config "$(pwd)/docs/readme-demo.conf"
```

![Browse hosts and directive preview](docs/screenshots/browse.png)

*Browse: host tree (left) and directive preview (right).*

![Host detail split view](docs/screenshots/detail.png)

*Detail: split view with directive list.*

![Filtering the host list](docs/screenshots/filter.png)

*Filter mode narrows rows by the visible Host column.*

## Help

```bash
sshui --help
sshui completion bash  # pipe to your shell rc
```
