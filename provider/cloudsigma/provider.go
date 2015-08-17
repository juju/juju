// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Juju provider for CloudSigma

package cloudsigma

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/storage/provider/registry"
)

var logger = loggo.GetLogger("juju.provider.cloudsigma")

const (
	providerType = "cloudsigma"
)

func getImageSource(env environs.Environ) (simplestreams.DataSource, error) {
	e, ok := env.(*environ)
	if !ok {
		return nil, errors.NotSupportedf("non-cloudsigma environment")
	}
	return simplestreams.NewURLDataSource("cloud images", fmt.Sprintf(CloudsigmaCloudImagesURLTemplate, e.ecfg.region()), utils.VerifySSLHostnames), nil
}

type environProvider struct{}

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
	registry.RegisterEnvironStorageProviders(providerType)
}

// Boilerplate returns a default configuration for the environment in yaml format.
// The text should be a key followed by some number of attributes:
//    `environName:
//        type: environTypeName
//        attr1: val1
//    `
// The text is used as a template (see the template package) with one extra template
// function available, rand, which expands to a random hexadecimal string when invoked.
func (environProvider) BoilerplateConfig() string {
	return boilerplateConfig
}

// Open opens the environment and returns it.
// The configuration must have come from a previously
// prepared environment.
func (environProvider) Open(cfg *config.Config) (environs.Environ, error) {
	logger.Infof("opening environment %q", cfg.Name())

	cfg, err := prepareConfig(cfg)
	if err != nil {
		return nil, err
	}

	env := &environ{name: cfg.Name()}
	if err := env.SetConfig(cfg); err != nil {
		return nil, err
	}

	return env, nil
}

// RestrictedConfigAttributes are provider specific attributes stored in
// the config that really cannot or should not be changed across
// environments running inside a single juju server.
func (environProvider) RestrictedConfigAttributes() []string {
	return []string{"region"}
}

// PrepareForCreateEnvironment prepares an environment for creation. Any
// additional configuration attributes are added to the config passed in
// and returned.  This allows providers to add additional required config
// for new environments that may be created in an existing juju server.
func (environProvider) PrepareForCreateEnvironment(cfg *config.Config) (*config.Config, error) {
	// Not even sure if this will ever make sense.
	return nil, errors.NotImplementedf("PrepareForCreateEnvironment")
}

// Prepare prepares an environment for use. Any additional
// configuration attributes in the returned environment should
// be saved to be used later. If the environment is already
// prepared, this call is equivalent to Open.
func (environProvider) PrepareForBootstrap(ctx environs.BootstrapContext, cfg *config.Config) (environs.Environ, error) {
	logger.Infof("preparing environment %q", cfg.Name())
	return providerInstance.Open(cfg)
}

// Validate ensures that config is a valid configuration for this
// provider, applying changes to it if necessary, and returns the
// validated configuration.
// If old is not nil, it holds the previous environment configuration
// for consideration when validating changes.
func (environProvider) Validate(cfg, old *config.Config) (*config.Config, error) {
	logger.Infof("validating environment %q", cfg.Name())

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

// SecretAttrs filters the supplied configuration returning only values
// which are considered sensitive. All of the values of these secret
// attributes need to be strings.
func (environProvider) SecretAttrs(cfg *config.Config) (map[string]string, error) {
	logger.Infof("filtering secret attributes for environment %q", cfg.Name())

	// If you keep configSecretFields up to date, this method should Just Work.
	ecfg, err := validateConfig(cfg, nil)
	if err != nil {
		return nil, err
	}
	secretAttrs := map[string]string{}
	for _, field := range configSecretFields {
		if value, ok := ecfg.attrs[field]; ok {
			if stringValue, ok := value.(string); ok {
				secretAttrs[field] = stringValue
			} else {
				// All your secret attributes must be strings at the moment. Sorry.
				// It's an expedient and hopefully temporary measure that helps us
				// plug a security hole in the API.
				return nil, errors.Errorf(
					"secret %q field must have a string value; got %v",
					field, value,
				)
			}
		}
	}

	return secretAttrs, nil
}
