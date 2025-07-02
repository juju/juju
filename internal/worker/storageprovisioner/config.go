// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner

import (
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/storage"
)

// Config holds configuration and dependencies for a storageprovisioner worker.
type Config struct {
	Model       names.ModelTag
	Scope       names.Tag
	StorageDir  string
	Volumes     VolumeAccessor
	Filesystems FilesystemAccessor
	Life        LifecycleManager
	Registry    storage.ProviderRegistry
	Machines    MachineAccessor
	Status      StatusSetter
	Clock       clock.Clock
	Logger      logger.Logger
}

// Validate returns an error if the config cannot be relied upon to start a worker.
func (config Config) Validate() error {
	switch config.Scope.(type) {
	case nil:
		return errors.NotValidf("nil Scope")
	case names.ModelTag:
		if config.StorageDir != "" {
			return errors.NotValidf("environ Scope with non-empty StorageDir")
		}
	case names.MachineTag:
		if config.StorageDir == "" {
			return errors.NotValidf("machine Scope with empty StorageDir")
		}
		if config.Machines == nil {
			return errors.NotValidf("nil Machines")
		}
	case names.ApplicationTag:
		if config.StorageDir != "" {
			return errors.NotValidf("application Scope with StorageDir")
		}
	default:
		return errors.NotValidf("%T Scope", config.Scope)
	}
	if config.Volumes == nil {
		return errors.NotValidf("nil Volumes")
	}
	if config.Filesystems == nil {
		return errors.NotValidf("nil Filesystems")
	}
	if config.Life == nil {
		return errors.NotValidf("nil Life")
	}
	if config.Registry == nil {
		return errors.NotValidf("nil Registry")
	}
	if config.Status == nil {
		return errors.NotValidf("nil Status")
	}
	if config.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	return nil
}
