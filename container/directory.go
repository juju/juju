// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package container

import (
	"os"
	"path/filepath"

	"launchpad.net/juju-core/utils"
)

var (
	ContainerDir        = "/var/lib/juju/containers"
	RemovedContainerDir = "/var/lib/juju/removed-containers"
)

// NewDirectory creates a new directory for the container name in the
// directory identified by `ContainerDir`.
func NewDirectory(containerName string) (directory string, err error) {
	directory = dirForName(containerName)
	logger.Tracef("create directory: %s", directory)
	if err = os.MkdirAll(directory, 0755); err != nil {
		logger.Errorf("failed to create container directory: %v", err)
		return "", err
	}
	return directory, nil
}

// RemoveDirectory moves the container directory from `ContainerDir`
// to `RemovedContainerDir` and makes sure that the names don't clash.
func RemoveDirectory(containerName string) error {
	// Move the directory.
	logger.Tracef("create old container dir: %s", RemovedContainerDir)
	if err := os.MkdirAll(RemovedContainerDir, 0755); err != nil {
		logger.Errorf("failed to create removed container directory: %v", err)
		return err
	}
	removedDir, err := utils.UniqueDirectory(RemovedContainerDir, containerName)
	if err != nil {
		logger.Errorf("was not able to generate a unique directory: %v", err)
		return err
	}
	if err := os.Rename(dirForName(containerName), removedDir); err != nil {
		logger.Errorf("failed to rename container directory: %v", err)
		return err
	}
	return nil

}

func dirForName(containerName string) string {
	return filepath.Join(ContainerDir, containerName)
}
