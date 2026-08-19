package cloudlink

import (
	"encoding/json"
	"strings"

	"github.com/cloud-print/agent/internal/domain"
	"github.com/cloud-print/agent/internal/errs"
)

const (
	handshakeTypeAuthOK    = "auth_ok"
	handshakeTypeAuthFail  = "auth_fail"
	handshakeTypeAuthInvalid = "auth_invalid"
	handshakeTypeError     = "error"
)

type HandshakeResult struct {
	NetClass domain.NetClass
	Reason   string
}

type handshakePayload struct {
	Code    int    `json:"code,omitempty"`
	Reason  string `json:"reason,omitempty"`
	Status  string `json:"status,omitempty"`
	Detail  string `json:"detail,omitempty"`
}

func HandleHandshakeResponse(resp *domain.Envelope) (domain.NetClass, error) {
	if resp == nil {
		return domain.NetClassCloudGatewayUnreachable,
			errs.New(errs.ErrWSSHandshakeFail, "handshake response is nil")
	}

	switch resp.Type {
	case handshakeTypeAuthOK:
		return domain.NetClassOK, nil
	case handshakeTypeAuthFail, handshakeTypeAuthInvalid:
		var p handshakePayload
		_ = json.Unmarshal(resp.Payload, &p)
		reason := p.Reason
		if reason == "" {
			reason = "credential rejected by cloud"
		}
		return domain.NetClassOK, errs.New(errs.ErrAuthInvalid, reason)
	case handshakeTypeError:
		var p handshakePayload
		_ = json.Unmarshal(resp.Payload, &p)
		reason := p.Reason
		if reason == "" {
			reason = p.Detail
		}
		lower := strings.ToLower(reason)
		switch {
		case strings.Contains(lower, "dns"):
			return domain.NetClassDNSResolveFail,
				errs.NewNetError(errs.ErrDNSResolveFail, reason, domain.NetClassDNSResolveFail)
		case strings.Contains(lower, "unreachable") || strings.Contains(lower, "gateway"):
			return domain.NetClassCloudGatewayUnreachable,
				errs.NewNetError(errs.ErrCloudGatewayUnreachable, reason, domain.NetClassCloudGatewayUnreachable)
		case strings.Contains(lower, "local"):
			return domain.NetClassLocalNetFail,
				errs.NewNetError(errs.ErrLocalNetFail, reason, domain.NetClassLocalNetFail)
		}
		return domain.NetClassCloudGatewayUnreachable,
			errs.New(errs.ErrWSSHandshakeFail, reason)
	default:
		var p handshakePayload
		_ = json.Unmarshal(resp.Payload, &p)
		if p.Code == 401 || p.Code == 403 {
			return domain.NetClassOK, errs.New(errs.ErrAuthInvalid, p.Reason)
		}
		return domain.NetClassOK, nil
	}
}