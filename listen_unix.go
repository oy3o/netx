//go:build linux || darwin || dragonfly || freebsd || netbsd || openbsd || aix || solaris

package netx

import (
	"syscall"

	"golang.org/x/sys/unix"
)

func controlReusePort(network, address string, c syscall.RawConn) error {
	var opErr error
	err := c.Control(func(fd uintptr) {
		// 使用 golang.org/x/sys/unix 设置 SO_REUSEPORT
		// 这里的 int(fd) 转换在不同平台上是安全的
		opErr = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEPORT, 1)
	})
	if err != nil {
		return err
	}
	return opErr
}
