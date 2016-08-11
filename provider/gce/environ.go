// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"strings"
	"sync"

	"github.com/juju/errors"

	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/instance"
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
	name  string
	uuid  string
	cloud environs.CloudSpec
	gce   gceConnection

	lock sync.Mutex // lock protects access to ecfg
	ecfg *environConfig

	// namespace is used to create the machine and device hostnames.
	namespace instance.Namespace
}

// Function entry points defined as variables so they can be overridden
// for testing purposes.
var (
	newConnection = func(conn google.ConnectionConfig, creds *google.Credentials) (gceConnection, error) {
		return google.Connect(conn, creds)
	}
	destroyEnv = common.Destroy
	bootstrap  = common.Bootstrap
)

func newEnviron(cloud environs.CloudSpec, cfg *config.Config) (*environ, error) {
	ecfg, err := newConfig(cfg, nil)
	if err != nil {
		return nil, errors.Annotate(err, "invalid config")
	}

	credAttrs := cloud.Credential.Attributes()
	if cloud.Credential.AuthType() == jujucloud.JSONFileAuthType {
		contents := credAttrs[credAttrFile]
		credential, err := parseJSONAuthFile(strings.NewReader(contents))
		if err != nil {
			return nil, errors.Trace(err)
		}
		credAttrs = credential.Attributes()
	}

	credential := &google.Credentials{
		ClientID:    credAttrs[credAttrClientID],
		ProjectID:   credAttrs[credAttrProjectID],
		ClientEmail: credAttrs[credAttrClientEmail],
		PrivateKey:  []byte(credAttrs[credAttrPrivateKey]),
	}
	connectionConfig := google.ConnectionConfig{
		Region:    cloud.Region,
		ProjectID: credential.ProjectID,
	}

	// Connect and authenticate.
	conn, err := newConnection(connectionConfig, credential)
	if err != nil {
		return nil, errors.Trace(err)
	}
	namespace, err := instance.NewNamespace(cfg.UUID())
	if err != nil {
		return nil, errors.Trace(err)
	}

	return &environ{
		name:      ecfg.config.Name(),
		uuid:      ecfg.config.UUID(),
		cloud:     cloud,
		ecfg:      ecfg,
		gce:       conn,
		namespace: namespace,
	}, nil
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
	return simplestreams.CloudSpec{
		Region:   env.cloud.Region,
		Endpoint: env.cloud.Endpoint,
	}, nil
}

// SetConfig updates the env's configuration.
func (env *environ) SetConfig(cfg *config.Config) error {
	env.lock.Lock()
	defer env.lock.Unlock()

	ecfg, err := newConfig(cfg, env.ecfg.config)
	if err != nil {
		return errors.Annotate(err, "invalid config change")
	}
	env.ecfg = ecfg
	return nil
}

// Config returns the configuration data with which the env was created.
func (env *environ) Config() *config.Config {
	env.lock.Lock()
	defer env.lock.Unlock()
	return env.ecfg.config
}

// PrepareForBootstrap implements environs.Environ.
func (env *environ) PrepareForBootstrap(ctx environs.BootstrapContext) error {
	if ctx.ShouldVerifyCredentials() {
		if err := env.gce.VerifyCredentials(); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

// Create implements environs.Environ.
func (env *environ) Create(environs.CreateParams) error {
	if err := env.gce.VerifyCredentials(); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// Bootstrap creates a new instance, chosing the series and arch out of
// available tools. The series and arch are returned along with a func
// that must be called to finalize the bootstrap process by transferring
// the tools and installing the initial juju controller.
func (env *environ) Bootstrap(ctx environs.BootstrapContext, params environs.BootstrapParams) (*environs.BootstrapResult, error) {
	// Ensure the API server port is open (globally for all instances
	// on the network, not just for the specific node of the state
	// server). See LP bug #1436191 for details.
	ports := network.PortRange{
		FromPort: params.ControllerConfig.APIPort(),
		ToPort:   params.ControllerConfig.APIPort(),
		Protocol: "tcp",
	}
	if err := env.gce.OpenPorts(env.globalFirewallName(), ports); err != nil {
		return nil, errors.Trace(err)
	}
	return bootstrap(ctx, env, params)
}

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

// DestroyController implements the Environ interface.
func (env *environ) DestroyController(controllerUUID string) error {
	// TODO(wallyworld): destroy hosted model resources
	return env.Destroy()
}
