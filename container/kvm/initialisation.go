// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kvm

import (
	"path/filepath"

	"github.com/juju/errors"
	"github.com/juju/utils/packaging/manager"
	"github.com/juju/utils/series"

	"github.com/juju/juju/container"
	"github.com/juju/juju/juju/paths"
)

var requiredPackages = []string{
	"libvirt-bin",
	"qemu-utils",
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
	if err := createPool(paths.DataDir, run); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// getPackageManager is a helper function which returns the
// package manager implementation for the current system.
func getPackageManager() (manager.PackageManager, error) {
	return manager.NewPackageManager(series.HostSeries())
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

// createPool creates the libvirt storage pool directory.
func createPool(pathfinder func(string) (string, error), runFunc func(string, ...string) (string, error)) error {
	baseDir, err := pathfinder(series.HostSeries())
	if err != nil {
		return errors.Trace(err)
	}
	poolDir := filepath.Join(baseDir, guestDir)
	output, err := runFunc("virsh", "pool-define-as", poolName, "dir", "- - - -", poolDir)
	if err != nil {
		return errors.Annotate(err, output)
	}
	output, err = runFunc("virsh", "pool-build", poolName)
	if err != nil {
		return errors.Annotate(err, output)
	}
	output, err = runFunc("virsh", "pool-start", poolName)
	if err != nil {
		return errors.Annotate(err, output)
	}
	output, err = runFunc("virsh", "pool-autostart", poolName)
	if err != nil {
		return errors.Annotate(err, output)
	}
	return nil
}
