package netx

import (
	"context"
	"net"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// Bucket 是一个抽象的令牌桶接口。
// 外部可以使用 time/rate 或 juju/ratelimit 实现此接口。
type Bucket interface {
	// Take 阻塞等待，直到获取到 tokens 个令牌。
	// ctx 可用于取消等待。
	Take(ctx context.Context, tokens int64) error
}

// ShaperFactory 根据连接信息返回读写限速器。
// 如果返回 nil，表示不限制。
type ShaperFactory func(net.Conn) (readBucket, writeBucket Bucket)

// WithShaper 返回一个中间件，用于对连接进行流量整形（限速）。
func WithShaper(factory ShaperFactory) Middleware {
	return func(l net.Listener) net.Listener {
		return &shaperListener{
			Listener: l,
			factory:  factory,
		}
	}
}

type shaperListener struct {
	net.Listener
	factory ShaperFactory
}

func (l *shaperListener) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}

	rB, wB := l.factory(c)
	if rB == nil && wB == nil {
		return c, nil // 无需整形，零开销
	}

	// 尝试获取 Context 用于限速等待时的取消
	ctx := GetContext(c)

	// 防御性编程。如果连接没有绑定 Context（说明 WithContext 中间件缺失或顺序错误），
	// 我们必须手动创建一个与连接生命周期绑定的 Context。
	// 否则，当连接关闭时，阻塞在 Take() 上的 Goroutine 将永远无法释放。
	if ctx == context.Background() {
		// 创建一个新的可取消 Context
		newCtx, cancel := context.WithCancel(context.Background())
		// 包装连接以确保 Close 时调用 cancel
		c = &ctxFallbackConn{
			Conn:   c,
			cancel: cancel,
		}
		ctx = newCtx
	}

	return &throttledConn{
		Conn: c,
		ctx:  ctx,
		rB:   rB,
		wB:   wB,
	}, nil
}

// 用于在缺失 WithContext 时提供兜底的 Context 绑定
type ctxFallbackConn struct {
	net.Conn
	cancel    context.CancelFunc
	closeOnce sync.Once
}

func (c *ctxFallbackConn) Close() error {
	err := c.Conn.Close()
	c.closeOnce.Do(func() {
		c.cancel()
	})
	return err
}

type throttledConn struct {
	net.Conn
	ctx context.Context
	rB  Bucket
	wB  Bucket
}

func (c *throttledConn) Unwrap() net.Conn { return c.Conn }

func (c *throttledConn) Read(p []byte) (n int, err error) {
	// 1. 先读数据
	n, err = c.Conn.Read(p)
	if n > 0 && c.rB != nil {
		// 2. 再拿令牌 (Read-then-Throttle)
		// 这样能更精确地反映实际读取的字节数
		if tErr := c.rB.Take(c.ctx, int64(n)); tErr != nil {
			return n, tErr
		}
	}
	return n, err
}

func (c *throttledConn) Write(p []byte) (n int, err error) {
	// 1. 先拿令牌 (Throttle-then-Write)
	// 确保有带宽再发数据，避免网络拥塞
	lenP := int64(len(p))
	if lenP > 0 && c.wB != nil {
		if tErr := c.wB.Take(c.ctx, lenP); tErr != nil {
			return 0, tErr
		}
	}
	// 2. 再写数据
	return c.Conn.Write(p)
}

// LimitConfig 定义限流配置
type LimitConfig struct {
	Read  *rate.Limiter // 入站限流 (nil 表示不限制)
	Write *rate.Limiter // 出站限流 (nil 表示不限制)
}

// WithPPSLimit 限制 UDP 的每秒包数。
func WithPPSLimit(cfg LimitConfig) UDPMiddleware {
	return func(c net.PacketConn) net.PacketConn {
		// 1. 针对 *net.UDPConn 的优化路径
		if udp, ok := c.(*net.UDPConn); ok {
			return &udpShaperConn{
				UDPConn: udp,
				cfg:     cfg,
			}
		}

		// 2. 通用路径
		return &packetShaperConn{
			PacketConn: c,
			cfg:        cfg,
		}
	}
}

type udpShaperConn struct {
	*net.UDPConn
	cfg LimitConfig
}

// ReadFrom 必须真读，然后丢弃
func (c *udpShaperConn) ReadFrom(p []byte) (int, net.Addr, error) {
	for {
		n, addr, err := c.UDPConn.ReadFrom(p)
		if err != nil {
			return n, addr, err // 包含超时错误、Socket 关闭错误等
		}

		// 如果没有配置限流器，或者令牌允许，则返回
		if c.cfg.Read == nil || c.cfg.Read.Allow() {
			return n, addr, nil
		}

		// 限流触发：静默丢弃
		// 增加短暂休眠，防止在泛洪攻击下 CPU 占用率飙升到 100% (忙等)
		// 这会让出 CPU 时间片，利用内核缓冲区进行背压丢包
		time.Sleep(10 * time.Millisecond)
	}
}

func (c *udpShaperConn) ReadMsgUDP(b, oob []byte) (n, oobn, flags int, addr *net.UDPAddr, err error) {
	for {
		n, oobn, flags, addr, err = c.UDPConn.ReadMsgUDP(b, oob)
		if err != nil {
			return
		}

		if c.cfg.Read == nil || c.cfg.Read.Allow() {
			return
		}
		// 增加短暂休眠
		time.Sleep(10 * time.Millisecond)
	}
}

func (c *udpShaperConn) WriteMsgUDP(b, oob []byte, addr *net.UDPAddr) (n, oobn int, err error) {
	if c.cfg.Write != nil && !c.cfg.Write.Allow() {
		// 模拟丢包
		return len(b), len(oob), nil
	}
	return c.UDPConn.WriteMsgUDP(b, oob, addr)
}

func (c *udpShaperConn) WriteTo(p []byte, addr net.Addr) (int, error) {
	if c.cfg.Write != nil && !c.cfg.Write.Allow() {
		return len(p), nil
	}
	return c.UDPConn.WriteTo(p, addr)
}

// Unwrap 允许外部获取底层的 *net.UDPConn
func (c *udpShaperConn) Unwrap() net.PacketConn {
	return c.UDPConn
}

// --- 通用 PacketConn 实现 ---

type packetShaperConn struct {
	net.PacketConn
	cfg LimitConfig
}

func (c *packetShaperConn) ReadFrom(p []byte) (int, net.Addr, error) {
	for {
		n, addr, err := c.PacketConn.ReadFrom(p)
		if err != nil {
			return n, addr, err
		}
		if c.cfg.Read == nil || c.cfg.Read.Allow() {
			return n, addr, nil
		}
		// 增加短暂休眠
		time.Sleep(10 * time.Millisecond)
	}
}

func (c *packetShaperConn) WriteTo(p []byte, addr net.Addr) (int, error) {
	if c.cfg.Write != nil && !c.cfg.Write.Allow() {
		return len(p), nil
	}
	return c.PacketConn.WriteTo(p, addr)
}
