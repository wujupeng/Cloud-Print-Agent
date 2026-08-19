package device

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/cloud-print/agent/internal/domain"
)

const (
	defaultHealthCheckInterval = 10 * time.Second
	healthCheckDialTimeout     = 3 * time.Second
)

type StatusChangeCallback func(deviceID string, oldStatus, newStatus domain.DeviceStatus)

type HealthChecker struct {
	manager   *Manager
	logger    *zap.Logger
	callbacks []StatusChangeCallback
	cbMu      sync.RWMutex
}

func NewHealthChecker(manager *Manager, logger *zap.Logger) *HealthChecker {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &HealthChecker{manager: manager, logger: logger}
}

func (h *HealthChecker) OnStatusChange(callback StatusChangeCallback) {
	h.cbMu.Lock()
	defer h.cbMu.Unlock()
	h.callbacks = append(h.callbacks, callback)
}

func (h *HealthChecker) fireStatusChange(deviceID string, oldStatus, newStatus domain.DeviceStatus) {
	if oldStatus == newStatus {
		return
	}
	h.cbMu.RLock()
	callbacks := make([]StatusChangeCallback, len(h.callbacks))
	copy(callbacks, h.callbacks)
	h.cbMu.RUnlock()

	for _, cb := range callbacks {
		cb(deviceID, oldStatus, newStatus)
	}
}

func (h *HealthChecker) checkDevice(ctx context.Context, dev *domain.Device) {
	port := dev.DefaultPort()
	if port <= 0 {
		port = 9100
	}
	addr := fmt.Sprintf("%s:%d", dev.IP, port)

	reachable := false
	dialer := net.Dialer{Timeout: healthCheckDialTimeout}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err == nil {
		_ = conn.Close()
		reachable = true
	}

	newStatus := domain.DeviceStatusOffline
	if reachable {
		newStatus = domain.DeviceStatusOnline
	}

	h.manager.mu.Lock()
	existing, ok := h.manager.devices[dev.DeviceID]
	if !ok {
		h.manager.mu.Unlock()
		return
	}
	oldStatus := existing.Status
	if oldStatus == newStatus {
		h.manager.mu.Unlock()
		return
	}
	existing.Status = newStatus
	existing.UpdatedAt = time.Now().UTC()
	h.manager.mu.Unlock()

	h.logger.Info("device status changed",
		zap.String("device_id", dev.DeviceID),
		zap.String("from", oldStatus.String()),
		zap.String("to", newStatus.String()),
	)
	h.fireStatusChange(dev.DeviceID, oldStatus, newStatus)
}

func (h *HealthChecker) StartHealthCheck(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = defaultHealthCheckInterval
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		h.logger.Info("health check started",
			zap.Duration("interval", interval),
		)

		for {
			select {
			case <-ctx.Done():
				h.logger.Info("health check stopped")
				return
			case <-ticker.C:
				devices := h.manager.List()
				for _, dev := range devices {
					if ctx.Err() != nil {
						return
					}
					h.checkDevice(ctx, dev)
				}
			}
		}
	}()
}