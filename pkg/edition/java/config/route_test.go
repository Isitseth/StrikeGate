package config

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestRoutesLoadLegacyAndExtendedBackends(t *testing.T) {
	var cfg Config
	require.NoError(t, yaml.Unmarshal([]byte(`
servers:
  lobby: 127.0.0.1:25566
forcedHosts:
  old.example.com: [lobby]
routes:
  - hostnames: [single.example.com]
    backend: 127.0.0.1:25567
  - hostnames: [multi.example.com]
    backends:
      - id: backup
        address: 127.0.0.1:25569
        priority: 2
      - id: primary
        address: 127.0.0.1:25568
        priority: 1
`), &cfg))
	require.Equal(t, []string{"lobby"}, cfg.ForcedHosts["old.example.com"])
	require.Equal(t, "127.0.0.1:25567", cfg.Routes[0].BackendList()[0].Address)
	require.Equal(t, []string{"primary", "backup"}, []string{
		cfg.Routes[1].BackendList()[0].ID,
		cfg.Routes[1].BackendList()[1].ID,
	})
}

func TestRouteMatchingAliasesAndWildcardPrecedence(t *testing.T) {
	cfg := Config{Routes: []Route{
		{Hostnames: []string{"*.example.com"}, Backend: "127.0.0.1:25566"},
		{Hostnames: []string{"Play.Example.com", "mc.customer.test"}, Backend: "127.0.0.1:25567"},
	}}
	require.Same(t, &cfg.Routes[1], cfg.MatchRoute("play.example.com"))
	require.Same(t, &cfg.Routes[1], cfg.MatchRoute("MC.CUSTOMER.TEST"))
	require.Same(t, &cfg.Routes[0], cfg.MatchRoute("other.example.com"))
	require.Nil(t, cfg.MatchRoute("example.net"))
}

func TestRouteDuplicateAliasesRejectedCaseInsensitively(t *testing.T) {
	cfg := Config{Routes: []Route{
		{Hostnames: []string{"Play.Example.com"}, Backend: "127.0.0.1:25566"},
		{Hostnames: []string{"play.example.com"}, Backend: "127.0.0.1:25567"},
	}}
	_, errs := cfg.Validate()
	require.Condition(t, func() bool {
		for _, err := range errs {
			if strings.Contains(err.Error(), "duplicates route") {
				return true
			}
		}
		return false
	})
}

func TestRouteDefaultsAndRetryLimit(t *testing.T) {
	route := Route{
		Hostnames:  []string{"play.example.com"},
		RetryLimit: 2,
		Backends: []RouteBackend{
			{ID: "one", Address: "127.0.0.1:25566"},
			{ID: "two", Address: "127.0.0.1:25567"},
			{ID: "three", Address: "127.0.0.1:25568"},
		},
	}
	require.True(t, route.IsEnabled())
	require.Equal(t, 2, route.MaxAttempts())
	disabled := false
	route.Enabled = &disabled
	require.False(t, route.IsEnabled())
}

func TestPerRouteForwardingFallbackAndIndependentSecrets(t *testing.T) {
	global := Forwarding{
		Mode:              LegacyForwardingMode,
		VelocitySecret:    "global-velocity",
		BungeeGuardSecret: "global-bungeeguard",
	}
	routes := []Route{
		{Forwarding: &RouteForwarding{Mode: VelocityForwardingMode, Secret: "customer-one"}},
		{Forwarding: &RouteForwarding{Mode: VelocityForwardingMode, Secret: "customer-two"}},
		{Forwarding: &RouteForwarding{Mode: NoneForwardingMode}},
		{Forwarding: &RouteForwarding{Mode: VelocityForwardingMode}},
	}
	require.Equal(t, "customer-one", string(routes[0].EffectiveForwarding(global).VelocitySecret))
	require.Equal(t, "customer-two", string(routes[1].EffectiveForwarding(global).VelocitySecret))
	require.Equal(t, NoneForwardingMode, routes[2].EffectiveForwarding(global).Mode)
	require.Equal(t, "global-velocity", string(routes[3].EffectiveForwarding(global).VelocitySecret))
}

func TestRouteSecretNeverSerializes(t *testing.T) {
	const secret = "sentinel-customer-secret"
	cfg := Config{Routes: []Route{{
		Hostnames:  []string{"play.example.com"},
		Backend:    "127.0.0.1:25566",
		Forwarding: &RouteForwarding{Mode: VelocityForwardingMode, Secret: secret},
	}}}
	jsonBytes, err := json.Marshal(cfg)
	require.NoError(t, err)
	yamlBytes, err := yaml.Marshal(cfg)
	require.NoError(t, err)
	require.NotContains(t, string(jsonBytes), secret)
	require.NotContains(t, string(yamlBytes), secret)
	require.NotContains(t, fmt.Sprintf("%+v", cfg), secret)

	var loaded Config
	require.NoError(t, yaml.Unmarshal([]byte(`
routes:
  - hostnames: [play.example.com]
    backend: 127.0.0.1:25566
    forwarding:
      mode: velocity
      secret: sentinel-customer-secret
`), &loaded))
	require.Equal(t, secret, string(loaded.Routes[0].Forwarding.Secret))
}

func TestRouteMotdUsesExistingComponentFormatting(t *testing.T) {
	var cfg Config
	require.NoError(t, yaml.Unmarshal([]byte(`
status:
  motd: global
routes:
  - hostnames: [play.example.com]
    backend: 127.0.0.1:25566
    motd: "§bCustomer"
`), &cfg))
	route := cfg.MatchRoute("play.example.com")
	require.NotNil(t, route)
	require.Equal(t, route.Motd.C(), route.EffectiveMotd(cfg.Status.Motd))
	require.Equal(t, cfg.Status.Motd.C(), (*Route)(nil).EffectiveMotd(cfg.Status.Motd))
}
