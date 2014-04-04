// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package local

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"runtime"

	"launchpad.net/juju-core/container/kvm"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/utils"
	"launchpad.net/juju-core/version"
)

var notLinuxError = errors.New("The local provider is currently only available for Linux")

const aptAddRepositoryJujuStable = `
    sudo apt-add-repository ppa:juju/stable   # required for MongoDB SSL support
    sudo apt-get update`

const installLxcUbuntu = `
Linux Containers (LXC) userspace tools must be
installed to enable the local provider:

    sudo apt-get install lxc`

const installJujuLocalUbuntu = `
juju-local must be installed to enable the local provider:

    sudo apt-get install juju-local`

const installJujuLocalGeneric = `
juju-local must be installed to enable the local provider.
Please consult your operating system distribution's documentation
for instructions on installing this package.`

const installLxcGeneric = `
Linux Containers (LXC) userspace tools must be installed to enable the
local provider. Please consult your operating system distribution's
documentation for instructions on installing the LXC userspace tools.`

const errUnsupportedOS = `Unsupported operating system: %s
The local provider is currently only available for Linux`

// lxclsPath is the path to "lxc-ls", an LXC userspace tool
// we check the presence of to determine whether the
// tools are installed. This is a variable only to support
// unit testing.
var lxclsPath = "lxc-ls"

// isPackageInstalled is a variable to support testing.
var isPackageInstalled = utils.IsPackageInstalled

// defaultRsyslogGnutlsPath is the default path to the
// rsyslog GnuTLS module. This is a variable only to
// support unit testing.
var defaultRsyslogGnutlsPath = "/usr/lib/rsyslog/lmnsd_gtls.so"

// The operating system the process is running in.
// This is a variable only to support unit testing.
var goos = runtime.GOOS

// VerifyPrerequisites verifies the prerequisites of
// the local machine (machine 0) for running the local
// provider.
var VerifyPrerequisites = func(containerType instance.ContainerType) error {
	if goos != "linux" {
		return fmt.Errorf(errUnsupportedOS, goos)
	}
	if err := verifyJujuLocal(); err != nil {
		return err
	}
	switch containerType {
	case instance.LXC:
		return verifyLxc()
	case instance.KVM:
		return kvm.VerifyKVMEnabled()
	}
	return fmt.Errorf("Unknown container type specified in the config.")
}

func verifyLxc() error {
	_, err := exec.LookPath(lxclsPath)
	if err != nil {
		return wrapLxcNotFound(err)
	}
	return nil
}

func verifyJujuLocal() error {
	if isPackageInstalled("juju-local") {
		return nil
	}
	if utils.IsUbuntu() {
		return errors.New(installJujuLocalUbuntu)
	}
	// Not all Linuxes will distribute the module
	// in the same way. Check if it's in the default
	// location too.
	_, err := os.Stat(defaultRsyslogGnutlsPath)
	if err == nil {
		return nil
	}
	return fmt.Errorf("%v\n%s", err, installRsyslogGnutlsGeneric)
}

func wrapLxcNotFound(err error) error {
	if utils.IsUbuntu() {
		return fmt.Errorf("%v\n%s", err, installLxcUbuntu)
	}
	return fmt.Errorf("%v\n%s", err, installLxcGeneric)
}
