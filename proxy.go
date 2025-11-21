package netx

import (
	"net"

	"github.com/pires/go-proxyproto"
)

// WithProxyProtocol 启用 HAProxy PROXY 协议解析。
// 适用于部署在 L4 LB (AWS ELB, Nginx Stream) 后的服务。
func WithProxyProtocol(trustedCIDRs []string) Middleware {
	return func(l net.Listener) net.Listener {
		return &proxyproto.Listener{Listener: l}
	}
}
