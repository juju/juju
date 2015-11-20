// Copyright 2013 Joyent Inc.
// Licensed under the AGPLv3, see LICENCE file for details.

package joyent

import (
	"fmt"
	"sync"

	"github.com/juju/errors"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/environs/storage"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/state"
)

// This file contains the core of the Joyent Environ implementation.

type joyentEnviron struct {
	common.SupportsUnitPlacementPolicy

	name string

	// supportedArchitectures caches the architectures
	// for which images can be instantiated.
	archLock               sync.Mutex
	supportedArchitectures []string

	// All mutating operations should lock the mutex. Non-mutating operations
	// should read all fields (other than name, which is immutable) from a
	// shallow copy taken with getSnapshot().
	// This advice is predicated on the goroutine-safety of the values of the
	// affected fields.
	lock    sync.Mutex
	ecfg    *environConfig
	storage storage.Storage
	compute *joyentCompute
}

var _ environs.Environ = (*joyentEnviron)(nil)
var _ state.Prechecker = (*joyentEnviron)(nil)

// newEnviron create a new Joyent environ instance from config.
func newEnviron(cfg *config.Config) (*joyentEnviron, error) {
	env := new(joyentEnviron)
	if err := env.SetConfig(cfg); err != nil {
		return nil, err
	}
	env.name = cfg.Name()
	var err error
	env.storage, err = newStorage(env.ecfg, "")
	if err != nil {
		return nil, err
	}
	env.compute, err = newCompute(env.ecfg)
	if err != nil {
		return nil, err
	}
	return env, nil
}

func (env *joyentEnviron) SetName(envName string) {
	env.name = envName
}

func (*joyentEnviron) Provider() environs.EnvironProvider {
	return providerInstance
}

// PrecheckInstance is defined on the state.Prechecker interface.
func (env *joyentEnviron) PrecheckInstance(series string, cons constraints.Value, placement string) error {
	if placement != "" {
		return fmt.Errorf("unknown placement directive: %s", placement)
	}
	if !cons.HasInstanceType() {
		return nil
	}
	// Constraint has an instance-type constraint so let's see if it is valid.
	instanceTypes, err := env.listInstanceTypes()
	if err != nil {
		return err
	}
	for _, instanceType := range instanceTypes {
		if instanceType.Name == *cons.InstanceType {
			return nil
		}
	}
	return fmt.Errorf("invalid Joyent instance %q specified", *cons.InstanceType)
}

// SupportedArchitectures is specified on the EnvironCapability interface.
func (env *joyentEnviron) SupportedArchitectures() ([]string, error) {
	env.archLock.Lock()
	defer env.archLock.Unlock()
	if env.supportedArchitectures != nil {
		return env.supportedArchitectures, nil
	}
	cfg := env.Ecfg()
	// Create a filter to get all images from our region and for the correct stream.
	cloudSpec := simplestreams.CloudSpec{
		Region:   cfg.Region(),
		Endpoint: cfg.SdcUrl(),
	}
	imageConstraint := imagemetadata.NewImageConstraint(simplestreams.LookupParams{
		CloudSpec: cloudSpec,
		Stream:    cfg.ImageStream(),
	})
	var err error
	env.supportedArchitectures, err = common.SupportedArchitectures(env, imageConstraint)
	return env.supportedArchitectures, err
}

func (env *joyentEnviron) SetConfig(cfg *config.Config) error {
	env.lock.Lock()
	defer env.lock.Unlock()
	ecfg, err := providerInstance.newConfig(cfg)
	if err != nil {
		return err
	}
	env.ecfg = ecfg
	return nil
}

func (env *joyentEnviron) getSnapshot() *joyentEnviron {
	env.lock.Lock()
	clone := *env
	env.lock.Unlock()
	clone.lock = sync.Mutex{}
	return &clone
}

func (env *joyentEnviron) Config() *config.Config {
	return env.getSnapshot().ecfg.Config
}

func (env *joyentEnviron) Storage() storage.Storage {
	return env.getSnapshot().storage
}

func (env *joyentEnviron) Bootstrap(ctx environs.BootstrapContext, args environs.BootstrapParams) (*environs.BootstrapResult, error) {
	return common.Bootstrap(ctx, env, args)
}

func (env *joyentEnviron) StateServerInstances() ([]instance.Id, error) {
	return common.ProviderStateInstances(env, env.Storage())
}

func (env *joyentEnviron) Destroy() error {
	if err := common.Destroy(env); err != nil {
		return errors.Trace(err)
	}
	return env.Storage().RemoveAll()
}

func (env *joyentEnviron) Ecfg() *environConfig {
	return env.getSnapshot().ecfg
}

// MetadataLookupParams returns parameters which are used to query simplestreams metadata.
func (env *joyentEnviron) MetadataLookupParams(region string) (*simplestreams.MetadataLookupParams, error) {
	if region == "" {
		region = env.Ecfg().Region()
	}
	return &simplestreams.MetadataLookupParams{
		Series:        config.PreferredSeries(env.Ecfg()),
		Region:        region,
		Endpoint:      env.Ecfg().sdcUrl(),
		Architectures: []string{"amd64", "armhf"},
	}, nil
}

// Region is specified in the HasRegion interface.
func (env *joyentEnviron) Region() (simplestreams.CloudSpec, error) {
	return simplestreams.CloudSpec{
		Region:   env.Ecfg().Region(),
		Endpoint: env.Ecfg().sdcUrl(),
	}, nil
}
