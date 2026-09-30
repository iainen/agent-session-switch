//go:build !unix

package main

import (
	"errors"
	"os"
	"os/exec"
)

// execAgent runs the agent as a child process because Windows cannot
// replace the current process, and exits with the agent's exit code.
func execAgent(path string, argv []string) error {
	cmd := exec.Command(path, argv[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	err := cmd.Run()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		os.Exit(exitErr.ExitCode())
	}
	if err != nil {
		return err
	}
	os.Exit(0)
	return nil
}
