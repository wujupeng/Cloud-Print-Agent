package netprobe

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/cloud-print/agent/internal/domain"
	"github.com/cloud-print/agent/internal/observability"
)

const defaultProbeInterval = 10 * time.Second

type LoopProber struct {
	endpoint    string
	gatewayIP   string
	gatewayPort int
	interval    time.Duration
	metrics     *observability.NetMetrics
	logger      *zap.Logger
	audit       *observability.AuditLogger

	mu        sync.RWMutex
	current   domain.NetClass
	callbacks []func(old, new domain.NetClass)
	cancel    context.CancelFunc
	done      chan struct{}
	running   bool
}

func NewLoopProber(
	endpoint string,
	gatewayIP string,
	gatewayPort int,
	metrics *observability.NetMetrics,
	logger *zap.Logger,
	audit *observability.AuditLogger,
) *LoopProber {
	return &LoopProber{
		endpoint:    endpoint,
		gatewayIP:   gatewayIP,
		gatewayPort: gatewayPort,
		interval:    defaultProbeInterval,
		metrics:     metrics,
		logger:      logger,
		audit:       audit,
		current:     domain.NetClassOK,
	}
}

func (p *LoopProber) WithInterval(d time.Duration) *LoopProber {
	if d > 0 {
		p.interval = d
	}
	return p
}

func (p *LoopProber) Start(ctx context.Context) {
	p.mu.Lock()
	if p.running {
		p.mu.Unlock()
		return
	}
	cctx, cancel := context.WithCancel(ctx)
	p.cancel = cancel
	p.done = make(chan struct{})
	p.running = true
	p.mu.Unlock()

	go p.loop(cctx)
}

func (p *LoopProber) Stop() {
	p.mu.Lock()
	if !p.running {
		p.mu.Unlock()
		return
	}
	p.running = false
	cancel := p.cancel
	done := p.done
	p.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
}

func (p *LoopProber) OnClassChange(callback func(old, new domain.NetClass)) {
	p.mu.Lock()
	p.callbacks = append(p.callbacks, callback)
	p.mu.Unlock()
}

func (p *LoopProber) loop(ctx context.Context) {
	defer close(p.done)
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	p.probeOnce(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.probeOnce(ctx)
		}
	}
}

func (p *LoopProber) probeOnce(ctx context.Context) {
	class, topo, _ := p.Probe(ctx)
	p.metrics.Update(topo.DNSLatencyMs, topo.ResolvedIP, topo.GatewayReach, topo.LocalNetReach, class)

	p.mu.RLock()
	old := p.current
	callbacks := p.callbacks
	p.mu.RUnlock()

	if old == class {
		return
	}

	p.mu.Lock()
	p.current = class
	p.mu.Unlock()

	if p.logger != nil {
		p.logger.Info("net class changed",
			zap.String("from", string(old)),
			zap.String("to", string(class)),
		)
	}
	if p.audit != nil {
		p.audit.LogNetClassChange(string(old), string(class), "")
	}
	for _, cb := range callbacks {
		cb(old, class)
	}
}

func (p *LoopProber) Probe(ctx context.Context) (domain.NetClass, domain.NetTopology, error) {
	localOK, _ := ProbeLocalNet(ctx)
	resolvedIP, dnsLat, dnsErr := ProbeDNS(ctx, p.endpoint)
	dnsOK := dnsErr == nil
	gwOK, _ := ProbeGateway(ctx, p.gatewayIP, p.gatewayPort)

	class := Classify(localOK, dnsOK, gwOK)
	topo := domain.NetTopology{
		Endpoint:      p.endpoint,
		ResolvedIP:    resolvedIP,
		DNSLatencyMs:  dnsLat,
		GatewayIP:     p.gatewayIP,
		GatewayReach:  gwOK,
		LocalNetReach: localOK,
		NetClass:      class,
		LastCheckAt:   time.Now().UTC(),
	}
	return class, topo, dnsErr
}