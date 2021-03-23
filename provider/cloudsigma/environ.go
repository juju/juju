// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudsigma

import (
	"sync"

	"github.com/altoros/gosigma"
	"github.com/juju/errors"
	"github.com/juju/version/v2"

	"github.com/juju/juju/core/instance"
	coreseries "github.com/juju/juju/core/series"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/provider/common"
)

const (
	CloudsigmaCloudImagesURLTemplate = "https://%v.cloudsigma.com/"
)

// This file contains the core of the Environ implementation.
type environ struct {
	name   string
	cloud  environscloudspec.CloudSpec
	client *environClient
	lock   sync.Mutex
	ecfg   *environConfig
}

// Name returns the Environ's name.
func (env *environ) Name() string {
	return env.name
}

// Provider returns the EnvironProvider that created this Environ.
func (*environ) Provider() environs.EnvironProvider {
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
	env.ecfg = ecfg

	return nil
}

// Config returns the configuration data with which the Environ was created.
// Note that this is not necessarily current; the canonical location
// for the configuration data is stored in the state.
func (env *environ) Config() *config.Config {
	return env.ecfg.Config
}

// PrepareForBootstrap is part of the Environ interface.
func (env *environ) PrepareForBootstrap(ctx environs.BootstrapContext, controllerName string) error {
	logger.Infof("preparing model %q", env.name)
	return nil
}

// Create is part of the Environ interface.
func (env *environ) Create(context.ProviderCallContext, environs.CreateParams) error {
	return nil
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
func (env *environ) Bootstrap(ctx environs.BootstrapContext, callCtx context.ProviderCallContext, params environs.BootstrapParams) (*environs.BootstrapResult, error) {
	return common.Bootstrap(ctx, env, callCtx, params)
}

// ControllerInstances is part of the Environ interface.
func (e *environ) ControllerInstances(ctx context.ProviderCallContext, controllerUUID string) ([]instance.Id, error) {
	return e.client.getControllerIds()
}

// AdoptResources is part of the Environ interface.
func (e *environ) AdoptResources(ctx context.ProviderCallContext, controllerUUID string, fromVersion version.Number) error {
	// This provider doesn't track instance -> controller.
	return nil
}

// Destroy shuts down all known machines and destroys the
// rest of the environment. Note that on some providers,
// very recently started instances may not be destroyed
// because they are not yet visible.
//
// When Destroy has been called, any Environ referring to the
// same remote environment may become invalid
func (env *environ) Destroy(ctx context.ProviderCallContext) error {
	// You can probably ignore this method; the common implementation should work.
	return common.Destroy(env, ctx)
}

// DestroyController implements the Environ interface.
func (env *environ) DestroyController(ctx context.ProviderCallContext, controllerUUID string) error {
	// TODO(wallyworld): destroy hosted model resources
	return env.Destroy(ctx)
}

// PrecheckInstance performs a preflight check on the specified
// series and constraints, ensuring that they are possibly valid for
// creating an instance in this environment.
//
// PrecheckInstance is best effort, and not guaranteed to eliminate
// all invalid parameters. If PrecheckInstance returns nil, it is not
// guaranteed that the constraints are valid; if a non-nil error is
// returned, then the constraints are definitely invalid.
func (env *environ) PrecheckInstance(context.ProviderCallContext, environs.PrecheckInstanceParams) error {
	return nil
}

// Region is specified in the HasRegion interface.
func (env *environ) Region() (simplestreams.CloudSpec, error) {
	return simplestreams.CloudSpec{
		Region:   env.cloud.Region,
		Endpoint: env.cloud.Endpoint,
	}, nil
}

func (env *environ) ImageMetadataLookupParams(region string) (*simplestreams.MetadataLookupParams, error) {
	env.lock.Lock()
	defer env.lock.Unlock()
	return &simplestreams.MetadataLookupParams{
		Region:   region,
		Endpoint: gosigma.ResolveEndpoint(region),
		Release:  config.PreferredSeries(env.ecfg),
	}, nil
}

func (env *environ) AgentMetadataLookupParams(region string) (*simplestreams.MetadataLookupParams, error) {
	env.lock.Lock()
	defer env.lock.Unlock()
	series := config.PreferredSeries(env.ecfg)
	hostOSType := coreseries.DefaultOSTypeNameFromSeries(series)
	return &simplestreams.MetadataLookupParams{
		Region:   region,
		Endpoint: gosigma.ResolveEndpoint(region),
		Release:  hostOSType,
	}, nil
}
