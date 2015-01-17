// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"sync"

	"github.com/juju/errors"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/provider/gce/google"
)

// Note: This provider/environment does *not* implement storage.

type gceConnection interface {
	Connect(auth google.Auth) error
	VerifyCredentials() error

	// Instance gets the up-to-date info about the given instance
	// and returns it.
	Instance(id, zone string) (*google.Instance, error)
	Instances(prefix string, statuses ...string) ([]google.Instance, error)
	AddInstance(spec google.InstanceSpec, zones []string) (*google.Instance, error)
	RemoveInstances(prefix string, ids ...string) error

	Ports(fwname string) ([]network.PortRange, error)
	OpenPorts(fwname string, ports []network.PortRange) error
	ClosePorts(fwname string, ports []network.PortRange) error

	AvailabilityZones(region string) ([]google.AvailabilityZone, error)
}

type environ struct {
	common.SupportsUnitPlacementPolicy

	name string
	uuid string
	ecfg *environConfig
	gce  gceConnection

	lock     sync.Mutex
	archLock sync.Mutex

	supportedArchitectures []string
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
	env.ecfg = newEnvConfig(cfg)

	// Connect and authenticate.
	env.gce = newConnection(env.ecfg)
	err = env.gce.Connect(env.ecfg.auth())
	return errors.Trace(err)
}

var newConnection = func(ecfg *environConfig) gceConnection {
	return ecfg.newConnection()
}

func newEnvConfig(cfg *config.Config) *environConfig {
	return &environConfig{
		Config: cfg,
		attrs:  cfg.UnknownAttrs(),
	}
}

// getSnapshot returns a copy of the environment. This is useful for
// ensuring the env you are using does not get changed by other code
// while you are using it.
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

var bootstrap = common.Bootstrap

// Bootstrap creates a new instance, chosing the series and arch out of
// available tools. The series and arch are returned along with a func
// that must be called to finalize the bootstrap process by transferring
// the tools and installing the initial juju state server.
func (env *environ) Bootstrap(ctx environs.BootstrapContext, params environs.BootstrapParams) (arch, series string, _ environs.BootstrapFinalizer, _ error) {
	return bootstrap(ctx, env, params)
}

var destroyEnv = common.Destroy

// Destroy shuts down all known machines and destroys the rest of the
// known environment.
func (env *environ) Destroy() error {
	ports, err := env.Ports()
	if err != nil {
		return errors.Trace(err)
	}

	if len(ports) > 0 {
		if err := env.ClosePorts(ports); err != nil {
			return errors.Trace(err)
		}
	}

	return destroyEnv(env)
}
