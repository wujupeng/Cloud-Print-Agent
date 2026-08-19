package device

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/cloud-print/agent/internal/domain"
)

const debounceWindow = 30 * time.Second

type pendingChange struct {
	deviceID  string
	status    domain.DeviceStatus
	firstSeen time.Time
	timer     *time.Timer
}

type Debouncer struct {
	mu       sync.Mutex
	pending  map[string]*pendingChange
	logger   *zap.Logger
	reporter func(deviceID string, status domain.DeviceStatus)
}

func NewDebouncer(logger *zap.Logger, reporter func(deviceID string, status domain.DeviceStatus)) *Debouncer {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Debouncer{
		pending:  make(map[string]*pendingChange),
		logger:   logger,
		reporter: reporter,
	}
}

func (d *Debouncer) ApplyChange(deviceID string, newStatus domain.DeviceStatus) {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now().UTC()
	if existing, ok := d.pending[deviceID]; ok {
		existing.status = newStatus
		d.logger.Debug("debounce update",
			zap.String("device_id", deviceID),
			zap.String("status", newStatus.String()),
		)
		return
	}

	pc := &pendingChange{
		deviceID:  deviceID,
		status:    newStatus,
		firstSeen: now,
	}
	pc.timer = time.AfterFunc(debounceWindow, func() {
		d.flush(deviceID)
	})
	d.pending[deviceID] = pc

	d.logger.Debug("debounce start",
		zap.String("device_id", deviceID),
		zap.String("status", newStatus.String()),
	)
}

func (d *Debouncer) flush(deviceID string) {
	d.mu.Lock()
	pc, ok := d.pending[deviceID]
	if !ok {
		d.mu.Unlock()
		return
	}
	delete(d.pending, deviceID)
	d.mu.Unlock()

	if d.reporter != nil {
		d.reporter(pc.deviceID, pc.status)
	}

	d.logger.Info("debounce report",
		zap.String("device_id", pc.deviceID),
		zap.String("status", pc.status.String()),
	)
}

func (d *Debouncer) Start(ctx context.Context) {
	go func() {
		<-ctx.Done()
		d.mu.Lock()
		defer d.mu.Unlock()
		for deviceID, pc := range d.pending {
			if pc.timer != nil {
				pc.timer.Stop()
			}
			if d.reporter != nil {
				d.reporter(pc.deviceID, pc.status)
			}
			delete(d.pending, deviceID)
		}
	}()
}