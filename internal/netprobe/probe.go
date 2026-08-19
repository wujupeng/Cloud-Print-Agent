package netprobe

import (
	"context"

	"github.com/cloud-print/agent/internal/domain"
)

type Prober interface {
	Probe(ctx context.Context) (domain.NetClass, domain.NetTopology, error)
}

func Classify(localNetOK bool, dnsOK bool, gatewayOK bool) domain.NetClass {
	if !localNetOK {
		return domain.NetClassLocalNetFail
	}
	if !dnsOK {
		return domain.NetClassDNSResolveFail
	}
	if !gatewayOK {
		return domain.NetClassCloudGatewayUnreachable
	}
	return domain.NetClassOK
}