// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package ssh contains utilities for dealing with SSH connections,
// key management, and so on. All SSH-based command executions in
// Juju should use the Command/ScpCommand functions in this package.
//
// TODO(axw) use PuTTY/plink if it's available on Windows.
// TODO(axw) fallback to go.crypto/ssh if no native client is available.
package ssh

import (
	"os"
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

// sshpassWrap wraps the command/args with sshpass if it is found in $PATH
// and the SSHPASS environment variable is set. Otherwise, the original
// command/args are returned.
func sshpassWrap(cmd string, args []string) (string, []string) {
	if os.Getenv("SSHPASS") != "" {
		if path, err := exec.LookPath("sshpass"); err == nil {
			return path, append([]string{"-e", cmd}, args...)
		}
	}
	return cmd, args
}

// Command initialises an os/exec.Cmd to execute the native ssh program.
//
// If the SSHPASS environment variable is set, and the sshpass program
// is available in $PATH, then the ssh command will be run with "sshpass -e".
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
	bin, args := sshpassWrap("ssh", args)
	return exec.Command(bin, args...)
}

// ScpCommand initialises an os/exec.Cmd to execute the native scp program.
//
// If the SSHPASS environment variable is set, and the sshpass program
// is available in $PATH, then the scp command will be run with "sshpass -e".
func ScpCommand(source, destination string, options ...Option) *exec.Cmd {
	args := append([]string{}, commonOptions...)
	for _, option := range options {
		args = append(args, option...)
	}
	args = append(args, source, destination)
	bin, args := sshpassWrap("scp", args)
	return exec.Command(bin, args...)
}
