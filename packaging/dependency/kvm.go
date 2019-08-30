// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dependency

import (
	"github.com/juju/errors"
	"github.com/juju/juju/packaging"
	"github.com/juju/utils/arch"
)

// KVM returns a dependency instance for installing KVM support.
func KVM(arch string) packaging.Dependency {
	return &kvmDependency{arch: arch}
}

type kvmDependency struct {
	arch string
}

// PackageList implements packaging.Dependency.
func (dep kvmDependency) PackageList(series string) ([]packaging.Package, error) {
	var pkgList []string

	switch series {
	case "centos7", "opensuseleap":
		return nil, errors.NotSupportedf("installing kvm on series %q", series)
	default:
		if dep.arch == arch.ARM64 {
			// ARM64 doesn't support legacy BIOS so it requires Extensible Firmware
			// Interface.
			pkgList = append(pkgList, "qemu-efi")
		}

		pkgList = append(pkgList,
			// `qemu-kvm` must be installed before `libvirt-bin` on trusty. It appears
			// that upstart doesn't reload libvirtd if installed after, and we see
			// errors related to `qemu-kvm` not being installed.
			"qemu-kvm",
			"qemu-utils",
			"genisoimage",
			"libvirt-bin",
		)

		return packaging.MakePackageList(packaging.AptPackageManager, "", pkgList...), nil
	}
}
