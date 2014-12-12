// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"sync"

	"github.com/juju/errors"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/storage"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/juju/arch"
	"github.com/juju/juju/provider/common"
)

type environ struct {
	common.SupportsUnitPlacementPolicy

	name string

	lock    sync.Mutex
	ecfg    *environConfig
	storage storage.Storage

	auth gceAuth
	gce  *gceConnection
}

//TODO (wwitzel3): Investigate simplestreams.HasRegion for this provider
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
	// TODO(ericsnow) Should env.ecfg be explicitly copied-by-value?
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

func (env *environ) Bootstrap(ctx environs.BootstrapContext, params environs.BootstrapParams) (arch, series string, _ environs.BootstrapFinalizer, _ error) {
	return common.Bootstrap(ctx, env, params)
}

func (env *environ) Destroy() error {
	return common.Destroy(env)
}

func (env *environ) ConstraintsValidator() (constraints.Validator, error) {
	return nil, errNotImplemented
}

func (env *environ) PrecheckInstance(series string, cons constraints.Value, placement string) error {
	return errNotImplemented
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

func (env *environ) instances() ([]instance.Instance, error) {
	env = env.getSnapshot()

	// instances() only returns instances that are part of the
	// environment (instance IDs matches "juju-<env name>-machine-*").
	// This is important because otherwise juju will see they are not
	// tracked in state, assume they're stale/rogue, and shut them down.
	instances, err := env.gce.instances(env)
	if err != nil {
		return nil, errors.Trace(err)
	}
	// We further filter on the instance status.
	instances = filterInstances(instances, instStatuses...)

	// Turn *compute.Instance values into *environInstance values.
	var results []instance.Instance
	for _, inst := range instances {
		results = append(results, newInstance(inst, env))
	}
	return results, nil
}

func (env *environ) StateServerInstances() ([]instance.Id, error) {
	return common.ProviderStateInstances(env, env.Storage())
}

func (env *environ) SupportedArchitectures() ([]string, error) {
	return arch.AllSupportedArches, nil
}
