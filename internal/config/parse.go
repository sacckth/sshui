package config

import (
	"bufio"
	"io"
	"strings"
	"unicode"
)

// Parse reads an OpenSSH client config from r into Config.
// sshclick-style lines #@group:, #@desc:, #@info:, #@host: are interpreted;
// other comment lines outside Host stanzas are ignored. Phase 1: HasInclude is
// set if any Include directive appears; caller should warn.
func Parse(r io.Reader) (*Config, error) {
	cfg := &Config{}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var currentGroup *Group
	var currentHost *HostBlock

	ensureGroup := func(name string) *Group {
		name = strings.TrimSpace(name)
		for i := range cfg.Groups {
			if cfg.Groups[i].Name == name {
				return &cfg.Groups[i]
			}
		}
		cfg.Groups = append(cfg.Groups, Group{Name: name})
		return &cfg.Groups[len(cfg.Groups)-1]
	}

	for sc.Scan() {
		line := sc.Text()
		trimmed := strings.TrimSpace(line)

		if trimmed == "" {
			continue
		}

		if isIncludeLine(trimmed) {
			cfg.HasInclude = true
		}

		if strings.HasPrefix(trimmed, "#") {
			if meta, ok := parseMetaComment(trimmed); ok {
				switch meta.kind {
				case metaGroup:
					currentGroup = ensureGroup(meta.payload)
					currentHost = nil
				case metaDesc, metaInfo, metaHostTag:
					if currentGroup != nil {
						currentGroup.Descriptions = append(currentGroup.Descriptions, trimmed)
					}
				}
			}
			continue
		}

		fields := splitFields(trimmed)
		if len(fields) == 0 {
			continue
		}

		key := strings.ToLower(fields[0])
		if key == "host" {
			patterns := fields[1:]
			if len(patterns) == 0 {
				patterns = []string{""}
			}
			hb := HostBlock{Patterns: patterns, Directives: nil}
			if currentGroup == nil {
				cfg.DefaultHosts = append(cfg.DefaultHosts, hb)
				currentHost = &cfg.DefaultHosts[len(cfg.DefaultHosts)-1]
			} else {
				currentGroup.Hosts = append(currentGroup.Hosts, hb)
				currentHost = &currentGroup.Hosts[len(currentGroup.Hosts)-1]
			}
			continue
		}

		if currentHost == nil {
			// Global directives before any Host — treat as anonymous host "*" for safety
			hb := HostBlock{Patterns: []string{"*"}, Directives: nil}
			cfg.DefaultHosts = append(cfg.DefaultHosts, hb)
			currentHost = &cfg.DefaultHosts[len(cfg.DefaultHosts)-1]
		}

		d := Directive{Key: fields[0], Value: strings.TrimSpace(strings.TrimPrefix(trimmed, fields[0]))}
		currentHost.Directives = append(currentHost.Directives, d)
	}

	if err := sc.Err(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func isIncludeLine(trimmed string) bool {
	fields := splitFields(trimmed)
	if len(fields) == 0 {
		return false
	}
	return strings.EqualFold(fields[0], "Include")
}

type metaKind int

const (
	metaGroup metaKind = iota
	metaDesc
	metaInfo
	metaHostTag
)

type meta struct {
	kind    metaKind
	payload string
}

func parseMetaComment(trimmed string) (meta, bool) {
	rest := strings.TrimPrefix(trimmed, "#")
	rest = strings.TrimSpace(rest)
	switch {
	case strings.HasPrefix(rest, "@group:"):
		return meta{kind: metaGroup, payload: strings.TrimSpace(strings.TrimPrefix(rest, "@group:"))}, true
	case strings.HasPrefix(rest, "@desc:"):
		return meta{kind: metaDesc}, true
	case strings.HasPrefix(rest, "@info:"):
		return meta{kind: metaInfo}, true
	case strings.HasPrefix(rest, "@host:"):
		return meta{kind: metaHostTag}, true
	default:
		return meta{}, false
	}
}

func splitFields(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool {
		return unicode.IsSpace(r)
	})
}
