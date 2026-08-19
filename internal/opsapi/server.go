package opsapi

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/cloud-print/agent/internal/domain"
	"github.com/cloud-print/agent/internal/device"
	"github.com/cloud-print/agent/internal/observability"
	"github.com/cloud-print/agent/internal/taskqueue"
)

const (
	defaultOpsPort  = 9100
	shutdownTimeout = 5 * time.Second
)

type Server struct {
	cfg       *domain.AgentConfig
	deviceMgr *device.Manager
	queue     *taskqueue.Queue
	netMetrics *observability.NetMetrics
	logger    *zap.Logger
	audit     *observability.AuditLogger

	httpServer *http.Server
	listener   net.Listener
	stopOnce   sync.Once
}

func NewServer(
	cfg *domain.AgentConfig,
	deviceMgr *device.Manager,
	queue *taskqueue.Queue,
	netMetrics *observability.NetMetrics,
	logger *zap.Logger,
	audit *observability.AuditLogger,
) *Server {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Server{
		cfg:        cfg,
		deviceMgr:  deviceMgr,
		queue:      queue,
		netMetrics: netMetrics,
		logger:     logger,
		audit:      audit,
	}
}

func (s *Server) buildRouter() http.Handler {
	r := chi.NewRouter()
	r.Use(LocalOnly(s.audit, s.logger))

	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/healthz", s.handleHealthz)
		r.Get("/status", s.handleStatus)
		r.Get("/network", s.handleNetwork)
		r.Get("/devices", s.handleDevices)
		r.Get("/queues", s.handleQueues)
		r.Get("/queues/{device_id}", s.handleQueueDetail)
		r.Get("/logs", s.handleLogs)
		r.Get("/config", s.handleConfig)
	})
	return r
}

func (s *Server) Start(ctx context.Context) error {
	port := s.cfg.Ops.Port
	if port <= 0 {
		port = defaultOpsPort
	}
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		s.logger.Error("ops api listen failed", zap.Error(err))
		return err
	}
	s.listener = ln

	s.httpServer = &http.Server{
		Handler:           s.buildRouter(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		s.logger.Info("ops api serving", zap.String("addr", addr))
		if err := s.httpServer.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.logger.Error("ops api serve error", zap.Error(err))
		}
	}()

	return nil
}

func (s *Server) Stop() {
	s.stopOnce.Do(func() {
		if s.httpServer != nil {
			ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
			defer cancel()
			if err := s.httpServer.Shutdown(ctx); err != nil {
				s.logger.Warn("ops api shutdown error", zap.Error(err))
			}
		}
		s.logger.Info("ops api stopped")
	})
}