// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package configstore

import (
	"fmt"
	"sync"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/errors"
)

type memStore struct {
	mu sync.Mutex
	envs map[string] *memInfo
}

type memInfo struct {
	store *memStore
	name string
	environInfo
}

// clone returns a copy of the given environment info, isolated
// from the store itself.
func (info *memInfo) clone() *memInfo {
	// Note that none of the Set* methods ever set fields inside
	// references, which makes this OK to do.
	info1 := *info
	return &info1
}

// NewMem returns a ConfigStorage implementation that
// stores configuration in memory.
func NewMem(dir string) (environs.ConfigStorage, error) {
	return &memStore{}, nil
}

// CreateInfo implements environs.ConfigStorage.CreateInfo.
func (m *memStore) CreateInfo(envName string) (environs.EnvironInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.envs[envName] != nil {
		return nil, environs.ErrEnvironInfoAlreadyExists
	}
	info := &memInfo{
		store: m,
		name: envName,
	}
	m.envs[envName] = info.clone()
	return info, nil
}

// ReadInfo implements environs.ConfigStorage.ReadInfo.
func (m *memStore) ReadInfo(envName string) (environs.EnvironInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	info := m.envs[envName]
	if info != nil {
		return info.clone(), nil
	}
	return nil, errors.NotFoundf("environment %q", envName)
}

// Write implements environs.EnvironInfo.Write.
func (info *memInfo) Write() error {
	m := info.store
	m.mu.Lock()
	defer m.mu.Unlock()
	info.initialized = true
	m.envs[info.name] = info.clone()
	return nil
}

// Destroy implements environs.EnvironInfo.Destroy.
func (info *memInfo) Destroy() error {
	m := info.store
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.envs[info.name] == nil {
		return fmt.Errorf("environment info has already been removed")
	}
	delete(m.envs, info.name)
	return nil
}
