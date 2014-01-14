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

type JoyentEnviron struct {
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

var _ environs.Environ = (*JoyentEnviron)(nil)

func NewEnviron(cfg *config.Config) (*JoyentEnviron, error) {
	env := new(JoyentEnviron)
	err := env.SetConfig(cfg)
	if err != nil {
		return nil, err
	}
	env.name = cfg.Name()
	env.storage = NewStorage(env)
	return env, nil
}

func (env *JoyentEnviron) SetName(envName string) {
	env.name = envName
}

func (env *JoyentEnviron) Name() string {
	return env.name
}

func (*JoyentEnviron) Provider() environs.EnvironProvider {
	return providerInstance
}

func (env *JoyentEnviron) SetConfig(cfg *config.Config) error {
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

func (env *JoyentEnviron) getSnapshot() *JoyentEnviron {
	env.lock.Lock()
	clone := *env
	env.lock.Unlock()
	clone.lock = sync.Mutex{}
	return &clone
}

func (env *JoyentEnviron) Config() *config.Config {
	return env.getSnapshot().ecfg.Config
}

func (env *JoyentEnviron) Storage() storage.Storage {
	return env.getSnapshot().storage
}

func (env *JoyentEnviron) PublicStorage() storage.StorageReader {
	return environs.EmptyStorage
}

func (env *JoyentEnviron) Bootstrap(ctx environs.BootstrapContext, cons constraints.Value) error {
	return common.Bootstrap(ctx, env, cons)
}

func (env *JoyentEnviron) StateInfo() (*state.Info, *api.Info, error) {
	return common.StateInfo(env)
}

func (env *JoyentEnviron) Destroy() error {
	return common.Destroy(env)
}
