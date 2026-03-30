package tui

import "strings"

// ConnectivityKeys are OpenSSH client directives shown in the "connectivity" tab.
var connectivityKeys = map[string]struct{}{
	"proxyjump":                   {},
	"proxycommand":                {},
	"localforward":                {},
	"remoteforward":               {},
	"dynamicforward":              {},
	"permitlocalcommand":          {},
	"localcommand":                {},
	"tunnel":                      {},
	"gatewayports":                {},
	"exitonforwardfailure":        {},
	"canonicalizehostname":        {},
	"canonicaldomains":            {},
	"canonicalizefallbacklocal":   {},
	"canonicalizemaxdots":         {},
	"canonicalizepermittedcnames": {},
	"globalknownhostsfile":        {},
	"userknownhostsfile":          {},
	"verifyhostkeydns":            {},
	"stricthostkeychecking":       {},
	"knownhostscommand":           {},
}

// IsConnectivityKey reports whether key is part of the connectivity-focused tab.
func IsConnectivityKey(key string) bool {
	_, ok := connectivityKeys[strings.ToLower(strings.TrimSpace(key))]
	return ok
}
