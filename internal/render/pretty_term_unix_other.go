//go:build unix && !aix && !linux && !solaris && !zos && !darwin && !dragonfly && !freebsd && !netbsd && !openbsd

package render

import "io"

func platformEnableVirtualTerminal(io.Writer) (bool, func() error) { return true, nil }

func isTerminal(io.Writer) bool { return false }

func terminalColumns(io.Writer) int { return 0 }
