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
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/provider/common"
)

// This file contains the core of the Joyent Environ implementation.

type joyentEnviron struct {
	name    string
	cloud   environs.CloudSpec
	compute *joyentCompute

	lock sync.Mutex // protects ecfg
	ecfg *environConfig
}

// newEnviron create a new Joyent environ instance from config.
func newEnviron(cloud environs.CloudSpec, cfg *config.Config) (*joyentEnviron, error) {
	env := &joyentEnviron{
		name:  cfg.Name(),
		cloud: cloud,
	}
	if err := env.SetConfig(cfg); err != nil {
		return nil, err
	}
	var err error
	env.compute, err = newCompute(cloud)
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

// Create is part of the Environ interface.
func (env *joyentEnviron) Create(environs.CreateParams) error {
	if err := verifyCredentials(env); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (env *joyentEnviron) PrepareForBootstrap(ctx environs.BootstrapContext) error {
	if ctx.ShouldVerifyCredentials() {
		if err := verifyCredentials(env); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

func (env *joyentEnviron) Bootstrap(ctx environs.BootstrapContext, args environs.BootstrapParams) (*environs.BootstrapResult, error) {
	return common.Bootstrap(ctx, env, args)
}

func (env *joyentEnviron) ControllerInstances(controllerUUID string) ([]instance.Id, error) {
	instanceIds := []instance.Id{}

	filter := cloudapi.NewFilter()
	filter.Set(tagKey("group"), "juju")
	filter.Set(tagKey(tags.JujuModel), controllerUUID)
	filter.Set(tagKey(tags.JujuIsController), "true")

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

// DestroyController implements the Environ interface.
func (env *joyentEnviron) DestroyController(controllerUUID string) error {
	// TODO(wallyworld): destroy hosted model resources
	return env.Destroy()
}

func (env *joyentEnviron) Ecfg() *environConfig {
	env.lock.Lock()
	defer env.lock.Unlock()
	return env.ecfg
}

// MetadataLookupParams returns parameters which are used to query simplestreams metadata.
func (env *joyentEnviron) MetadataLookupParams(region string) (*simplestreams.MetadataLookupParams, error) {
	if region == "" {
		region = env.cloud.Region
	}
	return &simplestreams.MetadataLookupParams{
		Series:   config.PreferredSeries(env.Ecfg()),
		Region:   region,
		Endpoint: env.cloud.Endpoint,
	}, nil
}

// Region is specified in the HasRegion interface.
func (env *joyentEnviron) Region() (simplestreams.CloudSpec, error) {
	return simplestreams.CloudSpec{
		Region:   env.cloud.Region,
		Endpoint: env.cloud.Endpoint,
	}, nil
}
