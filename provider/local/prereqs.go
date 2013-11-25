// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package local

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"launchpad.net/juju-core/container/kvm"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/version"
)

var notLinuxError = errors.New("The local provider is currently only available for Linux")

const installMongodUbuntu = "MongoDB server must be installed to enable the local provider:"
const aptAddRepositoryJujuStable = `
    sudo apt-add-repository ppa:juju/stable   # required for MongoDB SSL support
    sudo apt-get update`
const aptGetInstallMongodbServer = `
    sudo apt-get install mongodb-server`

const installMongodGeneric = `
MongoDB server must be installed to enable the local provider.
Please consult your operating system distribution's documentation
for instructions on installing the MongoDB server. Juju requires
a MongoDB server built with SSL support.
`

const installLxcUbuntu = `
Linux Containers (LXC) userspace tools must be
installed to enable the local provider:
  
    sudo apt-get install lxc`

const installLxcGeneric = `
Linux Containers (LXC) userspace tools must be installed to enable the
local provider. Please consult your operating system distribution's
documentation for instructions on installing the LXC userspace tools.`

const errUnsupportedOS = `Unsupported operating system: %s
The local provider is currently only available for Linux`

const kvmNeedsUbuntu = `Sorry, but KVM support with the local provider is only supported
on the Ubuntu OS.`

const kvmNotSupported = `KVM is not currently supported with the current settings.
You could try running 'kvm-ok' yourself as root to get the full rationale as to
why it isn't supported, or potentially some BIOS settings to change to enable
KVM support.`

const neetToInstallKVMOk = `kvm-ok is not installed. Please install the cpu-checker package.
    sudo apt-get install cpu-checker`

// mongodPath is the path to "mongod", the MongoDB server.
// This is a variable only to support unit testing.
var mongodPath = "/usr/bin/mongod"

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
func VerifyPrerequisites(containerType instance.ContainerType) error {
	if goos != "linux" {
		return fmt.Errorf(errUnsupportedOS, goos)
	}
	if err := verifyMongod(); err != nil {
		return err
	}
	switch containerType {
	case instance.LXC:
		return verifyLxc()
	case instance.KVM:
		return verifyKvm()
	}
	return fmt.Errorf("Unknown container type specified in the config.")
}

func verifyMongod() error {
	if _, err := os.Stat(mongodPath); err != nil {
		if os.IsNotExist(err) {
			return wrapMongodNotExist(err)
		} else {
			return err
		}
	}
	// TODO(axw) verify version/SSL capabilities
	return nil
}

func verifyLxc() error {
	_, err := exec.LookPath(lxclsPath)
	if err != nil {
		return wrapLxcNotFound(err)
	}
	return nil
}

func isUbuntu() bool {
	out, err := exec.Command("lsb_release", "-i", "-s").CombinedOutput()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "Ubuntu"
}

func wrapMongodNotExist(err error) error {
	if isUbuntu() {
		series := version.Current.Series
		args := []interface{}{err, installMongodUbuntu}
		format := "%v\n%s\n%s"
		if series == "precise" || series == "quantal" {
			format += "%s"
			args = append(args, aptAddRepositoryJujuStable)
		}
		args = append(args, aptGetInstallMongodbServer)
		return fmt.Errorf(format, args...)
	}
	return fmt.Errorf("%v\n%s", err, installMongodGeneric)
}

func wrapLxcNotFound(err error) error {
	if isUbuntu() {
		return fmt.Errorf("%v\n%s", err, installLxcUbuntu)
	}
	return fmt.Errorf("%v\n%s", err, installLxcGeneric)
}

func verifyKvm() error {
	if !isUbuntu() {
		return fmt.Errorf(kvmNeedsUbuntu)
	}
	supported, err := kvm.IsKVMSupported()
	if err != nil {
		// Missing the kvm-ok package.
		return fmt.Errorf(neetToInstallKVMOk)
	}
	if !supported {
		return fmt.Errorf(kvmNotSupported)
	}
	// Check for other packages needed.
	// TODO: also check for:
	//   virsh from libvirt-bin
	//   uvt-kvm from uvtool-libvirt
	//   something from the kvm package
	return nil
}
