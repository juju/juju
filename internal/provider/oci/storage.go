// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oci

import (
	"github.com/juju/clock"
	"github.com/juju/errors"

	"github.com/juju/juju/internal/storage"
	storageprovider "github.com/juju/juju/internal/storage/provider"
)

type poolType string

const (
	ociStorageProviderType = storage.ProviderType("oci")

	// maxVolumeSizeInGB represents the maximum size in GiB for
	// a single volume. For more information please see:
	// https://docs.oracle.com/cloud/latest/stcomputecs/STCSA/op-storage-volume--post.html#request
	maxVolumeSizeInGB = 16000
	// minVolumeSizeInGB represents the minimum size in GiB for
	// a single volume. For more information please see:
	// https://docs.oracle.com/cloud/latest/stcomputecs/STCSA/op-storage-volume--post.html#request
	minVolumeSizeInGB = 50

	iscsiPool     poolType = "iscsi"
	ociVolumeType string   = "volume-type"
)

var poolTypeMap map[string]poolType = map[string]poolType{
	"iscsi": iscsiPool,
}

type StorageAPI interface{}

type storageProvider struct {
	env *Environ
	api StorageClient
}

var _ storage.Provider = (*storageProvider)(nil)

func (s *storageProvider) VolumeSource(cfg *storage.Config) (storage.VolumeSource, error) {
	envConfig := s.env.Config()
	name := envConfig.Name()
	uuid := envConfig.UUID()
	return &volumeSource{
		env:        s.env,
		envName:    name,
		modelUUID:  uuid,
		storageAPI: s.env.Storage,
		computeAPI: s.env.Compute,
		clock:      clock.WallClock,
	}, nil
}

func (s *storageProvider) FilesystemSource(cfg *storage.Config) (storage.FilesystemSource, error) {
	return nil, errors.NotSupportedf("filesystemsource")
}

func (s *storageProvider) Supports(kind storage.StorageKind) bool {
	return kind == storage.StorageKindBlock
}

func (s *storageProvider) Scope() storage.Scope {
	return storage.ScopeEnviron
}

func (s *storageProvider) Dynamic() bool {
	return true
}

func (s *storageProvider) Releasable() bool {
	// TODO (gsamfira): add support
	return false
}

// DefaultPools returns the default pools available through the oci provider.
// By default a pool by the same name as the provider is offered in addition to
// a iscsi backed storage pool.
//
// Implements [storage.Provider] interface.
func (s *storageProvider) DefaultPools() []*storage.Config {
	defaultPool, _ := storage.NewConfig(
		ociStorageProviderType.String(), ociStorageProviderType, storage.Attrs{},
	)
	iscsiPool, _ := storage.NewConfig("iscsi", ociStorageProviderType, storage.Attrs{
		ociVolumeType: iscsiPool,
	})
	return []*storage.Config{defaultPool, iscsiPool}
}

func (s *storageProvider) ValidateForK8s(map[string]any) error {
	return errors.NotValidf("storage provider type %q", ociStorageProviderType)
}

func (s *storageProvider) ValidateConfig(cfg *storage.Config) error {
	attrs := cfg.Attrs()
	var pool string
	if volType, ok := attrs[ociVolumeType]; ok {
		switch kind := volType.(type) {
		case string:
			pool = volType.(string)

		case poolType:
			pool = string(volType.(poolType))
		default:
			return errors.Errorf("invalid volume-type %T", kind)
		}
		if _, ok := poolTypeMap[pool]; !ok {
			return errors.Errorf("invalid volume-type %q", volType)
		}
		return nil
	}
	return nil
}

// StorageProviderTypes implements storage.ProviderRegistry.
func (e *Environ) StorageProviderTypes() ([]storage.ProviderType, error) {
	return []storage.ProviderType{
		ociStorageProviderType,
		storageprovider.TmpfsProviderType,
		storageprovider.RootfsProviderType,
		storageprovider.LoopProviderType,
	}, nil
}

// StorageProvider implements storage.ProviderRegistry.
func (e *Environ) StorageProvider(t storage.ProviderType) (storage.Provider, error) {
	switch t {
	case ociStorageProviderType:
		return &storageProvider{
			env: e,
			api: e.Storage,
		}, nil
	case storageprovider.TmpfsProviderType:
		return storageprovider.NewTmpfsProvider(storageprovider.LogAndExec), nil
	case storageprovider.RootfsProviderType:
		return storageprovider.NewRootfsProvider(storageprovider.LogAndExec), nil
	case storageprovider.LoopProviderType:
		return storageprovider.NewLoopProvider(storageprovider.LogAndExec), nil
	default:
		return nil, errors.NotFoundf("storage provider %q", t)
	}
}
