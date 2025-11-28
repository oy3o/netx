# netx: Network Extensions for Go

[![Go Report Card](https://goreportcard.com/badge/github.com/oy3o/netx)](https://goreportcard.com/report/github.com/oy3o/netx)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

[中文](./README.zh.md) | [English](./README.md)

`netx` 是一个**零依赖**、**高性能**的 Go 网络层扩展库。

它基于 **装饰器模式 (Decorator Pattern)** 设计，旨在增强标准的 `net.Listener`、`net.Conn` 和 `net.PacketConn`。它为底层网络层提供了 **生命周期管理**、**过载保护**、**流量整形** 和 **可观测性**，同时保持与标准库的完美兼容。

## 核心理念

1.  **机制与策略分离 (Mechanism over Policy)**: `netx` 只提供“如何限速”、“如何统计”的底层机制，而“限速多少”、“统计发给谁”完全由用户通过回调或工厂注入。
2.  **洋葱模型与上下文穿透**: 通过统一的 `Wrapper` 接口，支持 Context 从最外层穿透至最底层，打通 L4 (TCP) 与 L7 (Application) 的生命周期。
3.  **通用兼容性**: 所有组件均返回标准的 `net.Listener` 或 `net.PacketConn`，可直接用于 `http.Server`、`grpc.Server`、`quic-go` 或任何框架。

## 安装

```bash
go get github.com/oy3o/netx
```

## 快速开始

使用 `Chain` 将多个功能像积木一样组合起来：

```go
package main

import (
    "net"
    "net/http"
    "time"
    
    "github.com/oy3o/netx"
)

func main() {
    // 1. 创建高性能 Listener (开启 SO_REUSEPORT)
    ln, _ := netx.ListenTCP("tcp", ":8080", netx.ListenConfig{
        EnableReusePort: true, 
    })

    // 2. 像洋葱一样包裹它
    ln = netx.Chain(ln,
        // [性能] 开启 TCP KeepAlive (默认 3分钟)
        netx.WithKeepAlive(0),
        
        // [安全] 设置初始 I/O 超时 (防止 Slowloris 慢速攻击)
        netx.WithIOTimeout(5*time.Second, 5*time.Second),
        
        // [保护] 限制最大并发连接数为 10,000，防止 OOM
        netx.WithLimit(10000),
        
        // [生命周期] 绑定 Context，连接断开时 Context 自动 Cancel
        netx.WithContext(nil),
    )

    // 3. 启动标准 HTTP Server
    http.Serve(ln, nil)
}
```

## 核心组件

### 1. 生命周期 (Context & Lifecycle)

在 TCP 层引入 `context.Context` 是 `netx` 最强大的特性之一。

*   **功能**: 为每个连接绑定一个 Context。当物理连接断开 (`Close`) 时，该 Context 会自动取消 (`Done`)。
*   **用途**: 业务逻辑监听 Context，实现优雅退出或资源清理。

```go
ln = netx.WithContext(func(c net.Conn) context.Context {
    // 可选：在这里注入 TraceID 或 Logger
    return context.WithValue(context.Background(), "trace_id", uuid.NewString())
})

// 在业务代码中获取：
func HandleRequest(w http.ResponseWriter, r *http.Request) {
    // 即使经过层层包装（甚至经过 TLS），GetContext 也能递归找到最深层的 Context
    connContext := netx.GetContext(r.Context().Value(http.LocalAddrContextKey).(net.Conn))
    
    select {
    case <-connContext.Done():
        println("TCP 连接已断开，停止处理")
    }
}
```

### 2. 流量整形 (Traffic Shaping)

通过策略注入，支持 TCP 和 UDP 的限速逻辑。

*   **TCP (`WithShaper`)**: 拦截 `Read/Write`，使用令牌桶算法限速。
*   **UDP (`WithPPSLimit`)**: 限制 `Read/Write` 的每秒包数 (PPS)。

```go
// TCP 流量整形
factory := func(c net.Conn) (read, write netx.Bucket) {
    ip := getIP(c.RemoteAddr())
    if isVIP(ip) {
        return nil, nil // VIP 不限速
    }
    return globalRateLimiter, globalRateLimiter 
}
ln = netx.WithShaper(factory)

// UDP PPS 限速 (例如用于 QUIC)
pc = netx.WithPPSLimit(netx.LimitConfig{
    Read:  rate.NewLimiter(rate.Limit(1000), 100), // 接收最大 1000 pps
    Write: rate.NewLimiter(rate.Limit(1000), 100), // 发送最大 1000 pps
})(pc)
```

### 3. 安全与防护 (Security & Protection)

*   **WithLimit(n)**: 使用信号量机制限制最大并发连接数。当连接数满时，新连接的 `Accept` 会被阻塞，从而保护 Server 不被瞬间流量打垮。
*   **WithIOTimeout(read, write)**: 为连接设置初始的读写 Deadline。这是防御 **Slowloris** (慢速连接) 攻击的关键，防止恶意客户端建立连接后不发送数据占用资源。

### 4. 基础设施与代理 (Infrastructure)

*   **WithProxyProtocol**: 增加对 **HAProxy PROXY 协议** 的支持。当你的服务部署在 L4 负载均衡器 (如 AWS ELB, Nginx Stream) 之后时，这是获取客户端真实 IP 的标准方式。

### 5. QUIC / HTTP3 优化

`netx` 针对 UDP 协议（特别是 QUIC）提供了专属优化：

*   **WithUDPBuffer(rx, tx)**: 调整内核 Socket 缓冲区大小 (SO_RCVBUF/SO_SNDBUF)。高速 QUIC 传输需要较大的缓冲区 (如 2MB-4MB) 以避免丢包。
*   **EnableReusePort**: 允许在多核机器上启动多个进程/线程监听同一端口，由内核进行负载均衡，显著提升吞吐量。

```go
// 开启 ReusePort
pc, _ := netx.ListenUDP("udp", ":443", netx.ListenConfig{EnableReusePort: true})

// 优化缓冲区 + 限速
pc = netx.ChainUDP(pc,
    netx.WithUDPBuffer(4*1024*1024, 4*1024*1024), // 4MB 缓冲
    netx.WithPPSLimit(limitCfg),
)
```

## 上下文穿透与 `AsTCPConn`

`netx` 包装器允许您深入访问连接栈的底层。

*   **`GetContext(conn)`**: 获取绑定在连接上的生命周期上下文（Context），即使经过了 TLS 层的封装也能获取。
*   **`AsTCPConn(conn)`**: 递归剥离封装层（Metrics -> Limiter -> TLS），返回底层的 `*net.TCPConn` 对象。

```go
// 示例：在封装的 TLS 连接上设置 TCP NoDelay
if tc := netx.AsTCPConn(req.Context().Value(http.LocalAddrContextKey).(net.Conn)); tc != nil {
    tc.SetNoDelay(true)
}
```

## 最佳实践

建议在您的 `server` 包中封装默认的组合逻辑，遵循 **"Default to Secure & Observable"** 原则：

```go
func Listen(addr string) (net.Listener, error) {
    // 1. 使用 ReusePort 提升性能
    l, err := netx.ListenTCP("tcp", addr, netx.ListenConfig{EnableReusePort: true})
    if err != nil { return nil, err }
    
    return netx.Chain(l,
        netx.WithKeepAlive(0),                            // 总是开启保活
        netx.WithIOTimeout(5*time.Second, 5*time.Second), // 总是设置安全超时
        netx.WithContext(nil),                            // 总是绑定生命周期
        netx.WithLimit(50000),                            // 总是设置最大连接数
        netx.WithObserve(myMetrics),                      // 总是开启监控
    ), nil
}
```
