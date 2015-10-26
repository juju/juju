// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package local

import (
	"errors"
	"fmt"
	"os/exec"
	"runtime"

	"github.com/juju/utils"

	"github.com/juju/juju/container/kvm"
	"github.com/juju/juju/instance"
)

var notLinuxError = errors.New("The local provider is currently only available for Linux")

const aptAddRepositoryJujuStable = `
    sudo apt-add-repository ppa:juju/stable   # required for MongoDB SSL support
    sudo apt-get update`

const installLxcUbuntu = `
Linux Containers (LXC) userspace tools must be
installed to enable the local provider:

    sudo apt-get install lxc`

const installCloudImageUtils = `
cloud-image-utils must be installed to enable the local provider:

    sudo apt-get install cloud-image-utils`

const installJujuLocalUbuntu = `
juju-local must be installed to enable the local provider:

    sudo apt-get install juju-local`

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
	return verifyCloudImageUtils()
}

func verifyCloudImageUtils() error {
	if isPackageInstalled("cloud-image-utils") {
		return nil
	}
	return errors.New(installCloudImageUtils)
}

func verifyJujuLocal() error {
	if isPackageInstalled("juju-local") {
		return nil
	}
	return errors.New(installJujuLocalUbuntu)
}

func wrapLxcNotFound(err error) error {
	if utils.IsUbuntu() {
		return fmt.Errorf("%v\n%s", err, installLxcUbuntu)
	}
	return fmt.Errorf("%v\n%s", err, installLxcGeneric)
}
