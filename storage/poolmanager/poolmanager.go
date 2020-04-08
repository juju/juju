// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package poolmanager

import (
	"github.com/juju/errors"

	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/provider"
)

const (
	// Pool configuration attribute names.
	Name = "name"
	Type = "type"
)

var (
	MissingTypeError = errors.New("provider type is missing")
	MissingNameError = errors.New("pool name is missing")
)

// New returns a PoolManager implementation using the specified state.
func New(settings SettingsManager, registry storage.ProviderRegistry) PoolManager {
	return &poolManager{settings, registry}
}

var _ PoolManager = (*poolManager)(nil)

type poolManager struct {
	settings SettingsManager
	registry storage.ProviderRegistry
}

const globalKeyPrefix = "pool#"

func globalKey(name string) string {
	return globalKeyPrefix + name
}

// Create is defined on PoolManager interface.
func (pm *poolManager) Create(name string, providerType storage.ProviderType, attrs map[string]interface{}) (*storage.Config, error) {
	if name == "" {
		return nil, MissingNameError
	}
	if providerType == "" {
		return nil, MissingTypeError
	}

	cfg, err := pm.validatedConfig(name, providerType, attrs)
	if err != nil {
		return nil, errors.Trace(err)
	}

	poolAttrs := cfg.Attrs()
	poolAttrs[Name] = name
	poolAttrs[Type] = string(providerType)
	if err := pm.settings.CreateSettings(globalKey(name), poolAttrs); err != nil {
		return nil, errors.Annotatef(err, "creating pool %q", name)
	}
	return cfg, nil
}

func (pm *poolManager) validatedConfig(name string, providerType storage.ProviderType, attrs map[string]interface{}) (*storage.Config, error) {
	cfg, err := storage.NewConfig(name, providerType, attrs)
	if err != nil {
		return nil, errors.Trace(err)
	}
	p, err := pm.registry.StorageProvider(providerType)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if err := provider.ValidateConfig(p, cfg); err != nil {
		return nil, errors.Annotate(err, "validating storage provider config")
	}

	return cfg, nil
}

// Delete is defined on PoolManager interface.
func (pm *poolManager) Delete(name string) error {
	err := pm.settings.RemoveSettings(globalKey(name))
	if errors.IsNotFound(err) {
		return errors.NotFoundf("storage pool %q", name)
	}
	return errors.Annotatef(err, "deleting pool %q", name)
}

// Replace is defined on PoolManager interface.
func (pm *poolManager) Replace(name, provider string, attrs map[string]interface{}) error {
	if name == "" {
		return MissingNameError
	}
	var providerType storage.ProviderType
	// Use the existing provider type unless explicitly overwritten.
	if provider != "" {
		providerType = storage.ProviderType(provider)
	} else {
		existingConfig, err := pm.Get(name)
		if err != nil {
			return errors.Trace(err)
		}
		providerType = existingConfig.Provider()
	}
	attrs[Type] = providerType
	attrs[Name] = name
	cfg, err := pm.validatedConfig(name, providerType, attrs)
	if err != nil {
		return errors.Trace(err)
	}
	validatedAttrs := cfg.Attrs()
	validatedAttrs[Name] = name
	validatedAttrs[Type] = string(providerType)
	return pm.settings.ReplaceSettings(globalKey(name), attrs)
}

// Get is defined on PoolManager interface.
func (pm *poolManager) Get(name string) (*storage.Config, error) {
	settings, err := pm.settings.ReadSettings(globalKey(name))
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, errors.NotFoundf("pool %q", name)
		} else {
			return nil, errors.Annotatef(err, "reading pool %q", name)
		}
	}
	return pm.configFromSettings(settings)
}

// List is defined on PoolManager interface.
func (pm *poolManager) List() ([]*storage.Config, error) {
	settings, err := pm.settings.ListSettings(globalKeyPrefix)
	if err != nil {
		return nil, errors.Annotate(err, "listing pool settings")
	}
	var result []*storage.Config
	for _, attrs := range settings {
		cfg, err := pm.configFromSettings(attrs)
		if err != nil {
			return nil, errors.Trace(err)
		}
		result = append(result, cfg)
	}
	return result, nil
}

func (pm *poolManager) configFromSettings(settings map[string]interface{}) (*storage.Config, error) {
	providerType := storage.ProviderType(settings[Type].(string))
	name := settings[Name].(string)
	// Ensure returned attributes are stripped of name and type,
	// as these are not user-specified attributes.
	delete(settings, Name)
	delete(settings, Type)
	cfg, err := storage.NewConfig(name, providerType, settings)
	if err != nil {
		return nil, errors.Trace(err)
	}
	p, err := pm.registry.StorageProvider(providerType)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if err := provider.ValidateConfig(p, cfg); err != nil {
		return nil, errors.Trace(err)
	}
	return cfg, nil
}
