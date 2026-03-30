package config

import (
	"fmt"
	"strings"
)

// AddGroup appends a new empty named group. Name must be non-empty and not "(default)".
func (cfg *Config) AddGroup(name string) error {
	name = strings.TrimSpace(name)
	if name == "" || strings.EqualFold(name, "(default)") {
		return fmt.Errorf("invalid group name")
	}
	for i := range cfg.Groups {
		if cfg.Groups[i].Name == name {
			return fmt.Errorf("group %q already exists", name)
		}
	}
	cfg.Groups = append(cfg.Groups, Group{Name: name})
	return nil
}

// DeleteGroupByName removes a named group, moving all its hosts to DefaultHosts.
// "(default)" is not a stored group and cannot be deleted.
func (cfg *Config) DeleteGroupByName(name string) error {
	if name == "(default)" || strings.EqualFold(name, "(default)") {
		return fmt.Errorf("(default) cannot be deleted")
	}
	for i := range cfg.Groups {
		if cfg.Groups[i].Name != name {
			continue
		}
		cfg.DefaultHosts = append(cfg.DefaultHosts, cfg.Groups[i].Hosts...)
		cfg.Groups = append(cfg.Groups[:i], cfg.Groups[i+1:]...)
		return nil
	}
	return fmt.Errorf("group %q not found", name)
}

// RenameGroup changes Groups[groupIdx].Name; newName must be unique and not "(default)".
func (cfg *Config) RenameGroup(groupIdx int, newName string) error {
	if groupIdx < 0 || groupIdx >= len(cfg.Groups) {
		return fmt.Errorf("invalid group index")
	}
	newName = strings.TrimSpace(newName)
	if newName == "" || strings.EqualFold(newName, "(default)") {
		return fmt.Errorf("invalid group name")
	}
	for i, g := range cfg.Groups {
		if i != groupIdx && g.Name == newName {
			return fmt.Errorf("group %q already exists", newName)
		}
	}
	cfg.Groups[groupIdx].Name = newName
	return nil
}

// SetGroupDescription replaces metadata description lines with a single #@desc: line,
// or clears them when text is empty.
func (cfg *Config) SetGroupDescription(groupIdx int, text string) error {
	if groupIdx < 0 || groupIdx >= len(cfg.Groups) {
		return fmt.Errorf("invalid group index")
	}
	text = strings.TrimSpace(text)
	if text == "" {
		cfg.Groups[groupIdx].Descriptions = nil
		return nil
	}
	cfg.Groups[groupIdx].Descriptions = []string{"#@desc: " + text}
	return nil
}
