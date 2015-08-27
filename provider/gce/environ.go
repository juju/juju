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

type gceConnection interface {
	VerifyCredentials() error

	// Instance gets the up-to-date info about the given instance
	// and returns it.
	Instance(id, zone string) (google.Instance, error)
	Instances(prefix string, statuses ...string) ([]google.Instance, error)
	AddInstance(spec google.InstanceSpec, zones ...string) (*google.Instance, error)
	RemoveInstances(prefix string, ids ...string) error

	Ports(fwname string) ([]network.PortRange, error)
	OpenPorts(fwname string, ports ...network.PortRange) error
	ClosePorts(fwname string, ports ...network.PortRange) error

	AvailabilityZones(region string) ([]google.AvailabilityZone, error)

	// Storage related methods.

	// CreateDisks will attempt to create the disks described by <disks> spec and
	// return a slice of Disk representing the created disks or error if one of them failed.
	CreateDisks(zone string, disks []google.DiskSpec) ([]*google.Disk, error)
	// Disks will return a list of Disk found the passed <zone>.
	Disks(zone string) ([]*google.Disk, error)
	// Disk will return a Disk representing the disk identified by the
	// passed <name> or error.
	Disk(zone, id string) (*google.Disk, error)
	// RemoveDisk will destroy the disk identified by <name> in <zone>.
	RemoveDisk(zone, id string) error
	// AttachDisk will attach the volume identified by <volumeName> into the instance
	// <instanceId> and return an AttachedDisk representing it or error.
	AttachDisk(zone, volumeName, instanceId string, mode google.DiskMode) (*google.AttachedDisk, error)
	// DetachDisk will detach <volumeName> disk from <instanceId> if possible
	// and return error.
	DetachDisk(zone, instanceId, volumeName string) error
	// InstanceDisks returns a list of the disks attached to the passed instance.
	InstanceDisks(zone, instanceId string) ([]*google.AttachedDisk, error)
}

type environ struct {
	common.SupportsUnitPlacementPolicy

	name string
	uuid string
	gce  gceConnection

	lock sync.Mutex
	ecfg *environConfig

	archLock               sync.Mutex
	supportedArchitectures []string
}

func newEnviron(cfg *config.Config) (*environ, error) {
	ecfg, err := newValidConfig(cfg, configDefaults)
	if err != nil {
		return nil, errors.Annotate(err, "invalid config")
	}

	uuid, ok := ecfg.UUID()
	if !ok {
		return nil, errors.New("UUID not set")
	}

	// Connect and authenticate.
	conn, err := newConnection(ecfg)
	if err != nil {
		return nil, errors.Trace(err)
	}

	env := &environ{
		name: ecfg.Name(),
		uuid: uuid,
		ecfg: ecfg,
		gce:  conn,
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

// Region returns the CloudSpec to use for the provider, as configured.
func (env *environ) Region() (simplestreams.CloudSpec, error) {
	return env.cloudSpec(env.ecfg.region()), nil
}

func (env *environ) cloudSpec(region string) simplestreams.CloudSpec {
	return simplestreams.CloudSpec{
		Region:   region,
		Endpoint: env.ecfg.imageEndpoint(),
	}
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

var newConnection = func(ecfg *environConfig) (gceConnection, error) {
	connCfg := ecfg.newConnection()
	auth := ecfg.auth()
	return google.Connect(connCfg, auth)
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
