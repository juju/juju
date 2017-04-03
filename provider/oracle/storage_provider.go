package oracle

import (
	ociCommon "github.com/juju/go-oracle-cloud/common"

	"github.com/juju/errors"
	"github.com/juju/juju/storage"
)

type poolType string

const (
	latencyPool      poolType = "latency"
	defaultPool      poolType = "default"
	oracleVolumeType string   = "volume-type"
)

var poolTypeMap map[poolType]ociCommon.StoragePool = map[poolType]ociCommon.StoragePool{
	latencyPool: ociCommon.LatencyPool,
	defaultPool: ociCommon.DefaultPool,
}

// VolumeSource returns a VolumeSource given the specified storage
// provider configurations, or an error if the provider does not
// support creating volumes or the configuration is invalid.
//
// If the storage provider does not support creating volumes as a
// first-class primitive, then VolumeSource must return an error
// satisfying errors.IsNotSupported.
func (s *storageProvider) VolumeSource(cfg *storage.Config) (storage.VolumeSource, error) {
	environConfig := s.env.Config()
	return &oracleVolumeSource{
		env:       s.env,
		envName:   environConfig.Name(),
		modelUUID: environConfig.UUID(),
		api:       s.env.client,
	}, nil
}

// FilesystemSource returns a FilesystemSource given the specified
// storage provider configurations, or an error if the provider does
// not support creating filesystems or the configuration is invalid.
func (s storageProvider) FilesystemSource(cfg *storage.Config) (storage.FilesystemSource, error) {
	return nil, errors.NotSupportedf("filesystemsource")
}

// Supports reports whether or not the storage provider supports
// the specified storage kind.
//
// A provider that supports volumes but not filesystems can still
// be used for creating filesystem storage; Juju will request a
// volume from the provider and then manage the filesystem itself.
func (s storageProvider) Supports(kind storage.StorageKind) bool {
	return kind == storage.StorageKindBlock
}

// Scope returns the scope of storage managed by this provider.
func (s storageProvider) Scope() storage.Scope {
	return storage.ScopeEnviron
}

// Dynamic reports whether or not the storage provider is capable
// of dynamic storage provisioning. Non-dynamic storage must be
// created at the time a machine is provisioned.
func (s storageProvider) Dynamic() bool {
	return true
}

// DefaultPools returns the default storage pools for this provider,
// to register in each new model.
func (s storageProvider) DefaultPools() []*storage.Config {
	latencyPool, _ := storage.NewConfig("oracle-latency", oracleStorageProvideType, map[string]interface{}{
		oracleVolumeType: latencyPool,
	})
	return []*storage.Config{latencyPool}
}

// ValidateConfig validates the provided storage provider config,
// returning an error if it is invalid.
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
