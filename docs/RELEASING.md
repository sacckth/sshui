# Releasing sshui

## Prerequisites

- Go 1.25+ installed
- Push access to the repository
- Clean working tree (or `ALLOW_DIRTY=1`)

## Steps

1. **Set the version** in `cmd/sshui/main.go`:

   ```go
   version = "x.y.z"
   ```

2. **Commit** the version bump and push to `main`.

3. **Create and push the tag:**

   ```bash
   make tag-push
   ```

   This creates an annotated tag `vx.y.z` and pushes it to origin. The tag name must match the `version` string in `main.go` (without the `v` prefix).

4. **CI builds the release.** The [release workflow](.github/workflows/release.yml) triggers on tags matching `v*`. It:
   - Verifies the tag matches `version` in `main.go`
   - Runs `go test ./...`
   - Runs `make packages` (produces `.deb`, `.rpm`, `.apk`, tarballs)
   - Writes `dist/SHA256SUMS`
   - Publishes a GitHub Release with all artifacts

5. **Verify** at [github.com/sacckth/sshui/releases](https://github.com/sacckth/sshui/releases).

## Artifacts produced

| File | Description |
|------|-------------|
| `sshui_x.y.z_amd64.deb` | Debian/Ubuntu package |
| `sshui-x.y.z-1.x86_64.rpm` | RHEL/Fedora package |
| `sshui_x.y.z_x86_64.apk` | Alpine package |
| `sshui-x.y.z-linux-amd64.tar.gz` | Linux binary tarball |
| `sshui-x.y.z-darwin-arm64.tar.gz` | macOS Apple Silicon tarball |
| `SHA256SUMS` | Checksums for all artifacts |

## Local packaging (without CI)

```bash
make packages                      # all artifacts in dist/
make packages VERSION=1.0.0        # override version
make pkg-deb                       # just .deb
make clean                         # remove dist/ and binary
```

Run `make help` for all available targets.

## Troubleshooting

- **Version mismatch:** CI fails if the tag doesn't match `main.go`. Fix `main.go`, delete the bad tag (`git tag -d vx.y.z && git push origin :refs/tags/vx.y.z`), and re-tag.
- **Dirty tree:** `make tag-push` refuses to tag with uncommitted changes. Commit or stash first, or use `ALLOW_DIRTY=1 make tag-push`.
