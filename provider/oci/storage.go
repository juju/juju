// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oci

import (
	"github.com/juju/errors"
	"github.com/juju/utils/clock"

	providerCommon "github.com/juju/juju/provider/oci/common"
	"github.com/juju/juju/storage"
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
	api providerCommon.ApiClient
}

var _ storage.Provider = (*storageProvider)(nil)

func (s *storageProvider) VolumeSource(cfg *storage.Config) (storage.VolumeSource, error) {
	envConfig := s.env.Config()
	name := envConfig.Name()
	uuid := envConfig.UUID()
	return newOciVolumeSource(s.env, name, uuid, s.env.cli, clock.WallClock)
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

func (s *storageProvider) DefaultPools() []*storage.Config {
	iscsiPool, _ := storage.NewConfig("iscsi", ociStorageProviderType, map[string]interface{}{
		ociVolumeType: iscsiPool,
	})
	return []*storage.Config{iscsiPool}
}

func (s *storageProvider) ValidateConfig(cfg *storage.Config) error {
	return nil
}
