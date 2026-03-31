package config

import (
	"bufio"
	"io"
	"strings"
	"unicode"
)

// Parse reads an OpenSSH client config from r into Config.
// sshclick-style lines #@group:, #@desc:, #@info:, #@host:, #@fold: (after #@group:),
// and #@default-fold: (before default Host stanzas) are interpreted; other comment lines
// outside Host stanzas are ignored. HasInclude is set if any Include directive appears;
// caller should warn or merge.
func Parse(r io.Reader) (*Config, error) {
	cfg := &Config{}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var currentGroup *Group
	var currentHost *HostBlock
	var pendingHostMeta []string

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
				case metaDesc, metaInfo:
					if currentGroup != nil {
						currentGroup.Descriptions = append(currentGroup.Descriptions, trimmed)
					}
				case metaHostTag:
					pendingHostMeta = append(pendingHostMeta, trimmed)
				case metaFold:
					if currentGroup != nil {
						currentGroup.CollapsedByDefault = parseFoldPayload(meta.payload)
					}
				case metaDefaultFold:
					if currentGroup == nil {
						cfg.DefaultHostsCollapsed = parseFoldPayload(meta.payload)
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
			hb := HostBlock{
				HostComments: append([]string(nil), pendingHostMeta...),
				Patterns:     patterns,
				Directives:   nil,
			}
			pendingHostMeta = nil
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
			hb := HostBlock{
				HostComments: append([]string(nil), pendingHostMeta...),
				Patterns:     []string{"*"},
				Directives:   nil,
			}
			pendingHostMeta = nil
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
	metaFold
	metaDefaultFold
)

type meta struct {
	kind    metaKind
	payload string
}

func parseMetaComment(trimmed string) (meta, bool) {
	rest := strings.TrimSpace(strings.TrimPrefix(trimmed, "#"))
	switch {
	case strings.HasPrefix(rest, "@group:"):
		return meta{kind: metaGroup, payload: strings.TrimSpace(strings.TrimPrefix(rest, "@group:"))}, true
	case strings.HasPrefix(rest, "group:"):
		return meta{kind: metaGroup, payload: strings.TrimSpace(strings.TrimPrefix(rest, "group:"))}, true
	case strings.HasPrefix(rest, "@desc:"), strings.HasPrefix(rest, "desc:"):
		return meta{kind: metaDesc}, true
	case strings.HasPrefix(rest, "@info:"), strings.HasPrefix(rest, "info:"):
		return meta{kind: metaInfo}, true
	case strings.HasPrefix(rest, "@host:"), strings.HasPrefix(rest, "host:"):
		return meta{kind: metaHostTag}, true
	case strings.HasPrefix(rest, "@fold:"):
		return meta{kind: metaFold, payload: strings.TrimSpace(strings.TrimPrefix(rest, "@fold:"))}, true
	case strings.HasPrefix(rest, "fold:"):
		return meta{kind: metaFold, payload: strings.TrimSpace(strings.TrimPrefix(rest, "fold:"))}, true
	case strings.HasPrefix(rest, "@default-fold:"):
		return meta{kind: metaDefaultFold, payload: strings.TrimSpace(strings.TrimPrefix(rest, "@default-fold:"))}, true
	case strings.HasPrefix(rest, "default-fold:"):
		return meta{kind: metaDefaultFold, payload: strings.TrimSpace(strings.TrimPrefix(rest, "default-fold:"))}, true
	default:
		return meta{}, false
	}
}

// parseFoldPayload returns true when the fold metadata means "start collapsed".
func parseFoldPayload(payload string) bool {
	p := strings.ToLower(strings.TrimSpace(payload))
	switch p {
	case "-", "collapsed", "fold", "folded", "yes", "y", "1", "true":
		return true
	default:
		return false
	}
}

func splitFields(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool {
		return unicode.IsSpace(r)
	})
}
