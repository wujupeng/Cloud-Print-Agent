package cloudlink

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/cloud-print/agent/internal/domain"
	"github.com/cloud-print/agent/internal/observability"
)

const (
	defaultHeartbeatInterval = 30 * time.Second
	heartbeatMissThreshold   = 3
	heartbeatMsgType         = "heartbeat"
	heartbeatAckType         = "heartbeat_ack"
	heartbeatTimeout         = 10 * time.Second
)

type HeartbeatConfig struct {
	AgentID       string
	Version       string
	CloudEndpoint string
	Interval      time.Duration
}

type Heartbeat struct {
	cfg        HeartbeatConfig
	conn       *Conn
	reporter   *Reporter
	netMetrics *observability.NetMetrics
	queue      pendingCounter
	deviceMgr  deviceLister
	logger     *zap.Logger

	mu              sync.Mutex
	missCount       int
	connLossHandler func()
	cancel          context.CancelFunc
	done            chan struct{}
	running         bool
}

type pendingCounter interface {
	PendingCount(deviceID string) int
}

type deviceLister interface {
	List() []*domain.Device
}

func NewHeartbeat(
	cfg HeartbeatConfig,
	conn *Conn,
	reporter *Reporter,
	netMetrics *observability.NetMetrics,
	queue pendingCounter,
	deviceMgr deviceLister,
	logger *zap.Logger,
) *Heartbeat {
	if cfg.Interval <= 0 {
		cfg.Interval = defaultHeartbeatInterval
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Heartbeat{
		cfg:        cfg,
		conn:       conn,
		reporter:   reporter,
		netMetrics: netMetrics,
		queue:      queue,
		deviceMgr:  deviceMgr,
		logger:     logger,
	}
}

func (h *Heartbeat) OnConnLoss(handler func()) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.connLossHandler = handler
}

func (h *Heartbeat) UpdateConn(conn *Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.conn = conn
	h.missCount = 0
}

func (h *Heartbeat) Start(ctx context.Context) {
	h.mu.Lock()
	if h.running {
		h.mu.Unlock()
		return
	}
	subCtx, cancel := context.WithCancel(ctx)
	h.cancel = cancel
	h.done = make(chan struct{})
	h.running = true
	h.mu.Unlock()

	go h.loop(subCtx)
}

func (h *Heartbeat) Stop() {
	h.mu.Lock()
	if !h.running {
		h.mu.Unlock()
		return
	}
	h.running = false
	cancel := h.cancel
	done := h.done
	h.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
}

func (h *Heartbeat) loop(ctx context.Context) {
	defer close(h.done)
	ticker := time.NewTicker(h.cfg.Interval)
	defer ticker.Stop()

	h.sendOnce(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.sendOnce(ctx)
		}
	}
}

func (h *Heartbeat) sendOnce(ctx context.Context) {
	h.mu.Lock()
	conn := h.conn
	h.mu.Unlock()
	if conn == nil {
		h.recordMiss()
		return
	}

	online := 0
	pending := 0
	if h.deviceMgr != nil {
		devs := h.deviceMgr.List()
		for _, d := range devs {
			if d.Status == domain.DeviceStatusOnline {
				online++
			}
			if h.queue != nil {
				pending += h.queue.PendingCount(d.DeviceID)
			}
		}
	}
	netClass := domain.NetClassOK
	if h.netMetrics != nil {
		netClass = h.netMetrics.GetNetClass()
	}

	payload := domain.HeartbeatPayload{
		AgentID:       h.cfg.AgentID,
		Version:       h.cfg.Version,
		OnlineDevices: online,
		PendingTasks:  pending,
		CloudEndpoint: h.cfg.CloudEndpoint,
		NetClass:      netClass,
		Timestamp:     time.Now().UTC(),
	}
	env, err := domain.NewEnvelope(heartbeatMsgType, payload)
	if err != nil {
		h.logger.Warn("heartbeat marshal failed", zap.Error(err))
		h.recordMiss()
		return
	}

	sendCtx, cancel := context.WithTimeout(ctx, heartbeatTimeout)
	defer cancel()
	if err := conn.Write(sendCtx, env); err != nil {
		h.logger.Warn("heartbeat send failed", zap.Error(err))
		h.recordMiss()
		return
	}
	h.logger.Info("heartbeat sent", zap.String("agent_id", h.cfg.AgentID), zap.Int("online_devices", online))

	if h.deviceMgr != nil && h.reporter != nil {
		for _, d := range h.deviceMgr.List() {
			_ = h.reporter.ReportDeviceStatus(d.DeviceID, d.Status)
		}
	}

	h.mu.Lock()
	h.missCount = 0
	h.mu.Unlock()
}


func (h *Heartbeat) recordMiss() {
	h.mu.Lock()
	h.missCount++
	miss := h.missCount
	handler := h.connLossHandler
	h.mu.Unlock()

	h.logger.Warn("heartbeat miss",
		zap.Int("miss_count", miss),
	)
	if miss >= heartbeatMissThreshold && handler != nil {
		h.logger.Warn("heartbeat lost, trigger reconnect")
		go handler()
	}
}