// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kvm

import (
	"fmt"
	"strings"

	"launchpad.net/juju-core/container"
	"launchpad.net/juju-core/utils"
)

var requiredPackages = []string{
	"uvtool-libvirt",
	"uvtool",
}

type containerInitialiser struct{}

// containerInitialiser implements container.Initialiser.
var _ container.Initialiser = (*containerInitialiser)(nil)

// NewContainerInitialiser returns an instance used to perform the steps
// required to allow a host machine to run a KVM container.
func NewContainerInitialiser() container.Initialiser {
	return &containerInitialiser{}
}

// Initialise is specified on the container.Initialiser interface.
func (ci *containerInitialiser) Initialise() error {
	return ensureDependencies()
}

func ensureDependencies() error {
	return utils.AptGetInstall(requiredPackages...)
}

const kvmNeedsUbuntu = `Sorry, KVM support with the local provider is only supported
on the Ubuntu OS.`

const kvmNotSupported = `KVM is not currently supported with the current settings.
You could try running 'kvm-ok' yourself as root to get the full rationale as to
why it isn't supported, or potentially some BIOS settings to change to enable
KVM support.`

const neetToInstallKVMOk = `kvm-ok is not installed. Please install the cpu-checker package.
    sudo apt-get install cpu-checker`

const missingKVMDeps = `Some required packages are missing for KVM to work:

    sudo apt-get install %s
`

// VerifyKVMEnabled makes sure that the host OS is Ubuntu, and that the required
// packages are installed, and that the host CPU is able to support KVM.
func VerifyKVMEnabled() error {
	if !utils.IsUbuntu() {
		return fmt.Errorf(kvmNeedsUbuntu)
	}
	supported, err := IsKVMSupported()
	if err != nil {
		// Missing the kvm-ok package.
		return fmt.Errorf(neetToInstallKVMOk)
	}
	if !supported {
		return fmt.Errorf(kvmNotSupported)
	}
	// Check for other packages needed.
	toInstall := []string{}
	for _, pkg := range requiredPackages {
		if !utils.IsPackageInstalled(pkg) {
			toInstall = append(toInstall, pkg)
		}
	}
	if len(toInstall) > 0 {
		return fmt.Errorf(missingKVMDeps, strings.Join(toInstall, " "))
	}
	return nil
}
