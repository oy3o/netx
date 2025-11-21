//go:build windows

package netx

import (
	"syscall"
)

func controlReusePort(network, address string, c syscall.RawConn) error {
	// Windows 不支持标准的 SO_REUSEPORT 负载均衡机制。
	// 在开发环境下直接忽略此选项，回退到普通监听模式。
	return nil
}
