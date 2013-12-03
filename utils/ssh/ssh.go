// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh

import (
	"os/exec"
)

type Option []string

var (
	commonOptions Option = []string{"-o", "StrictHostKeyChecking no"}

	// AllocateTTY forces pseudo-TTY allocation, which is required,
	// for example, for sudo password prompts on the target host.
	AllocateTTY Option = []string{"-t"}

	// NoPasswordAuthentication disallows password-based authentication.
	NoPasswordAuthentication Option = []string{"-o", "PasswordAuthentication no"}
)

// Command initialises an os/exec.Cmd to execute the native ssh program.
func Command(host string, command []string, options ...Option) *exec.Cmd {
	args := append([]string{}, commonOptions...)
	for _, option := range options {
		args = append(args, option...)
	}
	args = append(args, host)
	if len(command) > 0 {
		args = append(args, "--")
		args = append(args, command...)
	}
	return exec.Command("ssh", args...)
}

// ScpCommand initialises an os/exec.Cmd to execute the native scp program.
func ScpCommand(source, destination string, options ...Option) *exec.Cmd {
	args := append([]string{}, commonOptions...)
	for _, option := range options {
		args = append(args, option...)
	}
	args = append(args, source, destination)
	return exec.Command("scp", args...)
}
