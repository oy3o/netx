package netx

import (
	"net"
	"sync"
)

// WithLimit 返回一个中间件，限制最大并发连接数。
func WithLimit(n int) Middleware {
	return func(l net.Listener) net.Listener {
		return &limitListener{
			Listener: l,
			sem:      make(chan struct{}, n),
			closeCh:  make(chan struct{}),
		}
	}
}

type limitListener struct {
	net.Listener
	sem     chan struct{}
	closeCh chan struct{}
	once    sync.Once // 确保 Close 只关一次 channel
}

func (l *limitListener) Accept() (net.Conn, error) {
	// 获取信号量 (阻塞直到有名额)
	select {
	case l.sem <- struct{}{}:
		// 获取成功
	case <-l.closeCh:
		// Listener 已关闭，返回标准 error
		return nil, net.ErrClosed
	}

	c, err := l.Listener.Accept()
	if err != nil {
		<-l.sem // 获取失败，归还名额
		return nil, err
	}
	return &limitConn{Conn: c, release: l.release}, nil
}

func (l *limitListener) Close() error {
	l.once.Do(func() {
		close(l.closeCh)
	})
	return l.Listener.Close()
}

func (l *limitListener) release() { <-l.sem }

type limitConn struct {
	net.Conn
	releaseOnce sync.Once
	release     func()
}

func (c *limitConn) Unwrap() net.Conn { return c.Conn }

func (c *limitConn) Close() error {
	err := c.Conn.Close()
	c.releaseOnce.Do(c.release) // 确保只归还一次
	return err
}
