package tui

import (
	"os/exec"
	"runtime"
)

// execSSHOnTTY returns a command that runs ssh or sftp. On Unix, the program is
// run under sh so that after it exits we write a short reset sequence to the
// controlling TTY (/dev/tty) before Bubble Tea repaints.
//
// Embedded SSH/SFTP can leave SGR attributes and terminal modes set by the remote
// session; Bubble Tea’s RestoreTerminal does not undo all of that, which can
// garble the next full-screen draw. We deliberately use a soft reset only:
//   - \x1b[m  — SGR reset (default colors/attributes)
//   - \x1b[!p — DECSTR, soft terminal reset (many modes/state without full hardware reset)
//
// We do not send RIS (ESC c, \x1bc) here: it is a full reset that can clear scrollback or
// feel disruptive on common emulators, while DECSTR + SGR is enough for typical
// modern terminals (iTerm, Kitty, VTE, Terminal.app, etc.).
//
// On Windows, the command is returned unchanged (no POSIX sh wrapper).
func execSSHOnTTY(name string, arg ...string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		return exec.Command(name, arg...)
	}
	const reset = "\x1b[m\x1b[!p" // SGR reset + DECSTR (soft); see doc above re RIS.
	// Not exec "$@" — exec would replace sh and skip the printf. Preserve ssh's exit status.
	script := "ec=0; \"$@\" || ec=$?; printf '%s' '" + reset + "' >/dev/tty; exit $ec"
	return exec.Command("sh", append([]string{"-c", script, "_", name}, arg...)...)
}
