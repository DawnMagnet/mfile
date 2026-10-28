//go:build !unix && !windows

package render

import "io"

func platformEnableVirtualTerminal(io.Writer) (bool, func() error) { return true, nil }

func isTerminal(io.Writer) bool { return false }

func terminalColumns(io.Writer) int { return 0 }
