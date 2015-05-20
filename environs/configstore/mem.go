// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package configstore

import (
	"fmt"
	"sync"

	"github.com/juju/errors"
)

type memStore struct {
	mu   sync.Mutex
	envs map[string]*memInfo
}

type memInfo struct {
	store *memStore
	name  string
	environInfo
}

// clone returns a copy of the given environment info, isolated
// from the store itself.
func (info *memInfo) clone() *memInfo {
	// Note that none of the Set* methods ever set fields inside
	// references, which makes this OK to do.
	info1 := *info
	newAttrs := make(map[string]interface{})
	for name, attr := range info.bootstrapConfig {
		newAttrs[name] = attr
	}
	info1.bootstrapConfig = newAttrs
	info1.source = sourceMem
	return &info1
}

// NewMem returns a ConfigStorage implementation that
// stores configuration in memory.
func NewMem() Storage {
	return &memStore{
		envs: make(map[string]*memInfo),
	}
}

// CreateInfo implements Storage.CreateInfo.
func (m *memStore) CreateInfo(envName string) EnvironInfo {
	m.mu.Lock()
	defer m.mu.Unlock()
	info := &memInfo{
		store: m,
		name:  envName,
	}
	info.source = sourceCreated
	return info
}

// List implements Storage.List
func (m *memStore) List() ([]string, error) {
	var envs []string
	m.mu.Lock()
	defer m.mu.Unlock()
	for name, env := range m.envs {
		api := env.APIEndpoint()
		if api.ServerUUID == "" || api.EnvironUUID != "" {
			envs = append(envs, name)
		}
	}
	return envs, nil
}

// ListSystems implements Storage.ListSystems
func (m *memStore) ListSystems() ([]string, error) {
	var servers []string
	m.mu.Lock()
	defer m.mu.Unlock()
	for name, env := range m.envs {
		api := env.APIEndpoint()
		if api.ServerUUID == "" ||
			api.ServerUUID == api.EnvironUUID ||
			api.EnvironUUID == "" {
			servers = append(servers, name)
		}
	}
	return servers, nil
}

// ReadInfo implements Storage.ReadInfo.
func (m *memStore) ReadInfo(envName string) (EnvironInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	info := m.envs[envName]
	if info != nil {
		return info.clone(), nil
	}
	return nil, errors.NotFoundf("environment %q", envName)
}

// Location implements EnvironInfo.Location.
func (info *memInfo) Location() string {
	return "memory"
}

// Write implements EnvironInfo.Write.
func (info *memInfo) Write() error {
	m := info.store
	m.mu.Lock()
	defer m.mu.Unlock()

	if !info.initialized() && m.envs[info.name] != nil {
		return ErrEnvironInfoAlreadyExists
	}

	info.source = sourceMem
	m.envs[info.name] = info.clone()
	return nil
}

// Destroy implements EnvironInfo.Destroy.
func (info *memInfo) Destroy() error {
	m := info.store
	m.mu.Lock()
	defer m.mu.Unlock()
	if info.initialized() {
		if m.envs[info.name] == nil {
			return fmt.Errorf("environment info has already been removed")
		}
		delete(m.envs, info.name)
	}
	return nil
}
