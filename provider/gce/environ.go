// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"strings"
	"sync"

	"code.google.com/p/google-api-go-client/compute/v1"
	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/provider/common"
)

// Note: This provider/environment does *not* implement storage.

const (
	metadataKeyRole = "juju-machine-role"
	// This is defined by the cloud-init code:
	// http://bazaar.launchpad.net/~cloud-init-dev/cloud-init/trunk/view/head:/cloudinit/sources/DataSourceGCE.py
	// http://cloudinit.readthedocs.org/en/latest/
	// https://cloud.google.com/compute/docs/metadata
	metadataKeyCloudInit = "user-data"
	metadataKeyEncoding  = "user-data-encoding"
	// GCE uses this specific key for authentication (*handwaving*)
	// https://cloud.google.com/compute/docs/instances#sshkeys
	metadataKeySSHKeys = "sshKeys"

	roleState = "state"

	// See https://cloud.google.com/compute/docs/operating-systems/linux-os#ubuntu
	// TODO(ericsnow) Should this be handled in cloud-images (i.e.
	// simplestreams)?
	imageBasePath = "projects/ubuntu-os-cloud/global/images/"
)

var (
	logger = loggo.GetLogger("juju.provider.gce")

	errNotImplemented = errors.NotImplementedf("gce provider")

	signedImageDataOnly = false
)

type environ struct {
	common.SupportsUnitPlacementPolicy

	name string

	lock sync.Mutex
	ecfg *environConfig

	gce *gceConnection
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
	err = env.gce.connect(env.ecfg.auth())

	return errors.Trace(err)
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
	return common.Destroy(env)
}

// instance stuff

var instStatuses = []string{statusPending, statusStaging, statusRunning}

// Instances returns the available instances in the environment that
// match the provided instance IDs. For IDs that did not match any
// instances, the result at the corresponding index will be nil. In that
// case the error will be environs.ErrPartialInstances (or
// ErrNoInstances if none of the IDs match an instance).
func (env *environ) Instances(ids []instance.Id) ([]instance.Instance, error) {
	if len(ids) == 0 {
		return nil, environs.ErrNoInstances
	}

	instances, err := env.instances()
	if err != nil {
		// We don't return the error since we need to pack one instance
		// for each ID into the result. If there is a problem then we
		// will return either ErrPartialInstances or ErrNoInstances.
		// TODO(ericsnow) Skip returning here only for certain errors?
		logger.Errorf("failed to get instances from GCE: %v", err)
		err = errors.Trace(err)
	}

	// Build the result, matching the provided instance IDs.
	numFound := 0 // This will never be greater than len(ids).
	results := make([]instance.Instance, len(ids))
	for i, id := range ids {
		inst := findInst(id, instances)
		if inst == nil {
			numFound += 1
		}
		results[i] = inst
	}

	if numFound == 0 {
		if err == nil {
			err = environs.ErrNoInstances
		}
	} else if numFound != len(ids) {
		err = environs.ErrPartialInstances
	}
	return results, err
}

func rawInstances(env *environ) ([]*compute.Instance, error) {
	// rawInstances() only returns instances that are part of the
	// environment (instance IDs matches "juju-<env name>-machine-*").
	// This is important because otherwise juju will see they are not
	// tracked in state, assume they're stale/rogue, and shut them down.
	instances, err := env.gce.instances(env)
	err = errors.Trace(err)

	// We further filter on the instance status, regardless of if there
	// was any error.
	instances = filterInstances(instances, instStatuses...)
	return instances, err
}

func (env *environ) instances() ([]instance.Instance, error) {
	env = env.getSnapshot()

	instances, err := rawInstances(env)
	err = errors.Trace(err)

	// Turn *compute.Instance values into *environInstance values,
	// whether or not we got an error.
	var results []instance.Instance
	for _, raw := range instances {
		inst := newInstance(raw, env)
		results = append(results, inst)
	}

	return results, err
}

// StateServerInstances returns the IDs of the instances corresponding
// to juju state servers.
func (env *environ) StateServerInstances() ([]instance.Id, error) {
	env = env.getSnapshot()

	instances, err := rawInstances(env)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var results []instance.Id
	for _, inst := range instances {
		role, ok := unpackMetadata(inst.Metadata)[metadataKeyRole]
		if ok && role == roleState {
			results = append(results, instance.Id(inst.Name))
		}
	}
	if len(results) == 0 {
		return nil, environs.ErrNotBootstrapped
	}
	return results, nil
}

func (env *environ) parsePlacement(placement string) (*gceAvailabilityZone, error) {
	pos := strings.IndexRune(placement, '=')
	if pos == -1 {
		return nil, errors.Errorf("unknown placement directive: %v", placement)
	}
	switch key, value := placement[:pos], placement[pos+1:]; key {
	case "zone":
		zoneName := value
		zones, err := env.AvailabilityZones()
		if err != nil {
			return nil, errors.Trace(err)
		}
		for _, z := range zones {
			if z.Name() == zoneName {
				return z.(*gceAvailabilityZone), nil
			}
		}
		return nil, errors.Errorf("invalid availability zone %q", zoneName)
	}
	return nil, errors.Errorf("unknown placement directive: %v", placement)
}

func checkInstanceType(cons constraints.Value) bool {
	// Constraint has an instance-type constraint so let's see if it is valid.
	for _, itype := range allInstanceTypes {
		if itype.Name == *cons.InstanceType {
			return true
		}
	}
	return false
}
