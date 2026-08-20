package cloudlink

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"nhooyr.io/websocket"
	"go.uber.org/zap"

	"github.com/cloud-print/agent/internal/domain"
	"github.com/cloud-print/agent/internal/errs"
)

const (
	cloudPath       = "/agent"
	cloudProtocolV  = "1"
	cloudPort       = 443
	writeTimeout    = 10 * time.Second
	readTimeout     = 60 * time.Second
)

type Client struct {
	resolver *Resolver
	agentID  string
	logger   *zap.Logger

	port            int
	protocol        string
	insecureSkip    bool
}

func NewClient(agentID string, resolver *Resolver, logger *zap.Logger) *Client {
	if logger == nil {
		logger = zap.NewNop()
	}
	if resolver == nil {
		resolver = NewResolver(logger)
	}

	port := cloudPort
	if v := os.Getenv("CPA_CLOUD_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil && p > 0 {
			port = p
		}
	}

	protocol := "wss"
	if v := os.Getenv("CPA_CLOUD_PROTOCOL"); v != "" {
		protocol = v
	}

	insecureSkip := false
	if v := os.Getenv("CPA_TLS_INSECURE"); v == "true" || v == "1" {
		insecureSkip = true
	}

	return &Client{
		resolver:    resolver,
		agentID:     agentID,
		logger:      logger,
		port:        port,
		protocol:    protocol,
		insecureSkip: insecureSkip,
	}
}

func (c *Client) buildWSSURL(endpoint string, token string) string {
	u := url.URL{
		Scheme: c.protocol,
		Host:   fmt.Sprintf("%s:%d", endpoint, c.port),
		Path:   cloudPath,
	}
	q := u.Query()
	q.Set("v", cloudProtocolV)
	q.Set("agent_id", c.agentID)
	q.Set("token", token)
	u.RawQuery = q.Encode()
	return u.String()
}

func (c *Client) dialOptions(token string) *websocket.DialOptions {
	tlsConfig := &tls.Config{
		MinVersion:         tls.VersionTLS12,
		ServerName:         "",
		InsecureSkipVerify: c.insecureSkip,
	}
	httpClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
		},
	}
	return &websocket.DialOptions{
		HTTPHeader: http.Header{
			"Authorization": []string{fmt.Sprintf("Bearer %s", token)},
		},
		HTTPClient: httpClient,
	}
}

func (c *Client) Dial(ctx context.Context, endpoint string, token string) (*Conn, error) {
	ip, lat, err := c.resolver.Resolve(ctx, endpoint)
	if err != nil {
		return nil, err
	}
	c.logger.Debug("dns resolved before dial",
		zap.String("endpoint", endpoint),
		zap.String("ip", ip),
		zap.Int("latency_ms", lat),
	)

	wssURL := c.buildWSSURL(endpoint, token)
	opts := c.dialOptions(token)
	if opts.HTTPClient != nil {
		if tr, ok := opts.HTTPClient.Transport.(*http.Transport); ok {
			if tr.TLSClientConfig != nil {
				tr.TLSClientConfig.ServerName = endpoint
			}
		}
	}

	dialCtx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()

	wsConn, resp, err := websocket.Dial(dialCtx, wssURL, opts)
	if err != nil {
		if resp != nil {
			_ = resp.Body.Close()
		}
		c.logger.Warn("wss dial failed",
			zap.String("endpoint", endpoint),
			zap.String("url", wssURL),
			zap.Error(err),
		)
		return nil, errs.Wrap(errs.ErrWSSHandshakeFail, "wss dial "+wssURL, err)
	}

	conn := &Conn{
		ws:       wsConn,
		endpoint: endpoint,
		resolved: ip,
		closed:   false,
	}
	c.logger.Info("wss connected",
		zap.String("endpoint", endpoint),
		zap.String("ip", ip),
	)
	return conn, nil
}

type Conn struct {
	mu       sync.Mutex
	ws       *websocket.Conn
	endpoint string
	resolved string
	closed   bool
}

func (c *Conn) Endpoint() string {
	return c.endpoint
}

func (c *Conn) ResolvedIP() string {
	return c.resolved
}

func (c *Conn) Read(ctx context.Context, msg *domain.Envelope) error {
	if c.closed {
		return errs.New(errs.ErrCloudDisconnected, "conn closed")
	}
	_, reader, err := c.ws.Reader(ctx)
	if err != nil {
		return c.wrapReadErr(err)
	}
	dec := json.NewDecoder(reader)
	if err := dec.Decode(msg); err != nil {
		if err == io.EOF {
			return errs.New(errs.ErrCloudDisconnected, "conn eof")
		}
		return errs.Wrap(errs.ErrCloudDisconnected, "read envelope", err)
	}
	return nil
}

func (c *Conn) Write(ctx context.Context, msg *domain.Envelope) error {
	if c.closed {
		return errs.New(errs.ErrCloudDisconnected, "conn closed")
	}
	writer, err := c.ws.Writer(ctx, websocket.MessageText)
	if err != nil {
		return c.wrapWriteErr(err)
	}
	enc := json.NewEncoder(writer)
	if err := enc.Encode(msg); err != nil {
		return errs.Wrap(errs.ErrCloudDisconnected, "write envelope", err)
	}
	if err := writer.Close(); err != nil {
		return errs.Wrap(errs.ErrCloudDisconnected, "close writer", err)
	}
	return nil
}

func (c *Conn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	c.closed = true
	return c.ws.Close(websocket.StatusNormalClosure, "bye")
}

func (c *Conn) wrapReadErr(err error) error {
	if err == nil {
		return nil
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "tls") {
		return errs.Wrap(errs.ErrTLSVerifyFail, "tls read", err)
	}
	return errs.Wrap(errs.ErrCloudDisconnected, "read", err)
}

func (c *Conn) wrapWriteErr(err error) error {
	if err == nil {
		return nil
	}
	return errs.Wrap(errs.ErrCloudDisconnected, "write", err)
}