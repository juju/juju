// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"sync"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/storage"
	"github.com/juju/juju/juju/arch"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/api"
)

// This file contains the core of the gce Environ implementation. You will
// probably not need to change this file very much to begin with; and if you
// never need to add any more fields, you may never need to touch it.
//
// The rest of the implementation is split into environ_instance.go (which
// must be implemented ) and environ_firewall.go (which can be safely
// ignored until you've got an environment bootstrapping successfully).

type environ struct {
	// This is used to check sanity of provisioning requests ahead of time. The
	// default implementation doesn't check anything; an ideal environ will use
	// its own PrecheckInstance method to prevent impossible provisioning
	// requests before they're made.
	common.NopPrecheckerPolicy

	// The SupportsUnitPlacementPolicy makes unit placement available on the
	// provider. The only reason to replace it would be if you were implementing
	// a provider like azure, in which we had to sacrifice unit placement in
	// favour of making it possible to keep services highly available.
	common.SupportsUnitPlacementPolicy

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

func (env *environ) Bootstrap(ctx environs.BootstrapContext, cons constraints.Value) error {
	// You can probably ignore this method; the common implementation should work.
	return common.Bootstrap(ctx, env, cons)
}

func (env *environ) StateInfo() (*state.Info, *api.Info, error) {
	// You can probably ignore this method; the common implementation should work.
	return common.StateInfo(env)
}

func (env *environ) Destroy() error {
	// You can probably ignore this method; the common implementation should work.
	return common.Destroy(env)
}

// SupportedArchitectures is specified on the EnvironCapability interface.
func (env *environ) SupportedArchitectures() ([]string, error) {
	// An ideal implementation will inspect the tools, images, and instance types
	// available in the environment to return correct values here.
	return arch.AllSupportedArches, nil
}

// SupportNetworks is specified on the EnvironCapability interface.
func (env *environ) SupportNetworks() bool {
	// An ideal implementation will support networking.
	return false
}
