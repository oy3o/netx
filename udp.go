package netx

import "net"

// WithUDPBuffer 设置内核缓冲区大小
// QUIC 协议强烈建议增大 UDP 缓冲区以提升吞吐量并减少丢包
func WithUDPBuffer(readBuf, writeBuf int) UDPMiddleware {
	return func(c net.PacketConn) net.PacketConn {
		// 尝试断言为 *net.UDPConn
		if udp, ok := c.(*net.UDPConn); ok {
			if readBuf > 0 {
				_ = udp.SetReadBuffer(readBuf)
			}
			if writeBuf > 0 {
				_ = udp.SetWriteBuffer(writeBuf)
			}
		}
		// 此中间件不包装结构体，只做配置副作用，直接返回原连接
		// 这样可以保留底层的接口实现（如 SyscallConn）
		return c
	}
}
