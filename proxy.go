package netx

import (
	"net"

	"github.com/pires/go-proxyproto"
)

// WithProxyProtocol 启用 HAProxy PROXY 协议解析。
// trustedCIDRs: 允许发送 PROXY 头的来源 IP 网段 (如 "10.0.0.0/8", "127.0.0.1/32")。
// 如果连接来自非信任 IP，将跳过 PROXY 解析（视为普通 TCP 连接），防止 IP 伪造。
func WithProxyProtocol(trustedCIDRs []string) Middleware {
	// 1. 预解析 CIDR，避免每次连接都解析
	var allowedNets []*net.IPNet
	for _, cidr := range trustedCIDRs {
		_, n, err := net.ParseCIDR(cidr)
		if err == nil {
			allowedNets = append(allowedNets, n)
		}
	}

	// 辅助函数：检查 IP 是否在白名单中
	isTrusted := func(addr net.Addr) bool {
		// 如果没有配置白名单，默认不允许任何 PROXY 解析 (安全优先)
		if len(allowedNets) == 0 {
			return false
		}

		tcpAddr, ok := addr.(*net.TCPAddr)
		if !ok {
			return false
		}
		ip := tcpAddr.IP

		for _, n := range allowedNets {
			if n.Contains(ip) {
				return true
			}
		}
		return false
	}

	return func(l net.Listener) net.Listener {
		return &proxyproto.Listener{
			Listener: l,
			Policy: func(upstream net.Addr) (proxyproto.Policy, error) {
				if isTrusted(upstream) {
					return proxyproto.USE, nil
				}
				// 如果来源不可信，跳过解析，将其视为普通连接
				// 这样攻击者无法通过伪造头来欺骗 RealIP
				return proxyproto.SKIP, nil
			},
		}
	}
}
