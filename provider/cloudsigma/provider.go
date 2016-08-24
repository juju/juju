// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Juju provider for CloudSigma

package cloudsigma

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/simplestreams"
)

var logger = loggo.GetLogger("juju.provider.cloudsigma")

const (
	providerType = "cloudsigma"
)

func getImageSource(env environs.Environ) (simplestreams.DataSource, error) {
	e, ok := env.(*environ)
	if !ok {
		return nil, errors.NotSupportedf("non-cloudsigma model")
	}
	return simplestreams.NewURLDataSource(
		"cloud images",
		fmt.Sprintf(CloudsigmaCloudImagesURLTemplate, e.cloud.Region),
		utils.VerifySSLHostnames,
		simplestreams.SPECIFIC_CLOUD_DATA,
		false,
	), nil
}

type environProvider struct {
	environProviderCredentials
}

var providerInstance = environProvider{}

// check the provider implements environs.EnvironProvider interface
var _ environs.EnvironProvider = (*environProvider)(nil)

func init() {
	// This will only happen in binaries that actually import this provider
	// somewhere. To enable a provider, import it in the "providers/all"
	// package; please do *not* import individual providers anywhere else,
	// except in direct tests for that provider.
	environs.RegisterProvider("cloudsigma", providerInstance)
	environs.RegisterImageDataSourceFunc("cloud sigma image source", getImageSource)
}

// Open opens the environment and returns it.
// The configuration must have come from a previously
// prepared environment.
func (environProvider) Open(args environs.OpenParams) (environs.Environ, error) {
	logger.Infof("opening model %q", args.Config.Name())
	if err := validateCloudSpec(args.Cloud); err != nil {
		return nil, errors.Annotate(err, "validating cloud spec")
	}

	client, err := newClient(args.Cloud, args.Config.UUID())
	if err != nil {
		return nil, errors.Trace(err)
	}
	env := &environ{
		name:   args.Config.Name(),
		cloud:  args.Cloud,
		client: client,
	}
	if err := env.SetConfig(args.Config); err != nil {
		return nil, err
	}

	return env, nil
}

// PrepareConfig is defined by EnvironProvider.
func (environProvider) PrepareConfig(args environs.PrepareConfigParams) (*config.Config, error) {
	if err := validateCloudSpec(args.Cloud); err != nil {
		return nil, errors.Annotate(err, "validating cloud spec")
	}
	return args.Config, nil
}

// Validate ensures that config is a valid configuration for this
// provider, applying changes to it if necessary, and returns the
// validated configuration.
// If old is not nil, it holds the previous environment configuration
// for consideration when validating changes.
func (environProvider) Validate(cfg, old *config.Config) (*config.Config, error) {
	logger.Infof("validating model %q", cfg.Name())

	// You should almost certainly not change this method; if you need to change
	// how configs are validated, you should edit validateConfig itself, to ensure
	// that your checks are always applied.
	newEcfg, err := validateConfig(cfg, nil)
	if err != nil {
		return nil, errors.Errorf("invalid config: %v", err)
	}
	if old != nil {
		oldEcfg, err := validateConfig(old, nil)
		if err != nil {
			return nil, errors.Errorf("invalid base config: %v", err)
		}
		if newEcfg, err = validateConfig(cfg, oldEcfg); err != nil {
			return nil, errors.Errorf("invalid config change: %v", err)
		}
	}

	return newEcfg.Config, nil
}

func validateCloudSpec(spec environs.CloudSpec) error {
	if err := spec.Validate(); err != nil {
		return errors.Trace(err)
	}
	if spec.Credential == nil {
		return errors.NotValidf("missing credential")
	}
	if authType := spec.Credential.AuthType(); authType != cloud.UserPassAuthType {
		return errors.NotSupportedf("%q auth-type", authType)
	}
	return nil
}
