package lifecycle

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/cloud-print/agent/internal/cloudlink"
	"github.com/cloud-print/agent/internal/config"
	"github.com/cloud-print/agent/internal/credential"
	"github.com/cloud-print/agent/internal/device"
	"github.com/cloud-print/agent/internal/domain"
	"github.com/cloud-print/agent/internal/errs"
	"github.com/cloud-print/agent/internal/executor"
	"github.com/cloud-print/agent/internal/netprobe"
	"github.com/cloud-print/agent/internal/observability"
	"github.com/cloud-print/agent/internal/opsapi"
	"github.com/cloud-print/agent/internal/storage"
	"github.com/cloud-print/agent/internal/taskqueue"
)

const (
	credentialEnvKey   = "CPA_MASTER_KEY"
	credentialEncPath  = "/etc/cloud-print-agent/credentials.enc"
	shutdownMaxWait    = 10 * time.Second
	storageDBFilename  = "agent.db"
	defaultWorkerCount = 5
)

type App struct {
	cfg       *domain.AgentConfig
	cfgPath   string
	logger    *zap.Logger
	audit     *observability.AuditLogger
	metrics   *observability.NetMetrics
	db        *storage.DB
	repo      *storage.TaskRepo
	prober    *netprobe.LoopProber
	deviceMgr *device.Manager
	queue     *taskqueue.Queue
	executor  *executor.Executor
	cloudLink *cloudlink.CloudLink
	opsServer *opsapi.Server
	cred      *credential.Manager

	pendingConfig bool
	mu            sync.Mutex
}

func Run(configPath string) error {
	app := &App{cfgPath: configPath}

	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config failed: %v\n", err)
		return err
	}
	app.cfg = cfg

	logger, err := observability.NewLogger(cfg.Storage.LogDir, cfg.Log.Level)
	if err != nil {
		fmt.Fprintf(os.Stderr, "init logger failed: %v\n", err)
		return err
	}
	app.logger = logger
	defer logger.Sync()

	audit, err := observability.NewAuditLogger(cfg.Storage.LogDir)
	if err != nil {
		logger.Error("init audit logger failed", zap.Error(err))
		return err
	}
	app.audit = audit
	defer audit.Close()

	app.metrics = observability.NewNetMetrics()

	logger.Info("starting cloud print agent",
		zap.String("version", domain.Version),
		zap.String("endpoint", cfg.Cloud.Endpoint),
		zap.String("config_path", configPath),
	)

	if err := app.initStorage(); err != nil {
		logger.Error("init storage failed", zap.Error(err))
		return err
	}

	app.deviceMgr = device.NewManager(app.logger, app.repo)
	for i := range cfg.Devices {
		dev := cfg.Devices[i]
		if err := app.deviceMgr.Add(&dev); err != nil {
			logger.Warn("add device from config failed",
				zap.String("device_id", dev.DeviceID),
				zap.Error(err),
			)
		}
	}

	app.queue = taskqueue.NewQueue(cfg.QueueCapacity, app.repo, app.logger)
	recoverer := taskqueue.NewRecoverer(app.queue, app.repo, app.logger)
	if err := recoverer.Recover(); err != nil {
		logger.Warn("recover tasks failed", zap.Error(err))
	}

	cred, credErr := credential.NewManager(credentialEnvKey, credentialEncPath)
	if credErr != nil {
		logger.Warn("load credential failed, entering PendingConfig mode",
			zap.Error(credErr),
		)
		app.pendingConfig = true
	} else {
		app.cred = cred
		logger.Info("credential loaded")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if app.pendingConfig {
		return app.runPendingConfig(ctx)
	}

	app.prober = netprobe.NewLoopProber(
		cfg.Cloud.Endpoint, "", cfg.CloudOutboundPort,
		app.metrics, app.logger, app.audit,
	)
	app.prober.Start(ctx)

	app.executor = executor.NewExecutor(app.queue, app.deviceMgr, app.logger, app.audit)
	app.executor.Start(ctx, defaultWorkerCount)

	app.cloudLink = cloudlink.NewCloudLink(
		app.cfg, app.cfgPath, app.cred, app.queue, app.deviceMgr,
		app.executor, app.prober, app.metrics, app.logger, app.audit,
	)
	app.executor.OnTaskResult(func(task *domain.PrintTask) {
		_ = app.cloudLink.Reporter().ReportTaskResult(task)
	})
	if err := app.cloudLink.Start(ctx); err != nil {
		logger.Error("cloudlink start failed", zap.Error(err))
	}

	app.opsServer = opsapi.NewServer(
		app.cfg, app.deviceMgr, app.queue, app.metrics, app.logger, app.audit,
	)
	if err := app.opsServer.Start(ctx); err != nil {
		logger.Error("ops api start failed", zap.Error(err))
		return err
	}

	if err := NotifyReady(); err != nil {
		logger.Warn("sd notify ready failed", zap.Error(err))
	}

	if interval, ok := WatchdogInterval(); ok {
		go StartWatchdog(ctx, interval/3)
		logger.Info("watchdog enabled", zap.Duration("interval", interval/3))
	}

	logger.Info("agent started",
		zap.String("endpoint", cfg.Cloud.Endpoint),
		zap.Int("devices", app.deviceMgr.Count()),
	)

	sig := WaitForSignal(ctx)
	logger.Info("received signal, shutting down", zap.String("signal", sig.String()))

	app.Shutdown()
	return nil
}

func (a *App) runPendingConfig(ctx context.Context) error {
	a.opsServer = opsapi.NewServer(
		a.cfg, a.deviceMgr, a.queue, a.metrics, a.logger, a.audit,
	)
	if err := a.opsServer.Start(ctx); err != nil {
		a.logger.Error("ops api start failed (pending config)", zap.Error(err))
		return err
	}

	if err := NotifyReady(); err != nil {
		a.logger.Warn("sd notify ready failed", zap.Error(err))
	}

	a.logger.Info("agent started in PendingConfig mode, waiting for credential injection")

	sig := WaitForSignal(ctx)
	a.logger.Info("received signal, shutting down", zap.String("signal", sig.String()))

	a.Shutdown()
	return nil
}

func (a *App) initStorage() error {
	dbPath := filepath.Join(a.cfg.Storage.DataDir, storageDBFilename)
	db, err := storage.Open(dbPath)
	if err != nil {
		return errs.Wrap(errs.ErrStorageIO, "open storage db", err)
	}
	a.db = db
	a.repo = storage.NewTaskRepo(db)
	return nil
}

func (a *App) Shutdown() {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.logger.Info("shutdown begin", zap.Duration("max_wait", shutdownMaxWait))

	done := make(chan struct{})
	go func() {
		defer close(done)

		if a.cloudLink != nil {
			a.cloudLink.Stop()
		}
		if a.executor != nil {
			a.executor.Stop()
		}
		if a.prober != nil {
			a.prober.Stop()
		}
		if a.queue != nil {
			a.queue.Close()
		}
		if a.opsServer != nil {
			a.opsServer.Stop()
		}
		if a.db != nil {
			if err := a.db.Close(); err != nil {
				a.logger.Warn("close storage failed", zap.Error(err))
			}
		}
	}()

	select {
	case <-done:
		a.logger.Info("shutdown complete")
	case <-time.After(shutdownMaxWait):
		a.logger.Warn("shutdown timeout, forcing exit", zap.Duration("waited", shutdownMaxWait))
	}
}