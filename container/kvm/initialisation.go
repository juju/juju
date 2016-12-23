// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build linux
// +build amd64 arm64 ppc64el

package kvm

import (
	"os"

	"github.com/juju/errors"
	"github.com/juju/utils/packaging/manager"
	"github.com/juju/utils/series"

	"github.com/juju/juju/container"
	"github.com/juju/juju/juju/paths"
)

var requiredPackages = []string{
	"genisoimage",
	"libvirt-bin",
	"qemu-utils",
	"qemu-kvm",
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
	if err := ensureDependencies(); err != nil {
		return errors.Trace(err)
	}

	// Check if we've done this already.
	poolInfo, err := poolInfo(run)
	if err != nil {
		return errors.Trace(err)
	}
	if poolInfo == nil {
		if err := createPool(paths.DataDir, run, chownToLibvirt); err != nil {
			return errors.Trace(err)
		}
		return nil
	}
	logger.Debugf(`pool already initialised "%#v"`, poolInfo)

	return nil
}

// getPackageManager is a helper function which returns the
// package manager implementation for the current system.
func getPackageManager() (manager.PackageManager, error) {
	hostSeries, err := series.HostSeries()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return manager.NewPackageManager(hostSeries)
}

func ensureDependencies() error {
	pacman, err := getPackageManager()
	if err != nil {
		return err
	}

	for _, pack := range requiredPackages {
		if err := pacman.Install(pack); err != nil {
			return err
		}
	}

	return nil
}

// createPool creates the libvirt storage pool directory. runCmd and chownFunc
// are here for testing. runCmd so we can check the right shell out calls are
// made, and chownFunc because we cannot chown unless we are root.
func createPool(pathfinder func(string) (string, error), runCmd runFunc, chownFunc func(string) error) error {
	poolDir, err := guestPath(pathfinder)
	if err != nil {
		return errors.Trace(err)
	}

	if err = definePool(poolDir, runCmd, chownFunc); err != nil {
		return errors.Trace(err)
	}
	if err = buildPool(runCmd); err != nil {
		return errors.Trace(err)
	}

	if err = startPool(runCmd); err != nil {
		return errors.Trace(err)
	}
	if err = autostartPool(runCmd); err != nil {
		return errors.Trace(err)
	}

	// We have to set ownership of the guest pool directory after running virsh
	// commands above, because it appears that the libvirt-bin version that
	// ships with trusty sets the ownership of the pool directory to the user
	// running the commands -- root in our case. Which causes container
	// initialization to fail as we couldn't write volumes to the pool. We
	// write them as libvirt-qemu:kvm so that libvirt -- which runs as that
	// user -- can read them to boot the domains.
	if err = chownFunc(poolDir); err != nil {
		return errors.Trace(err)
	}

	return nil
}

// definePool creates the required directories and changes ownershipt of the
// guest directory so that libvirt-qemu can read, write, and execute its
// guest volumes.
func definePool(dir string, runCmd runFunc, chownFunc func(string) error) error {
	// Permissions gleaned from https://goo.gl/SZIw14
	// The command itself would change the permissions to match anyhow.
	err := os.MkdirAll(dir, 0755)
	if err != nil {
		return errors.Trace(err)
	}

	output, err := runCmd(
		"virsh",
		"pool-define-as",
		poolName,
		"dir",
		"-", "-", "-", "-",
		dir)
	if err != nil {
		return errors.Trace(err)
	}
	logger.Debugf("pool-define-as ouput %s", output)

	return nil
}

// chownToLibvirt changes ownership of the provided directory to
// libvirt-qemu:kvm.
func chownToLibvirt(dir string) error {
	uid, gid, err := getUserUIDGID(libvirtUser)
	if err != nil {
		logger.Errorf("failed to get livirt-qemu uid:gid %s", err)
		return errors.Trace(err)
	}

	err = os.Chown(dir, uid, gid)
	if err != nil {
		logger.Errorf("failed to change ownership of %q to uid:gid %d:%d %s", dir, uid, gid, err)
		return errors.Trace(err)
	}
	logger.Tracef("%q is now owned by %q %d:%d", dir, libvirtUser, uid, gid)
	return nil
}

// buildPool sets up libvirt internals for the guest pool.
func buildPool(runCmd runFunc) error {
	// This can run without errror if the pool isn't active.
	output, err := runCmd("virsh", "pool-build", poolName)
	if err != nil {
		return errors.Trace(err)
	}
	logger.Debugf("pool-build ouput %s", output)
	return nil
}

// startPool makes the pool available for use in libvirt.
func startPool(runCmd runFunc) error {
	output, err := runCmd("virsh", "pool-start", poolName)
	if err != nil {
		return errors.Trace(err)
	}
	logger.Debugf("pool-start ouput %s", output)

	return nil
}

// autostartPool sets up the pool to run automatically when libvirt starts.
func autostartPool(runCmd runFunc) error {
	output, err := runCmd("virsh", "pool-autostart", poolName)
	if err != nil {
		return errors.Trace(err)
	}
	logger.Debugf("pool-autostart ouput %s", output)

	return nil
}
