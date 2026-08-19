package observability

import (
	"sync"
	"time"

	"github.com/cloud-print/agent/internal/domain"
)

type NetMetrics struct {
	mu             sync.RWMutex
	dnsLatencyMs   int
	resolvedIP     string
	gatewayReach   bool
	localNetReach  bool
	netClass       domain.NetClass
	lastCheckAt    time.Time
}

func NewNetMetrics() *NetMetrics {
	return &NetMetrics{
		netClass: domain.NetClassOK,
	}
}

func (m *NetMetrics) Update(dnsLatencyMs int, resolvedIP string, gatewayReach, localNetReach bool, netClass domain.NetClass) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dnsLatencyMs = dnsLatencyMs
	m.resolvedIP = resolvedIP
	m.gatewayReach = gatewayReach
	m.localNetReach = localNetReach
	m.netClass = netClass
	m.lastCheckAt = time.Now().UTC()
}

func (m *NetMetrics) UpdateNetClass(netClass domain.NetClass) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.netClass = netClass
	m.lastCheckAt = time.Now().UTC()
}

func (m *NetMetrics) UpdateDNS(dnsLatencyMs int, resolvedIP string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dnsLatencyMs = dnsLatencyMs
	m.resolvedIP = resolvedIP
	m.lastCheckAt = time.Now().UTC()
}

func (m *NetMetrics) UpdateReach(gatewayReach, localNetReach bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.gatewayReach = gatewayReach
	m.localNetReach = localNetReach
	m.lastCheckAt = time.Now().UTC()
}

func (m *NetMetrics) Get() (dnsLatencyMs int, resolvedIP string, gatewayReach, localNetReach bool, netClass domain.NetClass, lastCheckAt time.Time) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.dnsLatencyMs, m.resolvedIP, m.gatewayReach, m.localNetReach, m.netClass, m.lastCheckAt
}

func (m *NetMetrics) GetDNSLatencyMs() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.dnsLatencyMs
}

func (m *NetMetrics) GetResolvedIP() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.resolvedIP
}

func (m *NetMetrics) GetGatewayReach() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.gatewayReach
}

func (m *NetMetrics) GetLocalNetReach() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.localNetReach
}

func (m *NetMetrics) GetNetClass() domain.NetClass {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.netClass
}

func (m *NetMetrics) GetLastCheckAt() time.Time {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.lastCheckAt
}

func (m *NetMetrics) ToNetTopology(endpoint string) domain.NetTopology {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return domain.NetTopology{
		Endpoint:      endpoint,
		ResolvedIP:    m.resolvedIP,
		DNSLatencyMs:  m.dnsLatencyMs,
		GatewayReach:  m.gatewayReach,
		LocalNetReach: m.localNetReach,
		NetClass:      m.netClass,
		LastCheckAt:   m.lastCheckAt,
	}
}

func (m *NetMetrics) Snapshot() NetMetricsSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return NetMetricsSnapshot{
		DNSLatencyMs:  m.dnsLatencyMs,
		ResolvedIP:    m.resolvedIP,
		GatewayReach:  m.gatewayReach,
		LocalNetReach: m.localNetReach,
		NetClass:      m.netClass,
		LastCheckAt:   m.lastCheckAt,
	}
}

type NetMetricsSnapshot struct {
	DNSLatencyMs  int              `json:"dns_latency_ms"`
	ResolvedIP    string           `json:"resolved_ip"`
	GatewayReach  bool             `json:"gateway_reach"`
	LocalNetReach bool             `json:"local_net_reach"`
	NetClass      domain.NetClass  `json:"net_class"`
	LastCheckAt   time.Time        `json:"last_check_at"`
}