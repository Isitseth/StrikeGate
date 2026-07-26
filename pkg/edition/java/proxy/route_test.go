package proxy

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/edition/java/config"
	"go.minekube.com/gate/pkg/util/configutil"
	"go.minekube.com/gate/pkg/util/netutil"
)

func TestRouteBackendFailoverAndRetryLimit(t *testing.T) {
	cfg := &config.Config{
		Servers: map[string]string{},
		Routes: []config.Route{{
			Hostnames:  []string{"play.example.com"},
			RetryLimit: 2,
			Backends: []config.RouteBackend{
				{ID: "primary", Address: "127.0.0.1:25566", Priority: 1},
				{ID: "backup", Address: "127.0.0.1:25567", Priority: 2},
				{ID: "never", Address: "127.0.0.1:25568", Priority: 3},
			},
		}},
	}
	p := createTestProxyWithForcedHosts(t, cfg.RouteServers(), nil, nil)
	p.cfg.Routes = cfg.Routes
	player := &connectedPlayer{
		sessionHandlerDeps: &sessionHandlerDeps{proxy: p, configProvider: &testConfigProvider{cfg: p.cfg}},
		virtualHost:        netutil.NewAddr("play.example.com:25565", "tcp"),
		route:              &p.cfg.Routes[0],
		routeServers:       p.cfg.RouteServerNames(&p.cfg.Routes[0]),
	}

	primary := player.nextServerToTry(nil)
	require.Equal(t, "route-0-primary", primary.ServerInfo().Name())
	player.tryIndex++
	backup := player.nextServerToTry(primary)
	require.Equal(t, "route-0-backup", backup.ServerInfo().Name())
	player.tryIndex++
	require.Nil(t, player.nextServerToTry(backup))
}

func TestDisabledRouteRejectReason(t *testing.T) {
	disabled := false
	route := &config.Route{Enabled: &disabled}
	require.Equal(t, "This route is currently disabled.", disabledRouteReason(route).(*component.Text).Content)
	custom := configutil.TextComponent(component.Text{Content: "Customer maintenance"})
	route.DisabledMessage = &custom
	require.Equal(t, "Customer maintenance", disabledRouteReason(route).(*component.Text).Content)
	require.Nil(t, disabledRouteReason(nil))
}

func TestRouteBackendHealthTracking(t *testing.T) {
	p := &Proxy{backendHealth: map[string]backendHealth{}}
	server := newRegisteredServer(NewServerInfo("route-0-primary", netutil.NewAddr("127.0.0.1:25566", "tcp")))
	p.recordBackendResult(server, false)
	p.recordBackendResult(server, false)
	require.False(t, p.backendHealth["route-0-primary"].Healthy)
	require.Equal(t, 2, p.backendHealth["route-0-primary"].ConsecutiveFailures)
	p.recordBackendResult(server, true)
	require.True(t, p.backendHealth["route-0-primary"].Healthy)
	require.Zero(t, p.backendHealth["route-0-primary"].ConsecutiveFailures)
}

func TestUnhealthyRouteBackendIsTriedAfterHealthyBackend(t *testing.T) {
	p := &Proxy{backendHealth: map[string]backendHealth{
		"route-0-primary": {Healthy: false, ConsecutiveFailures: 1},
		"route-0-backup":  {Healthy: true},
	}}
	require.Equal(t,
		[]string{"route-0-backup", "route-0-unknown", "route-0-primary"},
		p.orderRouteServers([]string{"route-0-primary", "route-0-backup", "route-0-unknown"}))
}
