package tui

import (
	"fmt"
	"strings"

	scfg "github.com/sacckth/sshui/internal/config"
)

// sshWildKind describes how to build the ssh/sftp target from user input for a wildcard Host pattern.
type sshWildKind int

const (
	sshWildNone       sshWildKind = iota
	sshWildSuffixStar             // win*  → fixed prefix "win", user types suffix
	sshWildPrefixStar             // *srv → user types prefix, fixed suffix "srv"
	sshWildManual                 // user types full hostname (complex / ? / ! patterns)
)

type sshPostWildKind int

const (
	postWildNone sshPostWildKind = iota
	postWildConnectSSH
	postWildConnectSFTP
	postWildCopyCmd
)

// sshConnectPlan describes how to connect for the first Host pattern on a block.
type sshConnectPlan struct {
	FirstPattern string
	PatternCount int
	DirectTarget string // non-empty when no wildcard UI is needed
	NeedWildcard bool
	WildKind     sshWildKind
	LitPrefix    string
	LitSuffix    string
	Invalid      bool
}

func sshConnectPlanFromHost(h *scfg.HostBlock) sshConnectPlan {
	if len(h.Patterns) == 0 {
		return sshConnectPlan{Invalid: true}
	}
	first := strings.TrimSpace(h.Patterns[0])
	p := sshConnectPlan{
		FirstPattern: first,
		PatternCount: len(h.Patterns),
	}
	if first == "" {
		p.Invalid = true
		return p
	}
	if !strings.ContainsAny(first, "*?!") {
		p.DirectTarget = first
		return p
	}
	if strings.HasPrefix(first, "!") {
		p.NeedWildcard = true
		p.WildKind = sshWildManual
		return p
	}
	if strings.ContainsRune(first, '?') {
		p.NeedWildcard = true
		p.WildKind = sshWildManual
		return p
	}
	nStar := strings.Count(first, "*")
	if nStar == 1 {
		i := strings.IndexByte(first, '*')
		if i == 0 {
			p.NeedWildcard = true
			p.WildKind = sshWildPrefixStar
			p.LitSuffix = first[1:]
			return p
		}
		if i == len(first)-1 {
			p.NeedWildcard = true
			p.WildKind = sshWildSuffixStar
			p.LitPrefix = first[:i]
			return p
		}
	}
	p.NeedWildcard = true
	p.WildKind = sshWildManual
	return p
}

func composeSSHWildcardTarget(kind sshWildKind, litPrefix, litSuffix, user string) string {
	u := strings.TrimSpace(user)
	switch kind {
	case sshWildSuffixStar:
		return litPrefix + u
	case sshWildPrefixStar:
		return u + litSuffix
	case sshWildManual:
		return u
	default:
		return u
	}
}

func sshConnectMultiPatternNote(p sshConnectPlan) string {
	if p.PatternCount <= 1 {
		return ""
	}
	return fmt.Sprintf("This host entry has %d patterns; using the first (%q).", p.PatternCount, p.FirstPattern)
}

func sshWildcardValuePlaceholder(kind sshWildKind) string {
	switch kind {
	case sshWildSuffixStar:
		return "text after fixed prefix (may be empty)"
	case sshWildPrefixStar:
		return "text before fixed suffix (required)"
	case sshWildManual:
		return "full hostname for ssh"
	default:
		return ""
	}
}
