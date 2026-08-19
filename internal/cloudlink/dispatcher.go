package cloudlink

import (
	"context"
	"encoding/json"
	"sync"

	"go.uber.org/zap"

	"github.com/cloud-print/agent/internal/domain"

)

const (
	msgTypeTask          = "task"
	msgTypeDeviceAdd     = "device_add"
	msgTypeDeviceUpdate  = "device_update"
	msgTypeDeviceRemove  = "device_remove"
	msgTypeControl       = "control"
	msgTypeConfigUpdate  = "config_update"

	controlTestPage  = "test_page"
	controlQueryQueue = "query_queue"
	controlCancel    = "cancel"

	ackTypeTaskAck = "task_ack"
)

type Dispatcher struct {
	conn       *Conn
	queue      queueOps
	deviceMgr  deviceMutator
	reporter   *Reporter
	domainUpd  *DomainUpdater
	logger     *zap.Logger

	cancel  context.CancelFunc
	done    chan struct{}
	running bool
	mu     sync.Mutex
}

type queueOps interface {
	Enqueue(task *domain.PrintTask) error
	Cancel(deviceID string, taskID string) error
}

type deviceMutator interface {
	Add(dev *domain.Device) error
	Update(dev *domain.Device) error
	Remove(deviceID string) error
}

func NewDispatcher(
	conn *Conn,
	queue queueOps,
	deviceMgr deviceMutator,
	reporter *Reporter,
	domainUpd *DomainUpdater,
	logger *zap.Logger,
) *Dispatcher {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Dispatcher{
		conn:      conn,
		queue:     queue,
		deviceMgr: deviceMgr,
		reporter:  reporter,
		domainUpd: domainUpd,
		logger:    logger,
	}
}

func (d *Dispatcher) UpdateConn(conn *Conn) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.conn = conn
}

func (d *Dispatcher) Start(ctx context.Context) {
	d.mu.Lock()
	if d.running {
		d.mu.Unlock()
		return
	}
	subCtx, cancel := context.WithCancel(ctx)
	d.cancel = cancel
	d.done = make(chan struct{})
	d.running = true
	d.mu.Unlock()

	go d.loop(subCtx)
}

func (d *Dispatcher) Stop() {
	d.mu.Lock()
	if !d.running {
		d.mu.Unlock()
		return
	}
	d.running = false
	cancel := d.cancel
	done := d.done
	d.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
}

func (d *Dispatcher) loop(ctx context.Context) {
	defer close(d.done)
	for {
		if ctx.Err() != nil {
			return
		}
		d.mu.Lock()
		conn := d.conn
		d.mu.Unlock()
		if conn == nil {
			return
		}

		var msg domain.Envelope
		if err := conn.Read(ctx, &msg); err != nil {
			if ctx.Err() != nil {
				return
			}
			d.logger.Warn("dispatcher read failed", zap.Error(err))
			return
		}
		d.handle(ctx, conn, &msg)
	}
}

func (d *Dispatcher) handle(ctx context.Context, conn *Conn, msg *domain.Envelope) {
	switch msg.Type {
	case msgTypeTask:
		d.handleTask(conn, msg)
	case msgTypeDeviceAdd:
		d.handleDeviceAdd(msg)
	case msgTypeDeviceUpdate:
		d.handleDeviceUpdate(msg)
	case msgTypeDeviceRemove:
		d.handleDeviceRemove(msg)
	case msgTypeControl:
		d.handleControl(msg)
	case msgTypeConfigUpdate:
		d.handleConfigUpdate(ctx, msg)
	default:
		d.logger.Debug("dispatcher unknown msg type",
			zap.String("type", msg.Type),
		)
	}
}

func (d *Dispatcher) handleTask(conn *Conn, msg *domain.Envelope) {
	var task domain.PrintTask
	if err := json.Unmarshal(msg.Payload, &task); err != nil {
		d.logger.Warn("task payload unmarshal failed",
			zap.String("trace_id", msg.TraceID),
			zap.Error(err),
		)
		d.sendTaskAck(conn, "", false, "bad payload")
		return
	}
	if task.TraceID == "" {
		task.TraceID = msg.TraceID
	}
	if err := d.queue.Enqueue(&task); err != nil {
		d.logger.Warn("enqueue task failed",
			zap.String("task_id", task.TaskID),
			zap.Error(err),
		)
		d.sendTaskAck(conn, task.TaskID, false, err.Error())
		return
	}
	d.sendTaskAck(conn, task.TaskID, true, "")
}

func (d *Dispatcher) sendTaskAck(conn *Conn, taskID string, accepted bool, reason string) {
	ack := domain.TaskAckPayload{TaskID: taskID, Accepted: accepted, Reason: reason}
	env, err := domain.NewEnvelope(ackTypeTaskAck, ack)
	if err != nil {
		return
	}
	ackCtx, cancel := context.WithTimeout(context.Background(), writeTimeout)
	defer cancel()
	if err := conn.Write(ackCtx, env); err != nil {
		d.logger.Warn("send task_ack failed",
			zap.String("task_id", taskID),
			zap.Error(err),
		)
	}
}

func (d *Dispatcher) handleDeviceAdd(msg *domain.Envelope) {
	var dev domain.Device
	if err := json.Unmarshal(msg.Payload, &dev); err != nil {
		d.logger.Warn("device_add payload unmarshal failed", zap.Error(err))
		return
	}
	if err := d.deviceMgr.Add(&dev); err != nil {
		d.logger.Warn("device_add failed",
			zap.String("device_id", dev.DeviceID),
			zap.Error(err),
		)
	}
}

func (d *Dispatcher) handleDeviceUpdate(msg *domain.Envelope) {
	var dev domain.Device
	if err := json.Unmarshal(msg.Payload, &dev); err != nil {
		d.logger.Warn("device_update payload unmarshal failed", zap.Error(err))
		return
	}
	if err := d.deviceMgr.Update(&dev); err != nil {
		d.logger.Warn("device_update failed",
			zap.String("device_id", dev.DeviceID),
			zap.Error(err),
		)
	}
}

func (d *Dispatcher) handleDeviceRemove(msg *domain.Envelope) {
	var p struct {
		DeviceID string `json:"device_id"`
	}
	if err := json.Unmarshal(msg.Payload, &p); err != nil {
		d.logger.Warn("device_remove payload unmarshal failed", zap.Error(err))
		return
	}
	if err := d.deviceMgr.Remove(p.DeviceID); err != nil {
		d.logger.Warn("device_remove failed",
			zap.String("device_id", p.DeviceID),
			zap.Error(err),
		)
	}
}

func (d *Dispatcher) handleControl(msg *domain.Envelope) {
	var p struct {
		Action   string `json:"action"`
		DeviceID string `json:"device_id"`
		TaskID   string `json:"task_id"`
	}
	if err := json.Unmarshal(msg.Payload, &p); err != nil {
		d.logger.Warn("control payload unmarshal failed", zap.Error(err))
		return
	}
	switch p.Action {
	case controlCancel:
		if err := d.queue.Cancel(p.DeviceID, p.TaskID); err != nil {
			d.logger.Warn("control cancel failed",
				zap.String("device_id", p.DeviceID),
				zap.String("task_id", p.TaskID),
				zap.Error(err),
			)
		}
	case controlTestPage, controlQueryQueue:
		d.logger.Debug("control action placeholder",
			zap.String("action", p.Action),
		)
	default:
		d.logger.Warn("unknown control action", zap.String("action", p.Action))
	}
}

func (d *Dispatcher) handleConfigUpdate(ctx context.Context, msg *domain.Envelope) {
	if d.domainUpd == nil {
		d.logger.Warn("domain updater not configured")
		return
	}
	var p struct {
		Endpoint string `json:"endpoint"`
	}
	if err := json.Unmarshal(msg.Payload, &p); err != nil {
		d.logger.Warn("config_update payload unmarshal failed", zap.Error(err))
		_ = d.reporter.ReportConfigAck(false, "bad payload", "cloud.endpoint")
		return
	}
	if err := d.domainUpd.UpdateDomain(ctx, p.Endpoint); err != nil {
		d.logger.Warn("domain update failed",
			zap.String("endpoint", p.Endpoint),
			zap.Error(err),
		)
		return
	}
}
