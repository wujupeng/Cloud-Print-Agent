package cloudlink

import (
	"context"
	"net"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/cloud-print/agent/internal/config"
	"github.com/cloud-print/agent/internal/domain"
	"github.com/cloud-print/agent/internal/errs"
	"github.com/cloud-print/agent/internal/observability"
)

const (
	domainValidateTimeout = 10 * time.Second
	domainProbeTimeout    = 10 * time.Second
)

type DomainUpdater struct {
	cfgPath    string
	cfg        *domain.AgentConfig
	cred       credentialProvider
	resolver   *Resolver
	client     *Client
	reporter   *Reporter
	logger     *zap.Logger
	audit      *observability.AuditLogger
	switchConn func(ctx context.Context, newEndpoint string) error

	mu sync.Mutex
}

type credentialProvider interface {
	GetDeviceToken() string
}

func NewDomainUpdater(
	cfgPath string,
	cfg *domain.AgentConfig,
	cred credentialProvider,
	resolver *Resolver,
	client *Client,
	reporter *Reporter,
	logger *zap.Logger,
	audit *observability.AuditLogger,
	switchConn func(ctx context.Context, newEndpoint string) error,
) *DomainUpdater {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &DomainUpdater{
		cfgPath:    cfgPath,
		cfg:        cfg,
		cred:       cred,
		resolver:   resolver,
		client:     client,
		reporter:   reporter,
		logger:     logger,
		audit:      audit,
		switchConn: switchConn,
	}
}

func (u *DomainUpdater) UpdateDomain(ctx context.Context, newEndpoint string) error {
	u.mu.Lock()
	defer u.mu.Unlock()

	traceID := observability.TraceIDFromCtx(ctx)

	if !isValidDomain(newEndpoint) {
		u.logger.Warn("domain update rejected: invalid domain",
			zap.String("endpoint", newEndpoint),
		)
		_ = u.reporter.ReportConfigAck(false, "invalid domain", "cloud.endpoint")
		return errs.Newf(errs.ErrDomainUpdateFail, "invalid domain %q", newEndpoint)
	}

	oldEndpoint := u.cfg.Cloud.Endpoint
	u.logger.Info("domain update start",
		zap.String("old", oldEndpoint),
		zap.String("new", newEndpoint),
		zap.String("trace_id", traceID),
	)

	if err := u.validateAndProbe(ctx, newEndpoint); err != nil {
		u.logger.Warn("domain update probe failed, rollback",
			zap.String("endpoint", newEndpoint),
			zap.Error(err),
		)
		if u.audit != nil {
			u.audit.LogDomainRollback(newEndpoint, traceID)
		}
		_ = u.reporter.ReportConfigAck(false, err.Error(), "cloud.endpoint")
		return err
	}

	backup := u.cfg.Cloud.Endpoint
	u.cfg.Cloud.Endpoint = newEndpoint
	u.cfg.Cloud.DerivedURL = DeriveURL(u.cfg.Cloud.Protocol, newEndpoint)

	if err := config.SaveAtomic(u.cfgPath, u.cfg); err != nil {
		u.cfg.Cloud.Endpoint = backup
		u.cfg.Cloud.DerivedURL = DeriveURL(u.cfg.Cloud.Protocol, backup)
		u.logger.Warn("domain update persist failed, rollback",
			zap.Error(err),
		)
		if u.audit != nil {
			u.audit.LogDomainRollback(newEndpoint, traceID)
		}
		_ = u.reporter.ReportConfigAck(false, "persist failed", "cloud.endpoint")
		return err
	}

	if u.switchConn != nil {
		switchCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		if err := u.switchConn(switchCtx, newEndpoint); err != nil {
			u.logger.Warn("domain update switch conn failed, keep old",
				zap.Error(err),
			)
			u.cfg.Cloud.Endpoint = backup
			u.cfg.Cloud.DerivedURL = DeriveURL(u.cfg.Cloud.Protocol, backup)
			_ = config.SaveAtomic(u.cfgPath, u.cfg)
			if u.audit != nil {
				u.audit.LogDomainRollback(newEndpoint, traceID)
			}
			_ = u.reporter.ReportConfigAck(false, "switch conn failed", "cloud.endpoint")
			return err
		}
	}

	if u.audit != nil {
		u.audit.LogDomainUpdate(newEndpoint, traceID)
	}
	u.logger.Info("domain update applied",
		zap.String("endpoint", newEndpoint),
	)
	_ = u.reporter.ReportConfigAck(true, "", "cloud.endpoint")
	return nil
}

func (u *DomainUpdater) validateAndProbe(ctx context.Context, endpoint string) error {
	_, cancel1 := context.WithTimeout(ctx, domainValidateTimeout)
	defer cancel1()
	if !isValidDomain(endpoint) {
		return errs.Newf(errs.ErrDomainUpdateFail, "invalid domain %q", endpoint)
	}

	probeCtx, cancel2 := context.WithTimeout(ctx, domainProbeTimeout)
	defer cancel2()
	ip, _, err := u.resolver.Resolve(probeCtx, endpoint)
	if err != nil {
		return err
	}
	u.logger.Debug("domain update dns ok",
		zap.String("endpoint", endpoint),
		zap.String("ip", ip),
	)

	token := ""
	if u.cred != nil {
		token = u.cred.GetDeviceToken()
	}
	dialCtx, cancel3 := context.WithTimeout(ctx, 15*time.Second)
	defer cancel3()
	conn, err := u.client.Dial(dialCtx, endpoint, token)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()

	handshakeCtx, cancel4 := context.WithTimeout(ctx, 10*time.Second)
	defer cancel4()
	var resp domain.Envelope
	if err := conn.Read(handshakeCtx, &resp); err != nil {
		return errs.Wrap(errs.ErrWSSHandshakeFail, "read handshake", err)
	}
	if _, herr := HandleHandshakeResponse(&resp); herr != nil {
		return herr
	}
	return nil
}

func isValidDomain(s string) bool {
	if s == "" || len(s) > 253 {
		return false
	}
	if strings.Contains(s, "://") {
		return false
	}
	if net.ParseIP(s) != nil {
		return false
	}
	for _, label := range strings.Split(s, ".") {
		if label == "" || len(label) > 63 {
			return false
		}
	}
	return true
}