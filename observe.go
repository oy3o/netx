package netx

import (
	"net"
	"sync"
)

type ConnState int

const (
	StateOpen  ConnState = iota // 连接建立
	StateClose                  // 连接关闭
)

// Observer 定义了观测回调函数签名
type Observer func(net.Conn, ConnState)

// WithObserve 返回一个中间件，用于监控连接的开启和关闭。
func WithObserve(observer Observer) Middleware {
	return func(l net.Listener) net.Listener {
		return &metricListener{
			Listener: l,
			observer: observer,
		}
	}
}

type metricListener struct {
	net.Listener
	observer Observer
}

func (l *metricListener) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}

	if l.observer != nil {
		l.observer(c, StateOpen)
	}

	return &metricConn{Conn: c, observer: l.observer}, nil
}

type metricConn struct {
	net.Conn
	observer  Observer
	closeOnce sync.Once
}

func (c *metricConn) Unwrap() net.Conn { return c.Conn }

func (c *metricConn) Close() error {
	err := c.Conn.Close()
	c.closeOnce.Do(func() {
		if c.observer != nil {
			c.observer(c.Conn, StateClose)
		}
	})
	return err
}
