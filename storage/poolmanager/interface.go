// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package poolmanager

import (
	"strings"

	"github.com/juju/errors"

	"github.com/juju/juju/storage"
)

// A PoolManager provides access to storage pools.
type PoolManager interface {
	// Create makes a new pool with the specified configuration and persists it to state.
	Create(name string, providerType storage.ProviderType, attrs map[string]interface{}) (*storage.Config, error)

	// Delete removes the pool with name from state.
	Delete(name string) error

	// Replace replaces pool configuration with the newly provided values.
	Replace(name, provider string, attrs map[string]interface{}) error

	// Get returns the pool with name from state.
	Get(name string) (*storage.Config, error)

	// List returns all the pools from state.
	List() ([]*storage.Config, error)
}

type SettingsManager interface {
	CreateSettings(key string, settings map[string]interface{}) error
	ReplaceSettings(key string, settings map[string]interface{}) error
	ReadSettings(key string) (map[string]interface{}, error)
	RemoveSettings(key string) error
	ListSettings(keyPrefix string) (map[string]map[string]interface{}, error)
}

// MemSettings is an in-memory implementation of SettingsManager.
// This type does not provide any goroutine-safety.
type MemSettings struct {
	Settings map[string]map[string]interface{}
}

// CreateSettings is part of the SettingsManager interface.
func (m MemSettings) CreateSettings(key string, settings map[string]interface{}) error {
	if _, ok := m.Settings[key]; ok {
		return errors.AlreadyExistsf("settings with key %q", key)
	}
	m.Settings[key] = make(map[string]interface{})
	for k, v := range settings {
		m.Settings[key][k] = v
	}
	return nil
}

// ReplaceSettings is part of the SettingsManager interface.
func (m MemSettings) ReplaceSettings(key string, settings map[string]interface{}) error {
	if _, ok := m.Settings[key]; !ok {
		return errors.NotFoundf("settings with key %q", key)
	}
	m.Settings[key] = make(map[string]interface{})
	for k, v := range settings {
		m.Settings[key][k] = v
	}
	return nil
}

// ReadSettings is part of the SettingsManager interface.
func (m MemSettings) ReadSettings(key string) (map[string]interface{}, error) {
	settings, ok := m.Settings[key]
	if !ok {
		return nil, errors.NotFoundf("settings with key %q", key)
	}
	return settings, nil
}

// RemoveSettings is part of the SettingsManager interface.
func (m MemSettings) RemoveSettings(key string) error {
	if _, ok := m.Settings[key]; !ok {
		return errors.NotFoundf("settings with key %q", key)
	}
	delete(m.Settings, key)
	return nil
}

// ListSettings is part of the SettingsManager interface.
func (m MemSettings) ListSettings(keyPrefix string) (map[string]map[string]interface{}, error) {
	result := make(map[string]map[string]interface{})
	for key, settings := range m.Settings {
		if !strings.HasPrefix(key, keyPrefix) {
			continue
		}
		result[key] = settings
	}
	return result, nil
}
