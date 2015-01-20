// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package pool

import (
	"github.com/juju/errors"

	"github.com/juju/juju/storage"
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

// config encapsulates the config attributes for a storage pool.
type config struct {
	attrs map[string]interface{}
}

func (c *config) values() map[string]interface{} {
	copy := make(map[string]interface{})
	for k, v := range c.attrs {
		copy[k] = v
	}
	return copy
}

func (c *config) validate() error {
	//TODO: validate the storage provider type's value, not just that it is non empty.
	if c.attrs[Type] == "" {
		return MissingTypeError
	}
	if c.attrs[Name] == "" {
		return MissingNameError
	}
	return nil
}

var _ Pool = (*pool)(nil)

type pool struct {
	cfg *config
}

// Name is defined on Pool interface.
func (p *pool) Name() string {
	return p.cfg.attrs[Name].(string)
}

// Type is defined on Pool interface.
func (p *pool) Type() storage.ProviderType {
	return storage.ProviderType(p.cfg.attrs[Type].(string))
}

// Config is defined on Pool interface.
func (p *pool) Config() map[string]interface{} {
	return p.cfg.values()
}

// NewPoolManager returns a NewPoolManager implementation using the specified state.
func NewPoolManager(settings SettingsManager) PoolManager {
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
func (pm *poolManager) Create(name string, providerType storage.ProviderType, attrs map[string]interface{}) (Pool, error) {
	// Take a copy of the config and record name, type.
	poolAttrs := attrs
	poolAttrs[Name] = name
	poolAttrs[Type] = string(providerType)

	// Make sure we validate.
	cfg := &config{poolAttrs}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	if err := pm.settings.CreateSettings(globalKey(name), poolAttrs); err != nil {
		return nil, errors.Annotatef(err, "creating pool %q", name)
	}
	return &pool{cfg}, nil
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
func (pm *poolManager) Get(name string) (Pool, error) {
	settings, err := pm.settings.ReadSettings(globalKey(name))
	if err != nil {
		return nil, errors.Annotatef(err, "reading pool %q", name)
	}
	return &pool{&config{settings}}, nil
}

// List is defined on PoolManager interface.
func (pm *poolManager) List() ([]Pool, error) {
	settings, err := pm.settings.ListSettings(globalKeyPrefix)
	if err != nil {
		return nil, errors.Annotate(err, "listing pool settings")
	}
	var result []Pool
	for _, attrs := range settings {
		result = append(result, &pool{&config{attrs}})
	}
	return result, nil
}
