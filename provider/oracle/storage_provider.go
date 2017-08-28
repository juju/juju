// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oracle

import (
	"github.com/juju/errors"
	ociCommon "github.com/juju/go-oracle-cloud/common"
	"github.com/juju/utils/clock"

	"github.com/juju/juju/storage"
)

type poolType string

const (
	// for details please see:
	// https://docs.oracle.com/cloud/latest/stcomputecs/STCSA/op-storage-volume--post.html#request-definitions-StorageVolume-post-request-properties-properties
	latencyPool      poolType = "latency"
	defaultPool      poolType = "default"
	oracleVolumeType string   = "volume-type"
)

var poolTypeMap map[poolType]ociCommon.StoragePool = map[poolType]ociCommon.StoragePool{
	latencyPool: ociCommon.LatencyPool,
	defaultPool: ociCommon.DefaultPool,
}

// VolumeSource is defined on the storage.Provider interface.
func (s *storageProvider) VolumeSource(cfg *storage.Config) (storage.VolumeSource, error) {
	environConfig := s.env.Config()
	name := environConfig.Name()
	uuid := environConfig.UUID()
	return newOracleVolumeSource(s.env, name, uuid, s.env.client, clock.WallClock)
}

// FilesystemSource is defined on the storage.Provider interface.
func (s storageProvider) FilesystemSource(cfg *storage.Config) (storage.FilesystemSource, error) {
	return nil, errors.NotSupportedf("filesystemsource")
}

// Supports  is defined on the storage.Provider interface.
func (s storageProvider) Supports(kind storage.StorageKind) bool {
	return kind == storage.StorageKindBlock
}

// Scope  is defined on the storage.Provider interface.
func (s storageProvider) Scope() storage.Scope {
	return storage.ScopeEnviron
}

// Dynamic  is defined on the storage.Provider interface.
func (s storageProvider) Dynamic() bool {
	return true
}

// Releasable is defined on the Provider interface.
func (s storageProvider) Releasable() bool {
	// TODO(axw) support releasing Oracle storage volumes.
	return false
}

// DefaultPools  is defined on the storage.Provider interface.
func (s storageProvider) DefaultPools() []*storage.Config {
	latencyPool, _ := storage.NewConfig("oracle-latency", oracleStorageProvideType, map[string]interface{}{
		oracleVolumeType: latencyPool,
	})
	return []*storage.Config{latencyPool}
}

// ValidateConfig  is defined on the storage.Provider interface.
func (s storageProvider) ValidateConfig(cfg *storage.Config) error {
	attrs := cfg.Attrs()
	if volType, ok := attrs[oracleVolumeType]; ok {
		switch kind := volType.(type) {
		case string:
			pool := volType.(string)
			if _, ok := poolTypeMap[poolType(pool)]; !ok {
				return errors.Errorf("invalid volume-type %q", volType)
			}
			return nil
		case poolType:
			if _, ok := poolTypeMap[volType.(poolType)]; !ok {
				return errors.Errorf("invalid volume-type %q", volType)
			}
			return nil
		default:
			return errors.Errorf("invalid volume-type %T", kind)
		}
	}
	return nil
}
