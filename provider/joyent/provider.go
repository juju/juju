// Copyright 2013 Joyent Inc.
// Licensed under the AGPLv3, see LICENCE file for details.

package joyent

import (
	"fmt"

	"github.com/joyent/gocommon/client"
	joyenterrors "github.com/joyent/gocommon/errors"
	"github.com/joyent/gosdc/cloudapi"
	"github.com/joyent/gosign/auth"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/simplestreams"
)

var logger = loggo.GetLogger("juju.provider.joyent")

type joyentProvider struct{}

var providerInstance = joyentProvider{}
var _ environs.EnvironProvider = providerInstance

var _ simplestreams.HasRegion = (*joyentEnviron)(nil)

var errNotImplemented = errors.New("not implemented in Joyent provider")

// RestrictedConfigAttributes is specified in the EnvironProvider interface.
func (joyentProvider) RestrictedConfigAttributes() []string {
	return nil
}

// PrepareForCreateEnvironment is specified in the EnvironProvider interface.
func (joyentProvider) PrepareForCreateEnvironment(cfg *config.Config) (*config.Config, error) {
	// Turn an incomplete config into a valid one, if possible.
	attrs := cfg.UnknownAttrs()

	if _, ok := attrs["control-dir"]; !ok {
		uuid, err := utils.NewUUID()
		if err != nil {
			return nil, errors.Trace(err)
		}
		attrs["control-dir"] = fmt.Sprintf("%x", uuid.Raw())
	}
	return cfg.Apply(attrs)
}

func (p joyentProvider) PrepareForBootstrap(ctx environs.BootstrapContext, cfg *config.Config) (environs.Environ, error) {
	cfg, err := p.PrepareForCreateEnvironment(cfg)
	if err != nil {
		return nil, errors.Trace(err)
	}
	e, err := p.Open(cfg)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if ctx.ShouldVerifyCredentials() {
		if err := verifyCredentials(e.(*joyentEnviron)); err != nil {
			return nil, errors.Trace(err)
		}
	}
	return e, nil
}

const unauthorisedMessage = `
Please ensure the Manta username and SSH access key you have
specified are correct. You can create or import an SSH key via
the "Account Summary" page in the Joyent console.`

// verifyCredentials issues a cheap, non-modifying request to Joyent to
// verify the configured credentials. If verification fails, a user-friendly
// error will be returned, and the original error will be logged at debug
// level.
var verifyCredentials = func(e *joyentEnviron) error {
	creds, err := credentials(e.Ecfg())
	if err != nil {
		return err
	}
	httpClient := client.NewClient(e.Ecfg().sdcUrl(), cloudapi.DefaultAPIVersion, creds, nil)
	apiClient := cloudapi.New(httpClient)
	_, err = apiClient.CountMachines()
	if err != nil {
		logger.Debugf("joyent request failed: %v", err)
		if joyenterrors.IsInvalidCredentials(err) || joyenterrors.IsNotAuthorized(err) {
			return errors.New("authentication failed.\n" + unauthorisedMessage)
		}
		return err
	}
	return nil
}

func credentials(cfg *environConfig) (*auth.Credentials, error) {
	authentication, err := auth.NewAuth(cfg.mantaUser(), cfg.privateKey(), cfg.algorithm())
	if err != nil {
		return nil, errors.Errorf("cannot create credentials: %v", err)
	}
	return &auth.Credentials{
		UserAuthentication: authentication,
		MantaKeyId:         cfg.mantaKeyId(),
		MantaEndpoint:      auth.Endpoint{URL: cfg.mantaUrl()},
		SdcKeyId:           cfg.sdcKeyId(),
		SdcEndpoint:        auth.Endpoint{URL: cfg.sdcUrl()},
	}, nil
}

func (joyentProvider) Open(cfg *config.Config) (environs.Environ, error) {
	env, err := newEnviron(cfg)
	if err != nil {
		return nil, err
	}
	return env, nil
}

func (joyentProvider) Validate(cfg, old *config.Config) (valid *config.Config, err error) {
	newEcfg, err := validateConfig(cfg, old)
	if err != nil {
		return nil, errors.Errorf("invalid Joyent provider config: %v", err)
	}
	return cfg.Apply(newEcfg.attrs)
}

func (joyentProvider) SecretAttrs(cfg *config.Config) (map[string]string, error) {
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

func (joyentProvider) BoilerplateConfig() string {
	return boilerplateConfig

}

func GetProviderInstance() environs.EnvironProvider {
	return providerInstance
}

// MetadataLookupParams returns parameters which are used to query image metadata to
// find matching image information.
func (p joyentProvider) MetadataLookupParams(region string) (*simplestreams.MetadataLookupParams, error) {
	if region == "" {
		return nil, errors.Errorf("region must be specified")
	}
	return &simplestreams.MetadataLookupParams{
		Region:        region,
		Architectures: []string{"amd64", "armhf"},
	}, nil
}

func (p joyentProvider) newConfig(cfg *config.Config) (*environConfig, error) {
	valid, err := p.Validate(cfg, nil)
	if err != nil {
		return nil, err
	}
	return &environConfig{valid, valid.UnknownAttrs()}, nil
}
