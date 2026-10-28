//go:build unix && !aix && !linux && !solaris && !zos && !darwin && !dragonfly && !freebsd && !netbsd && !openbsd

package render

import "io"

func platformEnableVirtualTerminal(io.Writer) bool { return true }

func isTerminal(io.Writer) bool { return false }

func terminalColumns(io.Writer) int { return 0 }
