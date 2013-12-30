// Copyright 2013 Joyent Inc.
// Licensed under the AGPLv3, see LICENCE file for details.

package joyent

import (
	"sync"

	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/environs/storage"
	"launchpad.net/juju-core/provider/common"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
)

// This file contains the core of the joyent Environ implementation. You will
// probably not need to change this file very much to begin with; and if you
// never need to add any more fields, you may never need to touch it.
//
// The rest of the implementation is split into environ_instance.go (which
// must be implemented ) and environ_firewall.go (which can be safely
// ignored until you've got an environment bootstrapping successfully).

type joyentEnviron struct {
	name string
	// All mutating operations should lock the mutex. Non-mutating operations
	// should read all fields (other than name, which is immutable) from a
	// shallow copy taken with getSnapshot().
	// This advice is predicated on the goroutine-safety of the values of the
	// affected fields.
	lock    sync.Mutex
	ecfg    *environConfig
	storage storage.Storage
}

var _ environs.Environ = (*joyentEnviron)(nil)

func NewEnviron(cfg *config.Config) (*joyentEnviron, error) {
	env := new(joyentEnviron)
	err := env.SetConfig(cfg)
	if err != nil {
		return nil, err
	}
	env.name = cfg.Name()
	env.storage = NewStorage(env)
	return env, nil
}

func (env *joyentEnviron) Name() string {
	return env.name
}

func (*joyentEnviron) Provider() environs.EnvironProvider {
	return providerInstance
}

func (env *joyentEnviron) SetConfig(cfg *config.Config) error {
	env.lock.Lock()
	defer env.lock.Unlock()
	ecfg, err := validateConfig(cfg, env.ecfg)
	if err != nil {
		return err
	}
	storage, err := newStorage(ecfg)
	if err != nil {
		return err
	}
	env.ecfg = ecfg
	env.storage = storage
	return nil
}

func (env *joyentEnviron) getSnapshot() *joyentEnviron {
	env.lock.Lock()
	clone := *env
	env.lock.Unlock()
	clone.lock = sync.Mutex{}
	return &clone
}

func (env *joyentEnviron) Config() *config.Config {
	return env.getSnapshot().ecfg.Config
}

func (env *joyentEnviron) Storage() storage.Storage {
	return env.getSnapshot().storage
}

func (env *joyentEnviron) PublicStorage() storage.StorageReader {
	return environs.EmptyStorage
}

func (env *joyentEnviron) Bootstrap(cons constraints.Value) error {
	return common.Bootstrap(env, cons)
}

func (env *joyentEnviron) StateInfo() (*state.Info, *api.Info, error) {
	return common.StateInfo(env)
}

func (env *joyentEnviron) Destroy() error {
	return common.Destroy(env)
}
