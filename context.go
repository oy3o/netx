package netx

import (
	"context"
	"net"
	"sync"
)

// ContextFactory 允许用户自定义 Context 的初始化逻辑（例如注入 TraceID, Logger）。
type ContextFactory func(net.Conn) context.Context

// WithContext 返回一个中间件，用于为每个连接绑定生命周期上下文。
// 当连接 Close 时，Context 会自动 Cancel。
func WithContext(factory ContextFactory) Middleware {
	return func(l net.Listener) net.Listener {
		return &ctxListener{
			Listener: l,
			factory:  factory,
		}
	}
}

// GetContext 递归地尝试从连接中获取上下文。
// 如果连接没有绑定 Context，返回 context.Background()。
func GetContext(c any) context.Context {
	for {
		// 1. 尝试直接获取
		if getter, ok := c.(ContextGetter); ok {
			return getter.Context()
		}

		// 2. 尝试解包
		if wrapper, ok := c.(Wrapper); ok {
			c = wrapper.Unwrap()
			continue
		}

		// 3. 到底了，没有 Context
		return context.Background()
	}
}

type ctxListener struct {
	net.Listener
	factory ContextFactory
}

func (l *ctxListener) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}

	// 1. 初始化 Context
	var ctx context.Context
	if l.factory != nil {
		ctx = l.factory(c)
	} else {
		ctx = context.Background()
	}

	// 2. 绑定生命周期：创建 Cancel 机制
	ctx, cancel := context.WithCancel(ctx)

	return &ctxConn{
		Conn:   c,
		ctx:    ctx,
		cancel: cancel,
	}, nil
}

type ctxConn struct {
	net.Conn
	ctx       context.Context
	cancel    context.CancelFunc
	closeOnce sync.Once
}

// Context 实现 ContextGetter
func (c *ctxConn) Context() context.Context { return c.ctx }

// Unwrap 实现 Wrapper
func (c *ctxConn) Unwrap() net.Conn { return c.Conn }

// Close 覆写 Close，确保连接关闭时 Context 被 Cancel
func (c *ctxConn) Close() error {
	err := c.Conn.Close()
	c.closeOnce.Do(func() {
		c.cancel() // 通知所有监听 ctx.Done() 的组件
	})
	return err
}
