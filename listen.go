package netx

import (
	"context"
	"net"
)

// ListenConfig 封装监听配置
type ListenConfig struct {
	// EnableReusePort 开启 SO_REUSEPORT 特性
	// 允许在多核机器上启动多个进程/线程监听同一端口，由内核进行负载均衡
	EnableReusePort bool
}

// ListenTCP 创建一个 TCP 监听器
func ListenTCP(network, address string, cfg ListenConfig) (net.Listener, error) {
	lc := net.ListenConfig{}
	if cfg.EnableReusePort {
		lc.Control = controlReusePort
	}
	return lc.Listen(context.Background(), network, address)
}

// ListenUDP 创建一个 UDP 连接（用于 QUIC/HTTP3）
func ListenUDP(network, address string, cfg ListenConfig) (net.PacketConn, error) {
	lc := net.ListenConfig{}
	if cfg.EnableReusePort {
		lc.Control = controlReusePort
	}
	return lc.ListenPacket(context.Background(), network, address)
}
