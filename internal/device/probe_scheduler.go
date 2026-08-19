package device

import (
	"context"

	"go.uber.org/zap"

	"github.com/cloud-print/agent/internal/domain"
	"github.com/cloud-print/agent/internal/errs"
	"github.com/cloud-print/agent/internal/protocol"
)

type ProbeScheduler struct {
	manager *Manager
	logger  *zap.Logger
}

func NewProbeScheduler(manager *Manager, logger *zap.Logger) *ProbeScheduler {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &ProbeScheduler{manager: manager, logger: logger}
}

func (s *ProbeScheduler) ProbeAndUpdate(ctx context.Context, deviceID string) error {
	dev, ok := s.manager.Get(deviceID)
	if !ok {
		return errs.Newf(errs.ErrDeviceNotFound, "device_id %s not found", deviceID)
	}

	proto, status, err := protocol.ProbeProtocol(ctx, dev.IP)
	if err != nil {
		s.logger.Warn("probe protocol failed",
			zap.String("device_id", deviceID),
			zap.String("ip", dev.IP),
			zap.Error(err),
		)
		s.manager.applyProbeResult(deviceID, domain.ProtocolUnknown, domain.DeviceStatusProbeFailed)
		return errs.Wrap(errs.ErrProtocolProbeFail, "probe protocol", err)
	}

	s.manager.applyProbeResult(deviceID, proto, status)

	s.logger.Info("probe updated",
		zap.String("device_id", deviceID),
		zap.String("protocol", proto.String()),
		zap.String("status", status.String()),
	)
	return nil
}

func (s *ProbeScheduler) ProbeAll(ctx context.Context) {
	devices := s.manager.List()
	for _, dev := range devices {
		if ctx.Err() != nil {
			return
		}
		_ = s.ProbeAndUpdate(ctx, dev.DeviceID)
	}
}