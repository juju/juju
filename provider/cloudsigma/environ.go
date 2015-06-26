// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudsigma

import (
	"sync"

	"github.com/altoros/gosigma"
	"github.com/juju/errors"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/juju/arch"
	"github.com/juju/juju/provider/common"
)

const (
	CloudsigmaCloudImagesURLTemplate = "https://%v.cloudsigma.com/"
)

// This file contains the core of the Environ implementation.
type environ struct {
	common.SupportsUnitPlacementPolicy
	name string

	lock      sync.Mutex
	archMutex sync.Mutex

	ecfg                   *environConfig
	client                 *environClient
	supportedArchitectures []string
}

// Name returns the Environ's name.
func (env environ) Name() string {
	return env.name
}

// Provider returns the EnvironProvider that created this Environ.
func (environ) Provider() environs.EnvironProvider {
	return providerInstance
}

// SetConfig updates the Environ's configuration.
//
// Calls to SetConfig do not affect the configuration of values previously obtained
// from Storage.
func (env *environ) SetConfig(cfg *config.Config) error {
	env.lock.Lock()
	defer env.lock.Unlock()

	ecfg, err := validateConfig(cfg, env.ecfg)
	if err != nil {
		return errors.Trace(err)
	}

	if env.client == nil || env.ecfg == nil || env.ecfg.clientConfigChanged(ecfg) {
		client, err := newClient(ecfg)
		if err != nil {
			return errors.Trace(err)
		}

		env.client = client
	}

	env.ecfg = ecfg

	return nil
}

// Config returns the configuration data with which the Environ was created.
// Note that this is not necessarily current; the canonical location
// for the configuration data is stored in the state.
func (env *environ) Config() *config.Config {
	return env.ecfg.Config
}

// Bootstrap initializes the state for the environment, possibly
// starting one or more instances.  If the configuration's
// AdminSecret is non-empty, the administrator password on the
// newly bootstrapped state will be set to a hash of it (see
// utils.PasswordHash), When first connecting to the
// environment via the juju package, the password hash will be
// automatically replaced by the real password.
//
// The supplied constraints are used to choose the initial instance
// specification, and will be stored in the new environment's state.
//
// Bootstrap is responsible for selecting the appropriate tools,
// and setting the agent-version configuration attribute prior to
// bootstrapping the environment.
func (env *environ) Bootstrap(ctx environs.BootstrapContext, params environs.BootstrapParams) (string, string, environs.BootstrapFinalizer, error) {
	return common.Bootstrap(ctx, env, params)
}

func (e *environ) StateServerInstances() ([]instance.Id, error) {
	return e.client.getStateServerIds()
}

// Destroy shuts down all known machines and destroys the
// rest of the environment. Note that on some providers,
// very recently started instances may not be destroyed
// because they are not yet visible.
//
// When Destroy has been called, any Environ referring to the
// same remote environment may become invalid
func (env *environ) Destroy() error {
	// You can probably ignore this method; the common implementation should work.
	return common.Destroy(env)
}

// PrecheckInstance performs a preflight check on the specified
// series and constraints, ensuring that they are possibly valid for
// creating an instance in this environment.
//
// PrecheckInstance is best effort, and not guaranteed to eliminate
// all invalid parameters. If PrecheckInstance returns nil, it is not
// guaranteed that the constraints are valid; if a non-nil error is
// returned, then the constraints are definitely invalid.
func (env *environ) PrecheckInstance(series string, cons constraints.Value, placement string) error {
	return nil
}

// Region is specified in the HasRegion interface.
func (env *environ) Region() (simplestreams.CloudSpec, error) {
	return env.cloudSpec(env.ecfg.region())
}

func (env *environ) cloudSpec(region string) (simplestreams.CloudSpec, error) {
	endpoint := gosigma.ResolveEndpoint(region)
	return simplestreams.CloudSpec{
		Region:   region,
		Endpoint: endpoint,
	}, nil
}

func (env *environ) MetadataLookupParams(region string) (*simplestreams.MetadataLookupParams, error) {
	if region == "" {
		region = gosigma.DefaultRegion
	}

	cloudSpec, err := env.cloudSpec(region)
	if err != nil {
		return nil, err
	}

	return &simplestreams.MetadataLookupParams{
		Region:        cloudSpec.Region,
		Endpoint:      cloudSpec.Endpoint,
		Architectures: arch.AllSupportedArches,
		Series:        config.PreferredSeries(env.ecfg),
	}, nil
}
