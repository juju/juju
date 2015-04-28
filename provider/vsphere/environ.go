// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !gccgo

package vsphere

import (
	"sync"

	"github.com/juju/errors"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/provider/common"
)

// TODO(ericsnow) All imports from github.com/juju/govmomi should change
// back to github.com/vmware/govmomi once our min Go is 1.3 or higher.

// Note: This provider/environment does *not* implement storage.

type environ struct {
	common.SupportsUnitPlacementPolicy

	name   string
	ecfg   *environConfig
	client *client

	lock     sync.Mutex
	archLock sync.Mutex

	supportedArchitectures []string
}

func newEnviron(cfg *config.Config) (*environ, error) {
	ecfg, err := newValidConfig(cfg, configDefaults)
	if err != nil {
		return nil, errors.Annotate(err, "invalid config")
	}

	client, err := newClient(ecfg)
	if err != nil {
		return nil, errors.Annotatef(err, "failed to create new client")
	}

	env := &environ{
		name:   ecfg.Name(),
		ecfg:   ecfg,
		client: client,
	}
	return env, nil
}

// Name returns the name of the environment.
func (env *environ) Name() string {
	return env.name
}

// Provider returns the environment provider that created this env.
func (*environ) Provider() environs.EnvironProvider {
	return providerInstance
}

// SetConfig updates the env's configuration.
func (env *environ) SetConfig(cfg *config.Config) error {
	env.lock.Lock()
	defer env.lock.Unlock()

	if env.ecfg == nil {
		return errors.New("cannot set config on uninitialized env")
	}

	if err := env.ecfg.update(cfg); err != nil {
		return errors.Annotate(err, "invalid config change")
	}
	return nil
}

// getSnapshot returns a copy of the environment. This is useful for
// ensuring the env you are using does not get changed by other code
// while you are using it.
func (env environ) getSnapshot() *environ {
	return &env
}

// Config returns the configuration data with which the env was created.
func (env *environ) Config() *config.Config {
	return env.getSnapshot().ecfg.Config
}

//this variable is exported, because it has to be rewritten in external unit tests
var Bootstrap = common.Bootstrap

// Bootstrap creates a new instance, chosing the series and arch out of
// available tools. The series and arch are returned along with a func
// that must be called to finalize the bootstrap process by transferring
// the tools and installing the initial juju state server.
func (env *environ) Bootstrap(ctx environs.BootstrapContext, params environs.BootstrapParams) (arch, series string, _ environs.BootstrapFinalizer, _ error) {
	return Bootstrap(ctx, env, params)
}

//this variable is exported, because it has to be rewritten in external unit tests
var DestroyEnv = common.Destroy

// Destroy shuts down all known machines and destroys the rest of the
// known environment.
func (env *environ) Destroy() error {
	return DestroyEnv(env)
}
