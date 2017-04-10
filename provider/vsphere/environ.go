// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vsphere

import (
	"sync"

	"github.com/juju/errors"
	"github.com/juju/version"
	"golang.org/x/net/context"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/provider/common"
)

// Note: This provider/environment does *not* implement storage.

type environ struct {
	name     string
	cloud    environs.CloudSpec
	provider *environProvider

	// namespace is used to create the machine and device hostnames.
	namespace instance.Namespace

	lock sync.Mutex // lock protects access the following fields.
	ecfg *environConfig

	archLock               sync.Mutex
	supportedArchitectures []string
}

func newEnviron(
	provider *environProvider,
	cloud environs.CloudSpec,
	cfg *config.Config,
) (*environ, error) {
	ecfg, err := newValidConfig(cfg, configDefaults)
	if err != nil {
		return nil, errors.Annotate(err, "invalid config")
	}

	namespace, err := instance.NewNamespace(cfg.UUID())
	if err != nil {
		return nil, errors.Trace(err)
	}

	env := &environ{
		name:      ecfg.Name(),
		cloud:     cloud,
		provider:  provider,
		ecfg:      ecfg,
		namespace: namespace,
	}
	return env, nil
}

func (env *environ) withClient(ctx context.Context, f func(Client) error) error {
	client, err := env.dialClient(ctx)
	if err != nil {
		return errors.Annotate(err, "dialing client")
	}
	defer client.Close(ctx)
	return f(client)
}

func (env *environ) dialClient(ctx context.Context) (Client, error) {
	return dialClient(ctx, env.cloud, env.provider.dial)
}

// Name is part of the environs.Environ interface.
func (env *environ) Name() string {
	return env.name
}

// Provider is part of the environs.Environ interface.
func (env *environ) Provider() environs.EnvironProvider {
	return env.provider
}

// SetConfig is part of the environs.Environ interface.
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

// Config is part of the environs.Environ interface.
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
func (env *environ) Create(args environs.CreateParams) error {
	return env.withSession(func(env *sessionEnviron) error {
		return env.Create(args)
	})
}

// Create implements environs.Environ.
func (env *sessionEnviron) Create(args environs.CreateParams) error {
	return nil
}

//this variable is exported, because it has to be rewritten in external unit tests
var Bootstrap = common.Bootstrap

// Bootstrap is part of the environs.Environ interface.
func (env *environ) Bootstrap(
	ctx environs.BootstrapContext,
	args environs.BootstrapParams,
) (result *environs.BootstrapResult, err error) {
	// NOTE(axw) we must not pass a sessionEnviron to common.Bootstrap,
	// as the Environ will be used during instance finalization after
	// the Bootstrap method returns, and the session will be invalid.
	return Bootstrap(ctx, env, args)
}

// Bootstrap is part of the environs.Environ interface.
func (env *sessionEnviron) Bootstrap(
	ctx environs.BootstrapContext,
	args environs.BootstrapParams,
) (result *environs.BootstrapResult, err error) {
	return nil, errors.Errorf("sessionEnviron.Bootstrap should never be called")
}

//this variable is exported, because it has to be rewritten in external unit tests
var DestroyEnv = common.Destroy

// AdoptResources is part of the Environ interface.
func (env *environ) AdoptResources(controllerUUID string, fromVersion version.Number) error {
	// This provider doesn't track instance -> controller.
	return nil
}

// AdoptResources is part of the Environ interface.
func (env *sessionEnviron) AdoptResources(controllerUUID string, fromVersion version.Number) error {
	return nil
}

// Destroy is part of the environs.Environ interface.
func (env *environ) Destroy() error {
	return env.withSession(func(env *sessionEnviron) error {
		return env.Destroy()
	})
}

// Destroy is part of the environs.Environ interface.
func (env *sessionEnviron) Destroy() error {
	return DestroyEnv(env)
}

// DestroyController implements the Environ interface.
func (env *environ) DestroyController(controllerUUID string) error {
	return env.withSession(func(env *sessionEnviron) error {
		return env.DestroyController(controllerUUID)
	})
}

// DestroyController implements the Environ interface.
func (env *sessionEnviron) DestroyController(controllerUUID string) error {
	return env.Destroy()
}
