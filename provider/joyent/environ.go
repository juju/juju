// Copyright 2013 Joyent Inc.
// Licensed under the AGPLv3, see LICENCE file for details.

package joyent

import (
	"fmt"
	"strings"
	"sync"

	"github.com/joyent/gosdc/cloudapi"
	"github.com/juju/errors"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/provider/common"
)

// This file contains the core of the Joyent Environ implementation.

type joyentEnviron struct {
	common.SupportsUnitPlacementPolicy

	name string

	compute *joyentCompute

	// supportedArchitectures caches the architectures
	// for which images can be instantiated.
	archLock               sync.Mutex
	supportedArchitectures []string

	lock sync.Mutex // protects ecfg
	ecfg *environConfig
}

// newEnviron create a new Joyent environ instance from config.
func newEnviron(cfg *config.Config) (*joyentEnviron, error) {
	env := new(joyentEnviron)
	if err := env.SetConfig(cfg); err != nil {
		return nil, err
	}
	env.name = cfg.Name()
	var err error
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

func (env *joyentEnviron) Config() *config.Config {
	return env.Ecfg().Config
}

func (env *joyentEnviron) Bootstrap(ctx environs.BootstrapContext, args environs.BootstrapParams) (*environs.BootstrapResult, error) {
	return common.Bootstrap(ctx, env, args)
}

func (env *joyentEnviron) ControllerInstances() ([]instance.Id, error) {
	instanceIds := []instance.Id{}

	filter := cloudapi.NewFilter()
	filter.Set(tagKey("group"), "juju")
	filter.Set(tagKey("model"), env.Config().Name())
	filter.Set(tagKey(tags.JujuModel), env.Config().UUID())
	filter.Set(tagKey(tags.JujuController), "true")

	machines, err := env.compute.cloudapi.ListMachines(filter)
	if err != nil || len(machines) == 0 {
		return nil, environs.ErrNotBootstrapped
	}

	for _, m := range machines {
		if strings.EqualFold(m.State, "provisioning") || strings.EqualFold(m.State, "running") {
			copy := m
			ji := &joyentInstance{machine: &copy, env: env}
			instanceIds = append(instanceIds, ji.Id())
		}
	}

	return instanceIds, nil
}

func (env *joyentEnviron) Destroy() error {
	return errors.Trace(common.Destroy(env))
}

func (env *joyentEnviron) Ecfg() *environConfig {
	env.lock.Lock()
	defer env.lock.Unlock()
	return env.ecfg
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
