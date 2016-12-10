// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build linux
// +build amd64 arm64 ppc64el

package kvm

import "github.com/juju/utils"

const libvirtUser = "libvirt-qemu"

// runFunc provides the signature for running an external command and returning
// the combined output.
type runFunc func(string, ...string) (string, error)

// run the command and return the combined output.
func run(command string, args ...string) (output string, err error) {
	logger.Debugf("%s %v", command, args)
	output, err = utils.RunCommand(command, args...)
	logger.Debugf("output: %v", output)
	return output, err
}
