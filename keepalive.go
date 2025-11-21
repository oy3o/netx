package netx

import (
	"net"
	"time"
)

// WithKeepAlive 返回一个中间件，自动开启 TCP KeepAlive。
// period: 探测间隔。如果为 0，默认使用 3 分钟。
func WithKeepAlive(period time.Duration) Middleware {
	return func(l net.Listener) net.Listener {
		return &keepAliveListener{
			Listener: l,
			period:   period,
		}
	}
}

type keepAliveListener struct {
	net.Listener
	period time.Duration
}

func (l *keepAliveListener) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}

	// 只有 TCP 连接支持 KeepAlive
	if tc := AsTCPConn(c); tc != nil {
		period := l.period
		if period == 0 {
			period = 3 * time.Minute
		}
		_ = tc.SetKeepAlive(true)
		_ = tc.SetKeepAlivePeriod(period)
	}

	return c, nil
}
