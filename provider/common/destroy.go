// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"strings"

	"github.com/juju/errors"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/provider/registry"
)

// Destroy is a common implementation of the Destroy method defined on
// environs.Environ; we strongly recommend that this implementation be
// used when writing a new provider.
func Destroy(env environs.Environ) error {
	logger.Infof("destroying environment %q", env.Config().Name())
	if err := destroyInstances(env); err != nil {
		return errors.Annotate(err, "destroying instances")
	}
	if err := destroyStorage(env); err != nil {
		return errors.Annotate(err, "destroying storage")
	}
	return nil
}

func destroyInstances(env environs.Environ) error {
	logger.Infof("destroying instances")
	instances, err := env.AllInstances()
	switch err {
	case nil:
		ids := make([]instance.Id, len(instances))
		for i, inst := range instances {
			ids[i] = inst.Id()
		}
		if err := env.StopInstances(ids...); err != nil {
			return err
		}
		fallthrough
	case environs.ErrNoInstances:
		return nil
	default:
		return err
	}
}

func destroyStorage(env environs.Environ) error {
	logger.Infof("destroying storage")
	environConfig := env.Config()
	storageProviderTypes, ok := registry.EnvironStorageProviders(environConfig.Type())
	if !ok {
		return nil
	}
	for _, storageProviderType := range storageProviderTypes {
		storageProvider, err := registry.StorageProvider(storageProviderType)
		if err != nil {
			return errors.Trace(err)
		}
		if !storageProvider.Dynamic() {
			continue
		}
		if storageProvider.Scope() != storage.ScopeEnviron {
			continue
		}
		if err := destroyVolumes(environConfig, storageProviderType, storageProvider); err != nil {
			return errors.Trace(err)
		}
		// TODO(axw) destroy env-level filesystems when we have them.
	}
	return nil
}

func destroyVolumes(
	environConfig *config.Config,
	storageProviderType storage.ProviderType,
	storageProvider storage.Provider,
) error {
	if !storageProvider.Supports(storage.StorageKindBlock) {
		return nil
	}

	storageConfig, err := storage.NewConfig(
		string(storageProviderType),
		storageProviderType,
		map[string]interface{}{},
	)
	if err != nil {
		return errors.Trace(err)
	}

	volumeSource, err := storageProvider.VolumeSource(environConfig, storageConfig)
	if err != nil {
		return errors.Annotate(err, "getting volume source")
	}

	volumeIds, err := volumeSource.ListVolumes()
	if err != nil {
		return errors.Annotate(err, "listing volumes")
	}

	var errStrings []string
	errs, err := volumeSource.DestroyVolumes(volumeIds)
	if err != nil {
		return errors.Annotate(err, "destroying volumes")
	}
	for _, err := range errs {
		if err != nil {
			errStrings = append(errStrings, err.Error())
		}
	}
	if len(errStrings) > 0 {
		return errors.Errorf("destroying volumes: %s", strings.Join(errStrings, ", "))
	}
	return nil
}
