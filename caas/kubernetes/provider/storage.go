// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"github.com/juju/errors"
	"github.com/juju/juju/caas"
	"github.com/juju/schema"

	"github.com/juju/juju/storage"
)

const (
	// K8s_ProviderType defines the Juju storage type which can be used
	// to provision storage on k8s models.
	K8s_ProviderType = storage.ProviderType("kubernetes")

	// K8s storage pool attributes.
	storageClass = "storage-class"

	// K8s storage pool attribute default values.
	defaultStorageClass = "<default>"
)

// StorageProviderTypes is defined on the storage.ProviderRegistry interface.
func (k *kubernetesClient) StorageProviderTypes() ([]storage.ProviderType, error) {
	return []storage.ProviderType{K8s_ProviderType}, nil
}

// StorageProvider is defined on the storage.ProviderRegistry interface.
func (k *kubernetesClient) StorageProvider(t storage.ProviderType) (storage.Provider, error) {
	if t == K8s_ProviderType {
		return &storageProvider{k}, nil
	}
	return nil, errors.NotFoundf("storage provider %q", t)
}

type storageProvider struct {
	client caas.Broker
}

var _ storage.Provider = (*storageProvider)(nil)

var storageConfigFields = schema.Fields{
	storageClass: schema.String(),
}

var storageConfigChecker = schema.FieldMap(
	storageConfigFields,
	schema.Defaults{
		storageClass: defaultStorageClass,
	},
)

type storageConfig struct {
	storageClass string
}

func newStorageConfig(attrs map[string]interface{}) (*storageConfig, error) {
	out, err := storageConfigChecker.Coerce(attrs, nil)
	if err != nil {
		return nil, errors.Annotate(err, "validating storage config")
	}
	coerced := out.(map[string]interface{})
	storageClass := coerced[storageClass].(string)
	storageConfig := &storageConfig{
		storageClass: storageClass,
	}
	return storageConfig, nil
}

// ValidateConfig is defined on the storage.Provider interface.
func (g *storageProvider) ValidateConfig(cfg *storage.Config) error {
	_, err := newStorageConfig(cfg.Attrs())
	return errors.Trace(err)
}

// Supports is defined on the storage.Provider interface.
func (g *storageProvider) Supports(k storage.StorageKind) bool {
	// Support both block and filesystem storage.
	return true
}

// Scope is defined on the storage.Provider interface.
func (g *storageProvider) Scope() storage.Scope {
	return storage.ScopeEnviron
}

// Dynamic is defined on the storage.Provider interface.
func (g *storageProvider) Dynamic() bool {
	return true
}

// Releasable is defined on the storage.Provider interface.
func (e *storageProvider) Releasable() bool {
	return false
}

// DefaultPools is defined on the storage.Provider interface.
func (g *storageProvider) DefaultPools() []*storage.Config {
	return nil
}

// VolumeSource is defined on the storage.Provider interface.
func (g *storageProvider) VolumeSource(cfg *storage.Config) (storage.VolumeSource, error) {
	return nil, errors.NotSupportedf("volumes")
}

// FilesystemSource is defined on the storage.Provider interface.
func (g *storageProvider) FilesystemSource(providerConfig *storage.Config) (storage.FilesystemSource, error) {
	return nil, errors.NotSupportedf("filesystems")
}
