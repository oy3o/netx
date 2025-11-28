# netx: Network Extensions for Go

[![Go Report Card](https://goreportcard.com/badge/github.com/oy3o/netx)](https://goreportcard.com/report/github.com/oy3o/netx)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

[中文](./README.zh.md) | [English](./README.md)

`netx` is a **zero-dependency**, **high-performance** network layer extension library for Go.

Designed based on the **Decorator Pattern**, it aims to enhance the standard `net.Listener`, `net.Conn`, and `net.PacketConn`. It provides **Lifecycle Management**, **Overload Protection**, **Traffic Shaping**, and **Observability** for the underlying network layer while maintaining perfect compatibility with the standard library.

## Core Philosophies

1.  **Mechanism over Policy**: `netx` only provides the underlying mechanisms for "how to limit rates" or "how to collect stats." Decisions on "what the rate limit is" or "where to send stats" are entirely controlled by the user via callbacks or factory injection.
2.  **Onion Model & Context Penetration**: Through a unified `Wrapper` interface, it supports Context penetration from the outermost layer to the innermost layer, bridging the lifecycle between L4 (TCP) and L7 (Application).
3.  **Universal Compatibility**: All components return standard `net.Listener` or `net.PacketConn`, making them directly usable with `http.Server`, `grpc.Server`, `quic-go`, or any framework.

## Installation

```bash
go get github.com/oy3o/netx
```

## Quick Start

Use `Chain` to combine multiple features like building blocks:

```go
package main

import (
    "net"
    "net/http"
    "time"
    
    "github.com/oy3o/netx"
)

func main() {
    // 1. Create a Listener with SO_REUSEPORT enabled (High Performance)
    ln, _ := netx.ListenTCP("tcp", ":8080", netx.ListenConfig{
        EnableReusePort: true, 
    })

    // 2. Wrap it like an onion
    ln = netx.Chain(ln,
        // [Performance] Enable TCP KeepAlive (Default: 3 minutes)
        netx.WithKeepAlive(0),
        
        // [Safety] Set Initial I/O Deadlines (Prevent Slowloris attacks)
        netx.WithIOTimeout(5*time.Second, 5*time.Second),
        
        // [Protection] Limit max concurrent connections to 10,000
        netx.WithLimit(10000),
        
        // [Lifecycle] Bind Context; auto-cancel Context when connection closes
        netx.WithContext(nil),
    )

    // 3. Start standard HTTP Server
    http.Serve(ln, nil)
}
```

## Core Components

### 1. Lifecycle (Context & Lifecycle)

Introducing `context.Context` at the TCP layer is one of the most powerful features of `netx`.

*   **Function**: Binds a Context to every connection. When the physical connection is closed (`Close`), the Context is automatically cancelled (`Done`).
*   **Usage**: Business logic can listen to the Context to implement graceful exits or resource cleanup.

```go
ln = netx.WithContext(func(c net.Conn) context.Context {
    // Optional: Inject TraceID or Logger here
    return context.WithValue(context.Background(), "trace_id", uuid.NewString())
})

// Access in business code:
func HandleRequest(w http.ResponseWriter, r *http.Request) {
    // GetContext can recursively find the deepest Context even through layers of wrappers
    // It even works through TLS connections!
    connContext := netx.GetContext(r.Context().Value(http.LocalAddrContextKey).(net.Conn))
    
    select {
    case <-connContext.Done():
        println("TCP connection closed, stop processing")
    }
}
```

### 2. Traffic Shaping (TCP & UDP)

Implement complex rate-limiting logic through policy injection.

*   **TCP (`WithShaper`)**: Intercepts `Read/Write` with Token Bucket algorithm.
*   **UDP (`WithPPSLimit`)**: Limits Packets Per Second for `Read/Write`.

```go
// TCP Traffic Shaping
factory := func(c net.Conn) (read, write netx.Bucket) {
    ip := getIP(c.RemoteAddr())
    if isVIP(ip) {
        return nil, nil // No limit for VIPs
    }
    return globalRateLimiter, globalRateLimiter 
}
ln = netx.WithShaper(factory)

// UDP PPS Limiting (e.g. for QUIC)
pc = netx.WithPPSLimit(netx.LimitConfig{
    Read:  rate.NewLimiter(rate.Limit(1000), 100), // Max 1000 pps Rx
    Write: rate.NewLimiter(rate.Limit(1000), 100), // Max 1000 pps Tx
})(pc)
```

### 3. Security & Protection

*   **WithLimit(n)**: Uses a semaphore to limit max concurrent connections. Blocks `Accept` when full.
*   **WithIOTimeout(read, write)**: Sets a deadline on `Accept`. Essential for defending against **Slowloris** attacks where clients hold connections open without sending data.

### 4. Infrastructure & Proxy

*   **WithProxyProtocol**: Adds support for **HAProxy PROXY protocol**. Essential when your service is deployed behind an L4 Load Balancer (AWS ELB, Nginx Stream) and you need the real client IP.

### 5. QUIC / HTTP3 Optimization

`netx` provides specific optimizations for UDP protocols like QUIC:

*   **WithUDPBuffer(rx, tx)**: Adjusts kernel socket buffer sizes (SO_RCVBUF/SO_SNDBUF). High-speed QUIC transfers require larger buffers (e.g., 2MB-4MB) to prevent packet loss.
*   **EnableReusePort**: Allows multiple processes to bind to the same port, enabling kernel-level load balancing.

```go
pc, _ := netx.ListenUDP("udp", ":443", netx.ListenConfig{EnableReusePort: true})

pc = netx.ChainUDP(pc,
    netx.WithUDPBuffer(4*1024*1024, 4*1024*1024), // 4MB Buffer
    netx.WithPPSLimit(limitCfg),
)
```

### Context Penetration & `AsTCPConn`

`netx` wrappers allow you to reach deep into the connection stack.

*   **`GetContext(conn)`**: Retrieves the lifecycle context bound to the connection, even through TLS layers.
*   **`AsTCPConn(conn)`**: Recursively unwraps layers (Metrics -> Limiter -> TLS) to return the underlying `*net.TCPConn`.

```go
// Example: Setting TCP NoDelay on a wrapped TLS connection
if tc := netx.AsTCPConn(req.Context().Value(http.LocalAddrContextKey).(net.Conn)); tc != nil {
    tc.SetNoDelay(true)
}
```

## Best Practices

Encapsulate default logic within your `server` package, following the **"Default to Secure & Observable"** principle:

```go
func Listen(addr string) (net.Listener, error) {
    // 1. Use ReusePort for performance
    l, err := netx.ListenTCP("tcp", addr, netx.ListenConfig{EnableReusePort: true})
    if err != nil { return nil, err }
    
    return netx.Chain(l,
        netx.WithKeepAlive(0),                            // KeepAlive
        netx.WithIOTimeout(5*time.Second, 5*time.Second), // Safety
        netx.WithContext(nil),                            // Lifecycle
        netx.WithLimit(50000),                            // Max Conns
        netx.WithObserve(myMetrics),                      // Metrics
    ), nil
}
```
