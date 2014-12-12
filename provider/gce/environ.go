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
	auth := gceAuth{
		clientID:    ecfg.attrs[cfgClientId].(string),
		clientEmail: ecfg.attrs[cfgClientEmail].(string),
		privateKey:  []byte(ecfg.attrs[cfgPrivateKey].(string)),
	}
	env.gce = &gceConnection{
		// TODO(ericsnow) Pull these from ecfg.
		region:    "",
		projectID: "",
	}
	err = env.gce.connect(auth)
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

func (env *environ) Instances(ids []instance.Id) ([]instance.Instance, error) {
	// Please note that this must *not* return instances that have not been
	// allocated as part of this environment -- if it does, juju will see they
	// are not tracked in state, assume they're stale/rogue, and shut them down.
	// This advice applies even if an instance id passed in corresponds to a
	// real instance that's not part of the environment -- the Environ should
	// treat that no differently to a request for one that does not exist.
	_ = env.getSnapshot()
	return nil, errNotImplemented
}

func (env *environ) StateServerInstances() ([]instance.Id, error) {
	return nil, errNotImplemented
}

func (env *environ) SupportedArchitectures() ([]string, error) {
	return arch.AllSupportedArches, nil
}

// SupportsUnitAssignment returns an error which, if non-nil, indicates
// that the environment does not support unit placement. If the environment
// does not support unit placement, then machines may not be created
// without units, and units cannot be placed explcitly.
func (env *environ) SupportsUnitPlacement() error {
	return errNotImplemented
}
