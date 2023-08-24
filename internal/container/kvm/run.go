// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kvm

import (
	"os/exec"
)

// This is the user on ubuntu. I don't know what the user would be on other
// linux distros. At the time of this writing we are only supporting ubuntu on
// ubuntu for kvm containers in Juju.
const libvirtUser = "libvirt-qemu"

// runFunc provides the signature for running an external
// command and returning the combined output.
// The first parameter, if non-empty will use the input
// path as the working directory for the command.
// NOTE: if changing runFunc, remember to edit BOTH copies of
// runAsLibvirt() in run_other.go and run_linux.go.  One doesn't
// compile on linux, thus easily missed.
type runFunc func(string, string, ...string) (string, error)

// run the command and return the combined output.
func run(dir, command string, args ...string) (string, error) {
	logger.Debugf("(%s) %s %v", dir, command, args)

	cmd := exec.Command(command, args...)
	if dir != "" {
		cmd.Dir = dir
	}

	out, err := cmd.CombinedOutput()
	output := string(out)

	logger.Debugf("output: %v", output)
	return output, err
}
