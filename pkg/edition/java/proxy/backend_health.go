package proxy

import (
	"sort"
	"strings"
	"time"
)

type backendHealth struct {
	Healthy             bool
	ConsecutiveFailures int
	LastAttempt         time.Time
}

func (p *Proxy) orderRouteServers(names []string) []string {
	ordered := append([]string(nil), names...)
	p.muS.RLock()
	defer p.muS.RUnlock()
	sort.SliceStable(ordered, func(i, j int) bool {
		left, leftKnown := p.backendHealth[strings.ToLower(ordered[i])]
		right, rightKnown := p.backendHealth[strings.ToLower(ordered[j])]
		leftUnhealthy := leftKnown && !left.Healthy
		rightUnhealthy := rightKnown && !right.Healthy
		return !leftUnhealthy && rightUnhealthy
	})
	return ordered
}

func (p *Proxy) recordBackendResult(server RegisteredServer, success bool) {
	if server == nil || !strings.HasPrefix(strings.ToLower(server.ServerInfo().Name()), "route-") {
		return
	}
	name := strings.ToLower(server.ServerInfo().Name())
	p.muS.Lock()
	defer p.muS.Unlock()
	health := p.backendHealth[name]
	health.LastAttempt = time.Now()
	health.Healthy = success
	if success {
		health.ConsecutiveFailures = 0
	} else {
		health.ConsecutiveFailures++
	}
	p.backendHealth[name] = health
}
