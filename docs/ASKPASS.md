# SSH_ASKPASS integration with sshui

sshui's password overlay (`password_hosts.toml`) connects to password-authenticated
hosts by setting `SSH_ASKPASS` in the child `ssh` process. The askpass program must
print the password to stdout and exit 0 — no TTY interaction.

## How it works

```text
sshui → exec ssh -p PORT user@host
         env: SSH_ASKPASS=/path/to/script
              SSH_ASKPASS_REQUIRE=force
         ssh calls the askpass script instead of prompting
```

`SSH_ASKPASS_REQUIRE=force` (OpenSSH 8.4+) tells ssh to always use the script,
even when a TTY is available. On older versions, unset `DISPLAY` or set it to
a dummy value and ensure no TTY is allocated.

## Overlay entry example

```toml
[[password_host]]
patterns = ["legacy-box"]
hostname = "192.168.50.10"
user = "admin"
port = 22
askpass = "~/.local/bin/ssh-askpass-pass"
askpass_require = "force"
```

The `askpass` value must be an executable that prints the password to stdout.
Below are recipes for popular open-source CLI secret stores.

---

## 1. pass (password-store)

[https://www.passwordstore.org/](https://www.passwordstore.org/)

GPG-encrypted file tree under `~/.password-store`.

**Install:**

```bash
# Debian/Ubuntu
sudo apt install pass
# macOS
brew install pass
```

**Store a password:**

```bash
pass insert ssh/legacy-box
```

**Retrieve (stdout, no newline issues):**

```bash
pass show ssh/legacy-box
```

**Wrapper script** (`~/.local/bin/ssh-askpass-pass`):

```bash
#!/bin/sh
exec pass show ssh/legacy-box 2>/dev/null | head -1
```

```bash
chmod +x ~/.local/bin/ssh-askpass-pass
```

**Security notes:** `pass show` decrypts via GPG agent; if the agent is locked,
it will prompt for the GPG passphrase (on a TTY or via pinentry). The password
briefly exists in process memory and on stdout. Avoid piping to files or logging.

---

## 2. gopass

[https://github.com/gopasspw/gopass](https://github.com/gopasspw/gopass)

Drop-in replacement for `pass` with team and multi-store features.

**Install:**

```bash
# macOS
brew install gopass
# Linux: see https://github.com/gopasspw/gopass/releases
```

**Store / retrieve:**

```bash
gopass insert ssh/legacy-box
gopass show -o ssh/legacy-box   # -o prints only the first line
```

**Wrapper script:**

```bash
#!/bin/sh
exec gopass show -o ssh/legacy-box 2>/dev/null
```

---

## 3. KeePassXC CLI

[https://keepassxc.org/](https://keepassxc.org/)

Desktop password manager with a CLI (`keepassxc-cli`).

**Install:**

```bash
# macOS
brew install --cask keepassxc
# Debian/Ubuntu
sudo apt install keepassxc
```

**Store a password (in a database):**

```bash
keepassxc-cli add -u admin ~/Passwords.kdbx ssh/legacy-box
```

**Retrieve:**

```bash
keepassxc-cli show -sa password ~/Passwords.kdbx ssh/legacy-box
```

The `-s` flag reads the database password from stdin (pipe it from another source
or use the `KEEPASSXC_CLI_PASSWORD` env var if available).

**Wrapper script:**

```bash
#!/bin/sh
echo "$KEEPASS_DB_PASS" | keepassxc-cli show -sa password ~/Passwords.kdbx ssh/legacy-box 2>/dev/null
```

**Security notes:** The database master password must be available non-interactively.
Consider using a key file (`-k`) or the Secret Service integration for auto-unlock.

---

## 4. secret-tool (libsecret / Secret Service API)

[https://wiki.gnome.org/Projects/Libsecret](https://wiki.gnome.org/Projects/Libsecret)

Stores secrets in the desktop keyring (GNOME Keyring, KDE Wallet, etc.).
Linux-only; requires a running Secret Service daemon.

**Install:**

```bash
# Debian/Ubuntu
sudo apt install libsecret-tools
```

**Store:**

```bash
secret-tool store --label="SSH legacy-box" service ssh host legacy-box
```

**Retrieve:**

```bash
secret-tool lookup service ssh host legacy-box
```

**Wrapper script:**

```bash
#!/bin/sh
exec secret-tool lookup service ssh host legacy-box 2>/dev/null
```

**Security notes:** Secrets are stored in the session keyring, unlocked when you
log in. No GPG or master password is needed while the session is active.

---

## 5. age (file encryption)

[https://github.com/FiloSottile/age](https://github.com/FiloSottile/age)

Encrypts files with public-key or passphrase-based encryption. No vault daemon —
just an encrypted file on disk.

**Install:**

```bash
# macOS
brew install age
# Go
go install filippo.io/age/cmd/...@latest
```

**Encrypt a password:**

```bash
echo "mysecretpassword" | age -R ~/.ssh/id_ed25519.pub -o ~/.secrets/legacy-box.age
```

**Decrypt (stdout):**

```bash
age -d -i ~/.ssh/id_ed25519 ~/.secrets/legacy-box.age
```

**Wrapper script:**

```bash
#!/bin/sh
exec age -d -i ~/.ssh/id_ed25519 ~/.secrets/legacy-box.age 2>/dev/null
```

**Security notes:** The SSH key used for decryption must be available (unlocked
agent or unencrypted key). The password is briefly on stdout. No daemon required.

---

## Optional: Bitwarden CLI (bw)

Bitwarden is popular but not fully open-source (server is source-available).

```bash
bw get password "ssh/legacy-box"
```

Requires `bw login` and `bw unlock` first; the session token must be in
`BW_SESSION`. See [Bitwarden CLI docs](https://bitwarden.com/help/cli/).

## Optional: 1Password CLI (op)

Proprietary. See [1Password CLI docs](https://developer.1password.com/docs/cli/).

```bash
op read "op://vault/ssh-legacy-box/password"
```

---

## General security considerations

- **stdout exposure:** The askpass script prints the password to a pipe read by ssh.
  Avoid logging or redirecting this output.
- **Shell history:** Store passwords via dedicated CLI commands, not `echo "pass" | ...`.
- **Process visibility:** On multi-user systems, `/proc/PID/environ` may expose
  `SSH_ASKPASS` paths (but not the password itself).
- **macOS:** `SSH_ASKPASS_REQUIRE=force` requires OpenSSH 8.4+. macOS ships an
  older version; install a newer one via Homebrew (`brew install openssh`).
- **DISPLAY:** Some older ssh builds require `DISPLAY` to be set (even to `:0`)
  for `SSH_ASKPASS` to be invoked. `SSH_ASKPASS_REQUIRE=force` bypasses this.
