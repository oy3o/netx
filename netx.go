package netx

import (
	"context"
	"crypto/tls"
	"net"
)

// Wrapper 接口允许解包底层的连接。
// 所有 netx 的装饰器连接都必须实现此接口，以支持上下文穿透。
type Wrapper interface {
	Unwrap() net.Conn
}

// ContextGetter 接口用于直接获取连接绑定的上下文。
type ContextGetter interface {
	Context() context.Context
}

// Middleware 定义了 Listener 的装饰器函数签名。
type Middleware func(net.Listener) net.Listener

// UDPMiddleware 定义 UDP 连接装饰器
type UDPMiddleware func(net.PacketConn) net.PacketConn

// Chain 是一个辅助函数，用于将多个中间件应用到一个 Listener 上。
// 顺序：Chain(l, A, B, C) -> C(B(A(l)))
// 越靠后的中间件越外层（越先处理 Accept）。
func Chain(l net.Listener, mws ...Middleware) net.Listener {
	for _, mw := range mws {
		l = mw(l)
	}
	return l
}

// ChainUDP 串联 UDP 中间件
func ChainUDP(c net.PacketConn, mws ...UDPMiddleware) net.PacketConn {
	for _, mw := range mws {
		c = mw(c)
	}
	return c
}

// AsTCPConn 尝试将 net.Conn 还原为 *net.TCPConn
// 即使被多层 Wrapper 包裹也能找到。
func AsTCPConn(c net.Conn) *net.TCPConn {
	for {
		if tc, ok := c.(*net.TCPConn); ok {
			return tc
		}
		if wrapper, ok := c.(Wrapper); ok {
			c = wrapper.Unwrap()
			continue
		}
		// 增强：处理 TLS 连接，获取底层的 net.Conn
		if tc, ok := c.(*tls.Conn); ok {
			c = tc.NetConn()
			continue
		}
		return nil
	}
}
