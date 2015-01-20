// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package pool

import (
	"github.com/juju/juju/storage"
)

// A Pool is a storage provider with configuration.
type Pool interface {
	// Name is the pool's name.
	Name() string

	// Type is the type of storage provider this pool represents, eg "loop", "ebs.
	Type() storage.ProviderType

	// Config is the pool's configuration attributes.
	Config() map[string]interface{}
}

// A PoolManager provides access to storage pools.
type PoolManager interface {
	// Create makes a new pool with the specified configuration and persists it to state.
	Create(name string, providerType storage.ProviderType, attrs map[string]interface{}) (Pool, error)

	// Delete removes the pool with name from state.
	Delete(name string) error

	// Get returns the pool with name from state.
	Get(name string) (Pool, error)

	// List returns all the pools from state.
	List() ([]Pool, error)
}

type SettingsManager interface {
	CreateSettings(key string, settings map[string]interface{}) error
	ReadSettings(key string) (map[string]interface{}, error)
	RemoveSettings(key string) error
	ListSettings(keyPrefix string) (map[string]map[string]interface{}, error)
}
