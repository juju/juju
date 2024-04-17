// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dependency

import (
	"github.com/juju/errors"

	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/base"
	"github.com/juju/juju/packaging"
)

// KVM returns a dependency instance for installing KVM support.
func KVM(arch string) packaging.Dependency {
	return &kvmDependency{arch: arch}
}

type kvmDependency struct {
	arch string
}

// PackageList implements packaging.Dependency.
func (dep kvmDependency) PackageList(b base.Base) ([]packaging.Package, error) {
	if b.OS != ubuntu {
		return nil, errors.NotSupportedf("installing kvm on base %q", b)
	}

	var pkgList []string
	if dep.arch == arch.ARM64 {
		// ARM64 doesn't support legacy BIOS so it requires Extensible Firmware
		// Interface.
		pkgList = append(pkgList, "qemu-efi")
	}

	pkgList = append(pkgList,
		"qemu-kvm",
		"qemu-utils",
		"genisoimage",
	)

	// On focal+ virsh is provided by libvirt-clients; also we need
	// to install the daemon package separately.
	pkgList = append(pkgList,
		"libvirt-daemon-system",
		"libvirt-clients",
	)

	return packaging.MakePackageList(packaging.AptPackageManager, "", pkgList...), nil
}
