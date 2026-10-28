//go:build amd64 && windows

package cpuguard

import (
	"errors"
	"os"
	"os/exec"
)

// reexecSelf starts a child with the guarded environment, passing the
// standard streams through, and exits with the child's status.
// It returns only when the child could not be started.
func reexecSelf(env []string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, os.Args[1:]...)
	cmd.Env = env
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	if err == nil {
		os.Exit(0)
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		os.Exit(exitErr.ExitCode())
	}
	return err
}
