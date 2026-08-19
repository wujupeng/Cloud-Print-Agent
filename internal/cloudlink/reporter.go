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
	msgTypeTaskResult     = "task_result"
	msgTypeDeviceStatus   = "device_status"
	msgTypeNetEvent       = "net_event"
	msgTypeConfigAck      = "config_ack"

	reporterBufferSize    = 256
	reporterSendTimeout   = 5 * time.Second
)

type Reporter struct {
	conn       *Conn
	logger     *zap.Logger
	netMetrics *observability.NetMetrics

	mu     sync.Mutex
	buf    []*domain.Envelope
	connMu sync.RWMutex
}

func NewReporter(conn *Conn, netMetrics *observability.NetMetrics, logger *zap.Logger) *Reporter {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Reporter{
		conn:       conn,
		logger:     logger,
		netMetrics: netMetrics,
	}
}

func (r *Reporter) UpdateConn(conn *Conn) {
	r.connMu.Lock()
	r.conn = conn
	r.connMu.Unlock()
	if conn != nil {
		r.flushBuffer()
	}
}

func (r *Reporter) Send(msg *domain.Envelope) error {
	if msg == nil {
		return nil
	}
	r.connMu.RLock()
	conn := r.conn
	r.connMu.RUnlock()
	if conn == nil {
		r.buffer(msg)
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), reporterSendTimeout)
	defer cancel()
	if err := conn.Write(ctx, msg); err != nil {
		r.logger.Warn("reporter send failed, buffer msg", zap.Error(err))
		r.buffer(msg)
		return err
	}
	return nil
}

func (r *Reporter) buffer(msg *domain.Envelope) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.buf) >= reporterBufferSize {
		r.buf = r.buf[1:]
	}
	r.buf = append(r.buf, msg)
}

func (r *Reporter) flushBuffer() {
	r.mu.Lock()
	if len(r.buf) == 0 {
		r.mu.Unlock()
		return
	}
	pending := r.buf
	r.buf = nil
	r.mu.Unlock()

	r.connMu.RLock()
	conn := r.conn
	r.connMu.RUnlock()
	if conn == nil {
		r.mu.Lock()
		r.buf = append(pending, r.buf...)
		r.mu.Unlock()
		return
	}
	for _, msg := range pending {
		ctx, cancel := context.WithTimeout(context.Background(), reporterSendTimeout)
		if err := conn.Write(ctx, msg); err != nil {
			r.logger.Warn("flush buffer send failed", zap.Error(err))
			r.buffer(msg)
		}
		cancel()
	}
}

func (r *Reporter) ReportTaskResult(task *domain.PrintTask) error {
	if task == nil {
		return nil
	}
	payload := domain.TaskResultPayload{
		TaskID:     task.TaskID,
		DeviceID:   task.DeviceID,
		Status:     task.Status,
		RetryCount: task.RetryCount,
		ErrorCode:  task.ErrorCode,
		ErrorMsg:   task.ErrorMsg,
		FinishedAt: task.FinishedAt,
	}
	env, err := domain.NewEnvelope(msgTypeTaskResult, payload)
	if err != nil {
		return err
	}
	env.TraceID = task.TraceID
	return r.Send(env)
}

func (r *Reporter) ReportDeviceStatus(deviceID string, status domain.DeviceStatus) error {
	payload := domain.DeviceStatusPayload{
		DeviceID: deviceID,
		Status:   status,
	}
	env, err := domain.NewEnvelope(msgTypeDeviceStatus, payload)
	if err != nil {
		return err
	}
	return r.Send(env)
}

func (r *Reporter) ReportNetEvent(class domain.NetClass, detail string) error {
	payload := domain.NetEventPayload{
		Class:  class,
		Detail: detail,
		TS:     time.Now().UTC(),
	}

	env, err := domain.NewEnvelope(msgTypeNetEvent, payload)
	if err != nil {
		return err
	}
	return r.Send(env)
}

func (r *Reporter) ReportConfigAck(applied bool, reason string, field string) error {
	payload := domain.ConfigAckPayload{
		Applied: applied,
		Reason:  reason,
		Field:   field,
	}
	env, err := domain.NewEnvelope(msgTypeConfigAck, payload)
	if err != nil {
		return err
	}
	return r.Send(env)
}
