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

type environ struct {
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

var _ environs.Environ = (*environ)(nil)

func (env *environ) Name() string {
	return env.name
}

func (*environ) Provider() environs.EnvironProvider {
	return providerInstance
}

func (env *environ) SetConfig(cfg *config.Config) error {
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

func (env *environ) getSnapshot() *environ {
	env.lock.Lock()
	clone := *env
	env.lock.Unlock()
	clone.lock = sync.Mutex{}
	return &clone
}

func (env *environ) Config() *config.Config {
	return env.getSnapshot().ecfg.Config
}

func (env *environ) Storage() storage.Storage {
	return env.getSnapshot().storage
}

func (env *environ) PublicStorage() storage.StorageReader {
	return environs.EmptyStorage
}

func (env *environ) Bootstrap(ctx environs.BootstrapContext, cons constraints.Value) error {
	return common.Bootstrap(ctx, env, cons)
}

func (env *environ) StateInfo() (*state.Info, *api.Info, error) {
	return common.StateInfo(env)
}

func (env *environ) Destroy() error {
	return common.Destroy(env)
}
