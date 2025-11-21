package netx

import (
	"net"
	"time"
)

// WithIOTimeout 设置读写超时。
// 这是一个非常重要的安全配置，防止客户端建立连接后不发送数据（Slowloris 攻击）。
func WithIOTimeout(read, write time.Duration) Middleware {
	return func(l net.Listener) net.Listener {
		return &timeoutListener{Listener: l, read: read, write: write}
	}
}

type timeoutListener struct {
	net.Listener
	read, write time.Duration
}

func (l *timeoutListener) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	// 初始设置，通常业务层在 Read 循环中需要 refresh deadline
	if l.read > 0 {
		c.SetReadDeadline(time.Now().Add(l.read))
	}
	if l.write > 0 {
		c.SetWriteDeadline(time.Now().Add(l.write))
	}
	return c, nil
}
