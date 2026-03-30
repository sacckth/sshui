package config

// Directive is one key/value line inside a Host block.
type Directive struct {
	Key   string
	Value string
}

// HostBlock is one OpenSSH Host stanza (patterns as on the Host line).
// HostComments holds sshclick-style metadata lines (#@host: …) before this stanza.
type HostBlock struct {
	HostComments []string
	Patterns     []string
	Directives   []Directive
}

// Group is a logical section introduced by #@group: (sshclick-style metadata).
type Group struct {
	Name         string
	Descriptions []string
	Hosts        []HostBlock
}

// Config is the in-memory representation of a single ssh config file.
type Config struct {
	DefaultHosts []HostBlock
	Groups       []Group
	HasInclude   bool
}

// HostRef addresses a host block within Config.
type HostRef struct {
	InDefault bool
	GroupIdx  int // used when InDefault is false
	HostIdx   int
}
