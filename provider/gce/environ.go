// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"sync"

	"github.com/juju/errors"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/provider/gce/gceapi"
)

// Note: This provider/environment does *not* implement storage.

type environ struct {
	common.SupportsUnitPlacementPolicy

	name string

	lock sync.Mutex
	ecfg *environConfig

	gce *gceapi.Connection
}

var _ environs.Environ = (*environ)(nil)
var _ simplestreams.HasRegion = (*environ)(nil)

// Name returns the name of the environment.
func (env *environ) Name() string {
	return env.name
}

// Provider returns the environment provider that created this env.
func (*environ) Provider() environs.EnvironProvider {
	return providerInstance
}

// Region returns the CloudSpec to use for the provider, as configured.
func (env *environ) Region() (simplestreams.CloudSpec, error) {
	cloudSpec, err := env.cloudSpec(env.ecfg.region())
	return cloudSpec, errors.Trace(err)
}

func (env *environ) cloudSpec(region string) (simplestreams.CloudSpec, error) {
	cloudSpec := simplestreams.CloudSpec{
		Region:   region,
		Endpoint: env.ecfg.imageEndpoint(),
	}
	return cloudSpec, nil
}

// SetConfig updates the env's configuration.
func (env *environ) SetConfig(cfg *config.Config) error {
	env.lock.Lock()
	defer env.lock.Unlock()

	// Build the config.
	var oldCfg *config.Config
	if env.ecfg != nil {
		oldCfg = env.ecfg.Config
	}
	cfg, err := providerInstance.Validate(cfg, oldCfg)
	if err != nil {
		return errors.Trace(err)
	}
	env.ecfg = &environConfig{cfg, cfg.UnknownAttrs()}

	// Connect and authenticate.
	env.gce = env.ecfg.newConnection()
	err = connect(env.gce, env.ecfg.auth())

	return errors.Trace(err)
}

var connect = func(conn *gceapi.Connection, auth gceapi.Auth) error {
	return conn.Connect(auth)
}

func (env *environ) getSnapshot() *environ {
	env.lock.Lock()
	clone := *env
	// The config values are all immutable so we don't need to also copy
	// env.ecfg and env.gce by value. If that changes we need to
	// re-evaluate copying them by value.
	env.lock.Unlock()

	clone.lock = sync.Mutex{}
	return &clone
}

// Config returns the configuration data with which the env was created.
func (env *environ) Config() *config.Config {
	return env.getSnapshot().ecfg.Config
}

// Bootstrap creates a new instance, chosing the series and arch out of
// available tools. The series and arch are returned along with a func
// that must be called to finalize the bootstrap process by transferring
// the tools and installing the initial juju state server.
func (env *environ) Bootstrap(ctx environs.BootstrapContext, params environs.BootstrapParams) (arch, series string, _ environs.BootstrapFinalizer, _ error) {
	return common.Bootstrap(ctx, env, params)
}

// Destroy shuts down all known machines and destroys the rest of the
// known environment.
func (env *environ) Destroy() error {
	ports, err := env.Ports()
	if err != nil {
		return errors.Trace(err)
	}

	if err := env.ClosePorts(ports); err != nil {
		return errors.Trace(err)
	}

	return common.Destroy(env)
}
