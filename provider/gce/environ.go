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
	"github.com/juju/juju/environs/storage"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/provider/common"
)

const (
	metadataKeyRole      = "juju-machine-role"
	metadataKeyCloudInit = "metadata.cloud-init:user-data"

	roleState = "state"
)

var (
	logger = loggo.GetLogger("juju.provider.gce")

	errNotImplemented = errors.NotImplementedf("gce provider")

	signedImageDataOnly = false
)

type environ struct {
	common.SupportsUnitPlacementPolicy

	name string

	lock    sync.Mutex
	ecfg    *environConfig
	storage storage.Storage

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
		Endpoint: env.ecfg.imageURL(),
	}
	return cloudSpec, nil
}

// SetConfig updates the env's configuration.
func (env *environ) SetConfig(cfg *config.Config) error {
	env.lock.Lock()
	defer env.lock.Unlock()
	var oldCfg *config.Config
	if env.ecfg != nil {
		oldCfg = env.ecfg.Config
	}
	ecfg, err := validateConfig(cfg, oldCfg)
	if err != nil {
		return err
	}

	storage, err := newStorage(ecfg)
	if err != nil {
		return err
	}
	env.ecfg = ecfg
	env.storage = storage

	// Connect and authenticate.
	env.gce = ecfg.newConnection()
	err = env.gce.connect(ecfg.auth())

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

// Storage returns storage specific to the environment.
func (env *environ) Storage() storage.Storage {
	return env.getSnapshot().storage
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
// match the provided instance IDs.
func (env *environ) Instances(ids []instance.Id) ([]instance.Instance, error) {
	instances, err := env.instances()
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Build the result, matching the provided instance IDs.
	var results []instance.Instance
	for _, id := range ids {
		inst := findInst(id, instances)
		if inst == nil {
			return results, errors.NotFoundf("GCE inst %q", id)
		}
		results = append(results, inst)
	}
	return results, nil
}

func rawInstances(env *environ) ([]*compute.Instance, error) {
	// rawInstances() only returns instances that are part of the
	// environment (instance IDs matches "juju-<env name>-machine-*").
	// This is important because otherwise juju will see they are not
	// tracked in state, assume they're stale/rogue, and shut them down.
	instances, err := env.gce.instances(env)
	if err != nil {
		return nil, errors.Trace(err)
	}
	// We further filter on the instance status.
	instances = filterInstances(instances, instStatuses...)
	return instances, nil
}

func (env *environ) instances() ([]instance.Instance, error) {
	env = env.getSnapshot()

	instances, err := rawInstances(env)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Turn *compute.Instance values into *environInstance values.
	var results []instance.Instance
	for _, inst := range instances {
		results = append(results, newInstance(inst, env))
	}
	return results, nil
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
