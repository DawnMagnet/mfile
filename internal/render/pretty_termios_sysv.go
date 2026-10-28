//go:build aix || linux || solaris || zos

package render

import "golang.org/x/sys/unix"

const ioctlReadTermios = unix.TCGETS
