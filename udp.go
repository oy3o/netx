package netx

import (
	"net"

	"github.com/rs/zerolog/log"
)

// WithUDPBuffer 设置内核缓冲区大小
// QUIC 协议强烈建议增大 UDP 缓冲区以提升吞吐量并减少丢包
func WithUDPBuffer(readBuf, writeBuf int) UDPMiddleware {
	return func(c net.PacketConn) net.PacketConn {
		// 尝试断言为 *net.UDPConn
		if udp, ok := c.(*net.UDPConn); ok {
			if readBuf > 0 {
				if err := udp.SetReadBuffer(readBuf); err != nil {
					log.Warn().Err(err).Msgf("failed to set UDP read buffer to %d", readBuf)
				}
			}
			if writeBuf > 0 {
				if err := udp.SetWriteBuffer(writeBuf); err != nil {
					log.Warn().Err(err).Msgf("failed to set UDP write buffer to %d", writeBuf)
				}
			}
		}
		// 此中间件不包装结构体，只做配置副作用，直接返回原连接
		return c
	}
}
