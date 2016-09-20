// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !gccgo

package vsphere

import (
	"sync"

	"github.com/juju/errors"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/provider/common"
)

// TODO(ericsnow) All imports from github.com/juju/govmomi should change
// back to github.com/vmware/govmomi once our min Go is 1.3 or higher.

// Note: This provider/environment does *not* implement storage.

type environ struct {
	name   string
	cloud  environs.CloudSpec
	client *client

	// namespace is used to create the machine and device hostnames.
	namespace instance.Namespace

	lock sync.Mutex // lock protects access the following fields.
	ecfg *environConfig

	archLock               sync.Mutex
	supportedArchitectures []string
}

func newEnviron(cloud environs.CloudSpec, cfg *config.Config) (*environ, error) {
	ecfg, err := newValidConfig(cfg, configDefaults)
	if err != nil {
		return nil, errors.Annotate(err, "invalid config")
	}

	client, err := newClient(cloud)
	if err != nil {
		return nil, errors.Annotatef(err, "failed to create new client")
	}

	namespace, err := instance.NewNamespace(cfg.UUID())
	if err != nil {
		return nil, errors.Trace(err)
	}

	env := &environ{
		name:      ecfg.Name(),
		cloud:     cloud,
		ecfg:      ecfg,
		client:    client,
		namespace: namespace,
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

// Config returns the configuration data with which the env was created.
func (env *environ) Config() *config.Config {
	env.lock.Lock()
	cfg := env.ecfg.Config
	env.lock.Unlock()
	return cfg
}

// PrepareForBootstrap implements environs.Environ.
func (env *environ) PrepareForBootstrap(ctx environs.BootstrapContext) error {
	return nil
}

// Create implements environs.Environ.
func (env *environ) Create(environs.CreateParams) error {
	return nil
}

//this variable is exported, because it has to be rewritten in external unit tests
var Bootstrap = common.Bootstrap

// Bootstrap creates a new instance, chosing the series and arch out of
// available tools. The series and arch are returned along with a func
// that must be called to finalize the bootstrap process by transferring
// the tools and installing the initial juju controller.
func (env *environ) Bootstrap(ctx environs.BootstrapContext, params environs.BootstrapParams) (*environs.BootstrapResult, error) {
	return Bootstrap(ctx, env, params)
}

//this variable is exported, because it has to be rewritten in external unit tests
var DestroyEnv = common.Destroy

// Destroy shuts down all known machines and destroys the rest of the
// known environment.
func (env *environ) Destroy() error {
	return DestroyEnv(env)
}

// DestroyController implements the Environ interface.
func (env *environ) DestroyController(controllerUUID string) error {
	return env.Destroy()
}
