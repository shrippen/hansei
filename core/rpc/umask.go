package rpc

import "syscall"

// umask sets the process umask so the socket is created without group/other access.
func umask(mask int) int { return syscall.Umask(mask) }
