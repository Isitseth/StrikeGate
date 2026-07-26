package config

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/util/configutil"
	"go.minekube.com/gate/pkg/util/validation"
	"gopkg.in/yaml.v3"
)

// Route extends classic forced-host routing with per-host behavior.
// Legacy ForcedHosts remain supported and are evaluated after Routes.
type Route struct {
	Hostnames       []string                  `yaml:"hostnames" json:"hostnames"`
	Backend         string                    `yaml:"backend,omitempty" json:"backend,omitempty"`
	Backends        []RouteBackend            `yaml:"backends,omitempty" json:"backends,omitempty"`
	Forwarding      *RouteForwarding          `yaml:"forwarding,omitempty" json:"forwarding,omitempty"`
	Motd            *configutil.Component     `yaml:"motd,omitempty" json:"motd,omitempty"`
	Enabled         *bool                     `yaml:"enabled,omitempty" json:"enabled,omitempty"`
	DisabledMessage *configutil.TextComponent `yaml:"disabledMessage,omitempty" json:"disabledMessage,omitempty"`
	RetryLimit      int                       `yaml:"retryLimit,omitempty" json:"retryLimit,omitempty"`
}

// RouteBackend is a backend candidate belonging to a route.
type RouteBackend struct {
	ID       string `yaml:"id" json:"id"`
	Address  string `yaml:"address" json:"address"`
	Priority int    `yaml:"priority,omitempty" json:"priority,omitempty"`
}

// RouteForwarding contains route-local forwarding overrides.
// Secret is intentionally excluded from all serialization.
type RouteForwarding struct {
	Mode   ForwardingMode `yaml:"mode,omitempty" json:"mode,omitempty"`
	Secret Secret         `yaml:"-" json:"-"`
}

func (f *RouteForwarding) UnmarshalYAML(node *yaml.Node) error {
	var raw struct {
		Mode   ForwardingMode `yaml:"mode"`
		Secret Secret         `yaml:"secret"`
	}
	if err := node.Decode(&raw); err != nil {
		return err
	}
	f.Mode, f.Secret = raw.Mode, raw.Secret
	return nil
}

func (f RouteForwarding) MarshalYAML() (any, error) {
	return struct {
		Mode ForwardingMode `yaml:"mode,omitempty"`
	}{Mode: f.Mode}, nil
}

func (r *Route) IsEnabled() bool { return r == nil || r.Enabled == nil || *r.Enabled }

func (r *Route) EffectiveForwarding(global Forwarding) Forwarding {
	if r == nil || r.Forwarding == nil {
		return global
	}
	out := global
	if r.Forwarding.Mode != "" {
		out.Mode = r.Forwarding.Mode
	}
	if r.Forwarding.Secret != "" {
		switch out.Mode {
		case VelocityForwardingMode:
			out.VelocitySecret = r.Forwarding.Secret
		case BungeeGuardForwardingMode:
			out.BungeeGuardSecret = r.Forwarding.Secret
		}
	}
	return out
}

func (r *Route) EffectiveMotd(global *configutil.Component) component.Component {
	if r != nil && r.Motd != nil {
		return r.Motd.C()
	}
	if global == nil {
		return nil
	}
	return global.C()
}

func (r *Route) BackendList() []RouteBackend {
	if r == nil {
		return nil
	}
	backends := append([]RouteBackend(nil), r.Backends...)
	if r.Backend != "" {
		backends = append([]RouteBackend{{ID: "primary", Address: r.Backend}}, backends...)
	}
	sort.SliceStable(backends, func(i, j int) bool { return backends[i].Priority < backends[j].Priority })
	return backends
}

func (r *Route) MaxAttempts() int {
	n := len(r.BackendList())
	if r != nil && r.RetryLimit > 0 && r.RetryLimit < n {
		return r.RetryLimit
	}
	return n
}

// MatchRoute applies case-insensitive exact-first, wildcard-second matching.
func (c *Config) MatchRoute(host string) *Route {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	for i := range c.Routes {
		for _, alias := range c.Routes[i].Hostnames {
			if !hasWildcard(alias) && strings.EqualFold(strings.TrimSuffix(alias, "."), host) {
				return &c.Routes[i]
			}
		}
	}
	for i := range c.Routes {
		for _, alias := range c.Routes[i].Hostnames {
			if hasWildcard(alias) && wildcardMatch(host, alias) {
				return &c.Routes[i]
			}
		}
	}
	return nil
}

func validateRoutes(c *Config, e func(string, ...any)) {
	aliases := make(map[string]int)
	for i := range c.Routes {
		r := &c.Routes[i]
		if len(r.Hostnames) == 0 {
			e("Route %d has no hostnames", i)
		}
		for _, alias := range r.Hostnames {
			key := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(alias), "."))
			if key == "" {
				e("Route %d has an empty hostname", i)
				continue
			}
			if previous, exists := aliases[key]; exists {
				e("Route %d hostname %q duplicates route %d", i, alias, previous)
			} else {
				aliases[key] = i
			}
		}
		backends := r.BackendList()
		if len(backends) == 0 {
			e("Route %d has no backend configured", i)
		}
		ids := make(map[string]struct{}, len(backends))
		for j, backend := range backends {
			if backend.ID == "" {
				e("Route %d backend %d has no id", i, j)
			} else if !validation.ValidServerName(backend.ID) {
				e("Route %d backend id %q is invalid", i, backend.ID)
			} else if _, exists := ids[strings.ToLower(backend.ID)]; exists {
				e("Route %d has duplicate backend id %q", i, backend.ID)
			} else {
				ids[strings.ToLower(backend.ID)] = struct{}{}
			}
			if err := validation.ValidHostPort(backend.Address); err != nil {
				e("Route %d backend %q has invalid address: %v", i, backend.ID, err)
			}
		}
		if r.RetryLimit < 0 {
			e("Route %d retryLimit must be >= 0", i)
		}
		if r.Forwarding != nil && r.Forwarding.Mode != "" && !validForwardingMode(r.Forwarding.Mode) {
			e("Route %d has unknown forwarding mode %q", i, r.Forwarding.Mode)
		}
	}
}

func validForwardingMode(mode ForwardingMode) bool {
	switch mode {
	case NoneForwardingMode, LegacyForwardingMode, VelocityForwardingMode, BungeeGuardForwardingMode:
		return true
	default:
		return false
	}
}

func hasWildcard(s string) bool { return strings.ContainsAny(s, "*?") }

func wildcardMatch(host, pattern string) bool {
	quoted := regexp.QuoteMeta(strings.ToLower(strings.TrimSuffix(pattern, ".")))
	quoted = strings.ReplaceAll(quoted, `\*`, `.*`)
	quoted = strings.ReplaceAll(quoted, `\?`, `.`)
	matched, err := regexp.MatchString("^"+quoted+"$", host)
	return err == nil && matched
}

func routeServerName(routeIndex int, backend RouteBackend) string {
	return fmt.Sprintf("route-%d-%s", routeIndex, strings.ToLower(backend.ID))
}

// RouteServerNames returns private registered-server names for a route.
func (c *Config) RouteServerNames(route *Route) []string {
	for i := range c.Routes {
		if &c.Routes[i] != route {
			continue
		}
		backends := route.BackendList()
		limit := route.MaxAttempts()
		names := make([]string, 0, limit)
		for j := 0; j < len(backends) && j < limit; j++ {
			names = append(names, routeServerName(i, backends[j]))
		}
		return names
	}
	return nil
}

// RouteServers returns private registered server names and addresses.
func (c *Config) RouteServers() map[string]string {
	out := make(map[string]string)
	for i := range c.Routes {
		for _, backend := range c.Routes[i].BackendList() {
			out[routeServerName(i, backend)] = backend.Address
		}
	}
	return out
}
