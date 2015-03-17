// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package poolmanager

import (
	"github.com/juju/errors"

	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/provider/registry"
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
func New(settings SettingsManager) PoolManager {
	return &poolManager{settings}
}

var _ PoolManager = (*poolManager)(nil)

type poolManager struct {
	settings SettingsManager
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

	cfg, err := storage.NewConfig(name, providerType, attrs)
	if err != nil {
		return nil, err
	}
	// Take a copy of the config and record name, type.
	poolAttrs := make(map[string]interface{}, len(attrs))
	for k, v := range attrs {
		poolAttrs[k] = v
	}
	// Instantiate the provider to validate config.
	p, err := registry.StorageProvider(providerType)
	if err != nil {
		return nil, err
	}
	if err := p.ValidateConfig(cfg); err != nil {
		return nil, err
	}

	poolAttrs[Name] = name
	poolAttrs[Type] = string(providerType)
	if err := pm.settings.CreateSettings(globalKey(name), poolAttrs); err != nil {
		return nil, errors.Annotatef(err, "creating pool %q", name)
	}
	return cfg, nil
}

// Delete is defined on PoolManager interface.
func (pm *poolManager) Delete(name string) error {
	err := pm.settings.RemoveSettings(globalKey(name))
	if err == nil || errors.IsNotFound(err) {
		return nil
	}
	return errors.Annotatef(err, "deleting pool %q", name)
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
	providerType := storage.ProviderType(settings[Type].(string))
	// Ensure returned attributes are stripped
	// of name and type as these are not core settings values.
	delete(settings, Name)
	delete(settings, Type)
	return storage.NewConfig(name, providerType, settings)
}

// List is defined on PoolManager interface.
func (pm *poolManager) List() ([]*storage.Config, error) {
	settings, err := pm.settings.ListSettings(globalKeyPrefix)
	if err != nil {
		return nil, errors.Annotate(err, "listing pool settings")
	}
	var result []*storage.Config
	for _, attrs := range settings {
		name := attrs[Name].(string)
		providerType := storage.ProviderType(attrs[Type].(string))
		// Ensure returned attributes are stripped
		// of name and type as these are not core settings values.
		delete(attrs, Name)
		delete(attrs, Type)
		if cfg, err := storage.NewConfig(name, providerType, attrs); err != nil {
			return nil, err
		} else {
			result = append(result, cfg)
		}
	}
	return result, nil
}
