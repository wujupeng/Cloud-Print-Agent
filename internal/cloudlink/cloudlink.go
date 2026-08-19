package cloudlink

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/cloud-print/agent/internal/credential"
	"github.com/cloud-print/agent/internal/device"
	"github.com/cloud-print/agent/internal/domain"
	"github.com/cloud-print/agent/internal/errs"
	"github.com/cloud-print/agent/internal/executor"
	"github.com/cloud-print/agent/internal/netprobe"
	"github.com/cloud-print/agent/internal/observability"
	"github.com/cloud-print/agent/internal/taskqueue"
)

type CloudLink struct {
	cfg       *domain.AgentConfig
	cfgPath   string
	cred      *credential.Manager
	queue     *taskqueue.Queue
	deviceMgr *device.Manager
	executor  *executor.Executor
	prober    netprobe.Prober
	metrics   *observability.NetMetrics
	logger    *zap.Logger
	audit     *observability.AuditLogger

	resolver    *Resolver
	client      *Client
	reporter    *Reporter
	heartbeat   *Heartbeat
	reconnect   *Reconnect
	dispatcher  *Dispatcher
	domainUpd   *DomainUpdater

	mu       sync.Mutex
	conn     *Conn
	running  bool
	cancel   context.CancelFunc

	stopOnce sync.Once
}

func NewCloudLink(
	cfg *domain.AgentConfig,
	cfgPath string,
	cred *credential.Manager,
	queue *taskqueue.Queue,
	deviceMgr *device.Manager,
	executor *executor.Executor,
	prober netprobe.Prober,
	metrics *observability.NetMetrics,
	logger *zap.Logger,
	audit *observability.AuditLogger,
) *CloudLink {
	if logger == nil {
		logger = zap.NewNop()
	}
	cl := &CloudLink{
		cfg:       cfg,
		cfgPath:   cfgPath,
		cred:      cred,
		queue:     queue,
		deviceMgr: deviceMgr,
		executor:  executor,
		prober:    prober,
		metrics:   metrics,
		logger:    logger,
		audit:     audit,
	}
	cl.init()
	return cl
}

func (c *CloudLink) init() {
	c.resolver = NewResolver(c.logger)
	c.client = NewClient(c.cfg.Cloud.AgentID, c.resolver, c.logger)
	c.reporter = NewReporter(nil, c.metrics, c.logger)

	c.domainUpd = NewDomainUpdater(
		c.cfgPath, c.cfg, c.cred, c.resolver, c.client, c.reporter,
		c.logger, c.audit, c.switchConn,
	)
	c.dispatcher = NewDispatcher(
		nil, c.queue, c.deviceMgr, c.reporter, c.domainUpd, c.logger,
	)
	c.heartbeat = NewHeartbeat(
		HeartbeatConfig{
			AgentID:       c.cfg.Cloud.AgentID,
			Version:       domain.Version,
			CloudEndpoint: c.cfg.Cloud.Endpoint,
			Interval:      c.cfg.HeartbeatInterval,
		},
		nil, c.reporter, c.metrics, c.queue, c.deviceMgr, c.logger,
	)
	c.reconnect = NewReconnect(c.prober, c.logger, c.reconnectFn)
	c.heartbeat.OnConnLoss(func() {
		c.reconnect.HandleDisconnect(context.Background(), errs.New(errs.ErrCloudDisconnected, "heartbeat lost"))
	})
}

func (c *CloudLink) Start(ctx context.Context) error {
	c.mu.Lock()
	if c.running {
		c.mu.Unlock()
		return nil
	}
	subCtx, cancel := context.WithCancel(ctx)
	c.cancel = cancel

	c.running = true
	c.mu.Unlock()

	if err := c.connect(subCtx); err != nil {
		return err
	}
	c.heartbeat.Start(subCtx)
	c.dispatcher.Start(subCtx)
	c.logger.Info("cloudlink started",
		zap.String("endpoint", c.cfg.Cloud.Endpoint),
	)
	return nil
}

func (c *CloudLink) connect(ctx context.Context) error {
	token := ""
	if c.cred != nil {
		token = c.cred.GetDeviceToken()
	}
	conn, err := c.client.Dial(ctx, c.cfg.Cloud.Endpoint, token)
	if err != nil {
		return err
	}

	var handshake domain.Envelope
	hsCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := conn.Read(hsCtx, &handshake); err != nil {
		_ = conn.Close()
		return errs.Wrap(errs.ErrWSSHandshakeFail, "read handshake", err)
	}
	if _, herr := HandleHandshakeResponse(&handshake); herr != nil {
		_ = conn.Close()
		return herr
	}

	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()
	c.reporter.UpdateConn(conn)
	c.heartbeat.UpdateConn(conn)
	c.dispatcher.UpdateConn(conn)
	return nil
}

func (c *CloudLink) reconnectFn(ctx context.Context) error {
	if err := c.connect(ctx); err != nil {
		c.logger.Warn("reconnect failed", zap.Error(err))
		return err
	}
	c.flushPendingResults()
	return nil
}

func (c *CloudLink) flushPendingResults() {
	c.mu.Lock()
	conn := c.conn
	c.mu.Unlock()
	if conn == nil {
		return
	}
	for _, dev := range c.deviceMgr.List() {
		for _, task := range c.queue.ListPending(dev.DeviceID) {
			if task.Status.IsTerminal() {
				_ = c.reporter.ReportTaskResult(task)
			}
		}
	}
}

func (c *CloudLink) switchConn(ctx context.Context, newEndpoint string) error {
	c.mu.Lock()
	oldConn := c.conn
	c.mu.Unlock()
	if oldConn != nil {
		_ = oldConn.Close()
	}

	c.cfg.Cloud.Endpoint = newEndpoint
	c.cfg.Cloud.DerivedURL = DeriveURL(c.cfg.Cloud.Protocol, newEndpoint)
	c.heartbeat.cfg.CloudEndpoint = newEndpoint

	if err := c.connect(ctx); err != nil {
		return err
	}
	c.logger.Info("switched to new domain", zap.String("endpoint", newEndpoint))
	return nil
}

func (c *CloudLink) Reporter() *Reporter { return c.reporter }

func (c *CloudLink) Stop() {
	c.stopOnce.Do(func() {
		c.mu.Lock()
		c.running = false
		cancel := c.cancel
		conn := c.conn
		c.mu.Unlock()

		if c.heartbeat != nil {
			c.heartbeat.Stop()
		}
		if c.dispatcher != nil {
			c.dispatcher.Stop()
		}
		if c.reconnect != nil {
			c.reconnect.Stop()
		}
		if cancel != nil {
			cancel()
		}
		if conn != nil {
			_ = conn.Close()
		}
		c.logger.Info("cloudlink stopped")
	})
}