// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh

import (
	"os/exec"
)

type Option []string

var AllocateTTY Option = []string{"-t"}

var commonOptions = []string{"-o", "StrictHostKeyChecking no"}

func Command(host string, command string, options ...Option) *exec.Cmd {
	args := append([]string{}, commonOptions...)
	for _, option := range options {
		args = append(args, option...)
	}
	args = append(args, host, "--", command)
	return exec.Command("ssh", args...)
}
