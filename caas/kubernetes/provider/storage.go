// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"fmt"
	"strings"

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
	storageClass       = "storage-class"
	storageProvisioner = "storage-provisioner"
	storageLabel       = "storage-label"

	// K8s storage pool attribute default values.
	defaultStorageClass = "juju-unit-storage"
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
	storageClass:       schema.String(),
	storageLabel:       schema.String(),
	storageProvisioner: schema.String(),
}

var storageConfigChecker = schema.FieldMap(
	storageConfigFields,
	schema.Defaults{
		storageClass:       defaultStorageClass,
		storageLabel:       schema.Omit,
		storageProvisioner: schema.Omit,
	},
)

type storageConfig struct {
	storageClass       string
	storageProvisioner string
	storageLabels      []string
	parameters         map[string]string
}

func newStorageConfig(attrs map[string]interface{}) (*storageConfig, error) {
	out, err := storageConfigChecker.Coerce(attrs, nil)
	if err != nil {
		return nil, errors.Annotate(err, "validating storage config")
	}
	coerced := out.(map[string]interface{})
	storageClassName := coerced[storageClass].(string)
	storageConfig := &storageConfig{
		storageClass: storageClassName,
	}
	if storageProvisioner, ok := coerced[storageProvisioner].(string); ok {
		storageConfig.storageProvisioner = storageProvisioner
	}
	if storageConfig.storageProvisioner != "" && storageConfig.storageClass == "" {
		return nil, errors.New("storage-class must be specified if storage-provisioner is specified")
	}
	storageConfig.parameters = make(map[string]string)
	for k, v := range attrs {
		k = strings.TrimPrefix(k, "parameters.")
		storageConfig.parameters[k] = fmt.Sprintf("%v", v)
	}
	delete(storageConfig.parameters, storageClass)
	delete(storageConfig.parameters, storageLabel)
	delete(storageConfig.parameters, storageProvisioner)

	return storageConfig, nil
}

// ValidateConfig is defined on the storage.Provider interface.
func (g *storageProvider) ValidateConfig(cfg *storage.Config) error {
	_, err := newStorageConfig(cfg.Attrs())
	return errors.Trace(err)
}

// Supports is defined on the storage.Provider interface.
func (g *storageProvider) Supports(k storage.StorageKind) bool {
	return k == storage.StorageKindFilesystem
}

// Scope is defined on the storage.Provider interface.
func (g *storageProvider) Scope() storage.Scope {
	return storage.ScopeMachine
}

// Dynamic is defined on the storage.Provider interface.
func (g *storageProvider) Dynamic() bool {
	return false
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
