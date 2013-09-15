// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual

import (
	"os/exec"
)

type sshOption []string

var allocateTTY sshOption = []string{"-t"}

// TODO(axw) 2013-09-12 bug #1224230
// Move this to a common package for use in cmd/juju, and others.
var commonSSHOptions = []string{"-o", "StrictHostKeyChecking no"}

func sshCommand(host string, command string, options ...sshOption) *exec.Cmd {
	args := append([]string{}, commonSSHOptions...)
	for _, option := range options {
		args = append(args, option...)
	}
	args = append(args, host, "--", command)
	return exec.Command("ssh", args...)
}
