// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner

import (
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	environscontext "github.com/juju/juju/environs/context"
	"github.com/juju/juju/storage"
)

// Logger represents the methods used by the worker to log details.
type Logger interface {
	Tracef(string, ...interface{})
	Debugf(string, ...interface{})
	Warningf(string, ...interface{})
	Errorf(string, ...interface{})
}

// Config holds configuration and dependencies for a storageprovisioner worker.
type Config struct {
	Model            names.ModelTag
	Scope            names.Tag
	StorageDir       string
	Applications     ApplicationWatcher
	Volumes          VolumeAccessor
	Filesystems      FilesystemAccessor
	Life             LifecycleManager
	Registry         storage.ProviderRegistry
	Machines         MachineAccessor
	Status           StatusSetter
	Clock            clock.Clock
	Logger           Logger
	CloudCallContext environscontext.ProviderCallContext
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
		if config.Applications == nil {
			return errors.NotValidf("nil Applications")
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
	if config.CloudCallContext == nil {
		return errors.NotValidf("nil CloudCallContext")
	}
	return nil
}
