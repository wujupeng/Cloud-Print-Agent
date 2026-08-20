package device

import (
	"net"
	"regexp"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/cloud-print/agent/internal/domain"
	"github.com/cloud-print/agent/internal/errs"
	"github.com/cloud-print/agent/internal/storage"
)

var deviceIDPattern = regexp.MustCompile(`^[A-Za-z0-9\-]{3,64}$`)

type Manager struct {
	mu      sync.RWMutex
	devices map[string]*domain.Device
	logger  *zap.Logger
	repo    *storage.TaskRepo
}

func NewManager(logger *zap.Logger, repo *storage.TaskRepo) *Manager {
	return &Manager{
		devices: make(map[string]*domain.Device),
		logger:  logger,
		repo:    repo,
	}
}

func (m *Manager) validate(dev *domain.Device) error {
	if dev == nil {
		return errs.New(errs.ErrDeviceFieldInvalid, "device is nil")
	}
	if !deviceIDPattern.MatchString(dev.DeviceID) {
		return errs.Newf(errs.ErrDeviceFieldInvalid,
			"device_id must be 3-32 chars of uppercase letters and digits, got %q", dev.DeviceID)
	}
	if len(dev.Name) < 1 || len(dev.Name) > 64 {
		return errs.Newf(errs.ErrDeviceFieldInvalid,
			"name length must be 1-64, got %d", len(dev.Name))
	}
	if dev.Protocol != domain.ProtocolCUPS {
		ip := net.ParseIP(dev.IP)
		if ip == nil || ip.To4() == nil {
			return errs.Newf(errs.ErrDeviceFieldInvalid, "ip must be valid IPv4, got %q", dev.IP)
		}
	}
	if len(dev.Model) < 1 || len(dev.Model) > 64 {
		return errs.Newf(errs.ErrDeviceFieldInvalid,
			"model length must be 1-64, got %d", len(dev.Model))
	}
	if len(dev.Factory) < 1 || len(dev.Factory) > 64 {
		return errs.Newf(errs.ErrDeviceFieldInvalid,
			"factory length must be 1-64, got %d", len(dev.Factory))
	}
	return nil
}

func (m *Manager) Add(dev *domain.Device) error {
	if err := m.validate(dev); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.devices[dev.DeviceID]; exists {
		return errs.Newf(errs.ErrDeviceIDConflict, "device_id %s already exists", dev.DeviceID)
	}

	now := time.Now().UTC()
	clone := *dev
	if clone.CreatedAt.IsZero() {
		clone.CreatedAt = now
	}
	clone.UpdatedAt = now
	if clone.Status == "" {
		clone.Status = domain.DeviceStatusOffline
	}
	m.devices[clone.DeviceID] = &clone

	if m.logger != nil {
		m.logger.Info("device added",
			zap.String("device_id", clone.DeviceID),
			zap.String("name", clone.Name),
		)
	}
	return nil
}

func (m *Manager) Update(dev *domain.Device) error {
	if err := m.validate(dev); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	existing, exists := m.devices[dev.DeviceID]
	if !exists {
		return errs.Newf(errs.ErrDeviceNotFound, "device_id %s not found", dev.DeviceID)
	}

	clone := *dev
	clone.CreatedAt = existing.CreatedAt
	clone.UpdatedAt = time.Now().UTC()
	m.devices[clone.DeviceID] = &clone

	if m.logger != nil {
		m.logger.Info("device updated",
			zap.String("device_id", clone.DeviceID),
		)
	}
	return nil
}

func (m *Manager) Remove(deviceID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.devices[deviceID]; !exists {
		return errs.Newf(errs.ErrDeviceNotFound, "device_id %s not found", deviceID)
	}
	delete(m.devices, deviceID)

	if m.logger != nil {
		m.logger.Info("device removed",
			zap.String("device_id", deviceID),
		)
	}
	return nil
}

func (m *Manager) Get(deviceID string) (*domain.Device, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	dev, ok := m.devices[deviceID]
	if !ok {
		return nil, false
	}
	clone := *dev
	return &clone, true
}

func (m *Manager) List() []*domain.Device {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*domain.Device, 0, len(m.devices))
	for _, dev := range m.devices {
		clone := *dev
		result = append(result, &clone)
	}
	return result
}

func (m *Manager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.devices)
}


func (m *Manager) applyProbeResult(deviceID string, protocol domain.Protocol, status domain.DeviceStatus) {
	m.mu.Lock()
	defer m.mu.Unlock()

	dev, ok := m.devices[deviceID]
	if !ok {
		return
	}
	dev.Protocol = protocol
	dev.Status = status
	dev.LastProbeAt = time.Now().UTC()
	dev.UpdatedAt = dev.LastProbeAt
}