// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package pool

import (
	"github.com/juju/errors"

	"github.com/juju/juju/state"
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
	attrs := c.attrs
	return attrs
}

func (c *config) validate() error {
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
func NewPoolManager(st *state.State) PoolManager {
	return &poolManager{st}
}

var _ PoolManager = (*poolManager)(nil)

type poolManager struct {
	st *state.State
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
	if _, err := state.CreateSettings(pm.st, globalKey(name), poolAttrs); err != nil {
		return nil, errors.Annotatef(err, "creating pool %q", name)
	}
	return &pool{cfg}, nil
}

// Delete is defined on PoolManager interface.
func (pm *poolManager) Delete(name string) error {
	err := state.RemoveSettings(pm.st, globalKey(name))
	if err == nil || errors.IsNotFound(err) {
		return nil
	}
	return errors.Annotatef(err, "deleting pool %q", name)
}

// Pool is defined on PoolManager interface.
func (pm *poolManager) Pool(name string) (Pool, error) {
	settings, err := state.ReadSettings(pm.st, globalKey(name))
	if err != nil {
		return nil, errors.Annotatef(err, "reading pool %q", name)
	}
	return &pool{&config{settings.Map()}}, nil
}

// List is defined on PoolManager interface.
func (pm *poolManager) List() ([]Pool, error) {
	settings, err := state.ListSettings(pm.st, globalKeyPrefix)
	if err != nil {
		return nil, errors.Annotate(err, "listing pool settings")
	}
	result := make([]Pool, len(settings))
	for i, attrs := range settings {
		result[i] = &pool{&config{attrs}}
	}
	return result, nil
}
