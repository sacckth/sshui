package config

import (
	"fmt"
	"strings"
)

// RemoveHostStanza returns src with the Host stanza whose Host keyword is on
// 1-based line hostLine removed, together with the #@host: lines directly above
// it. The stanza ends at its last directive before the next Host, Match, or
// metadata line; comments and blank lines after that directive are kept with
// whatever follows. All other bytes of src are unchanged.
//
// patterns must equal the patterns on that Host line, which guards against src
// having changed since it was parsed.
func RemoveHostStanza(src string, hostLine int, patterns []string) (string, error) {
	lines := strings.SplitAfter(src, "\n")
	if hostLine < 1 || hostLine > len(lines) {
		return "", fmt.Errorf("host line %d out of range", hostLine)
	}
	idx := hostLine - 1
	fields := splitFields(strings.TrimSpace(lines[idx]))
	if len(fields) == 0 || !strings.EqualFold(fields[0], "host") ||
		strings.Join(fields[1:], " ") != strings.Join(patterns, " ") {
		return "", fmt.Errorf("line %d is no longer \"Host %s\" (file changed on disk?)", hostLine, strings.Join(patterns, " "))
	}

	start := idx
	for start > 0 {
		m, ok := parseMetaComment(strings.TrimSpace(lines[start-1]))
		if !ok || m.kind != metaHostTag {
			break
		}
		start--
	}

	end := idx
	for j := idx + 1; j < len(lines); j++ {
		t := strings.TrimSpace(lines[j])
		if t == "" {
			continue
		}
		if strings.HasPrefix(t, "#") {
			if _, ok := parseMetaComment(t); ok || t == sshuiManagedMarker {
				break
			}
			continue
		}
		f := splitFields(t)
		if k := strings.ToLower(f[0]); k == "host" || k == "match" {
			break
		}
		end = j
	}

	out := append([]string(nil), lines[:start]...)
	rest := lines[end+1:]
	if len(rest) > 0 && strings.TrimSpace(rest[0]) == "" &&
		(len(out) == 0 || strings.TrimSpace(out[len(out)-1]) == "") {
		rest = rest[1:]
	}
	return strings.Join(append(out, rest...), ""), nil
}
