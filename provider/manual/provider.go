// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual

import (
	"fmt"

	"github.com/juju/errors"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/manual"
)

type manualProvider struct {
	environProviderCredentials
}

// Verify that we conform to the interface.
var _ environs.EnvironProvider = (*manualProvider)(nil)

var initUbuntuUser = manual.InitUbuntuUser

func ensureBootstrapUbuntuUser(ctx environs.BootstrapContext, host string, cfg *environConfig) error {
	err := initUbuntuUser(host, cfg.bootstrapUser(), cfg.AuthorizedKeys(), ctx.GetStdin(), ctx.GetStdout())
	if err != nil {
		logger.Errorf("initializing ubuntu user: %v", err)
		return err
	}
	logger.Infof("initialized ubuntu user")
	return nil
}

// RestrictedConfigAttributes is specified in the EnvironProvider interface.
func (p manualProvider) RestrictedConfigAttributes() []string {
	return []string{"bootstrap-user"}
}

// DetectRegions is specified in the environs.CloudRegionDetector interface.
func (p manualProvider) DetectRegions() ([]cloud.Region, error) {
	return nil, errors.NotFoundf("regions")
}

// PrepareConfig is specified in the EnvironProvider interface.
func (p manualProvider) PrepareConfig(args environs.PrepareConfigParams) (*config.Config, error) {
	if args.Cloud.Endpoint == "" {
		return nil, errors.Errorf(
			"missing address of host to bootstrap: " +
				`please specify "juju bootstrap manual/<host>"`,
		)
	}
	envConfig, err := p.validate(args.Config, nil)
	if err != nil {
		return nil, err
	}
	return args.Config.Apply(envConfig.attrs)
}

func (p manualProvider) Open(args environs.OpenParams) (environs.Environ, error) {
	if err := validateCloudSpec(args.Cloud); err != nil {
		return nil, errors.Trace(err)
	}
	_, err := p.validate(args.Config, nil)
	if err != nil {
		return nil, err
	}
	// validate adds missing manual-specific config attributes
	// with their defaults in the result; we don't wnat that in
	// Open.
	envConfig := newModelConfig(args.Config, args.Config.UnknownAttrs())
	return p.open(args.Cloud.Endpoint, envConfig)
}

func validateCloudSpec(spec environs.CloudSpec) error {
	if spec.Endpoint == "" {
		return errors.Errorf(
			"missing address of host to bootstrap: " +
				`please specify "juju bootstrap manual/<host>"`,
		)
	}
	return nil
}

func (p manualProvider) open(host string, cfg *environConfig) (environs.Environ, error) {
	env := &manualEnviron{host: host, cfg: cfg}
	// Need to call SetConfig to initialise storage.
	if err := env.SetConfig(cfg.Config); err != nil {
		return nil, err
	}
	return env, nil
}

func checkImmutableString(cfg, old *environConfig, key string) error {
	if old.attrs[key] != cfg.attrs[key] {
		return fmt.Errorf("cannot change %s from %q to %q", key, old.attrs[key], cfg.attrs[key])
	}
	return nil
}

func (p manualProvider) validate(cfg, old *config.Config) (*environConfig, error) {
	// Check for valid changes for the base config values.
	if err := config.Validate(cfg, old); err != nil {
		return nil, err
	}
	validated, err := cfg.ValidateUnknownAttrs(configFields, configDefaults)
	if err != nil {
		return nil, err
	}
	envConfig := newModelConfig(cfg, validated)
	// Check various immutable attributes.
	if old != nil {
		oldEnvConfig, err := p.validate(old, nil)
		if err != nil {
			return nil, err
		}
		for _, key := range [...]string{
			"bootstrap-user",
		} {
			if err = checkImmutableString(envConfig, oldEnvConfig, key); err != nil {
				return nil, err
			}
		}
	}

	// If the user hasn't already specified a value, set it to the
	// given value.
	defineIfNot := func(keyName string, value interface{}) {
		if _, defined := cfg.AllAttrs()[keyName]; !defined {
			logger.Infof("%s was not defined. Defaulting to %v.", keyName, value)
			envConfig.attrs[keyName] = value
		}
	}

	// If the user hasn't specified a value, refresh the
	// available updates, but don't upgrade.
	defineIfNot("enable-os-refresh-update", true)
	defineIfNot("enable-os-upgrade", false)

	return envConfig, nil
}

func (p manualProvider) Validate(cfg, old *config.Config) (valid *config.Config, err error) {
	envConfig, err := p.validate(cfg, old)
	if err != nil {
		return nil, err
	}
	return cfg.Apply(envConfig.attrs)
}

func (p manualProvider) SecretAttrs(cfg *config.Config) (map[string]string, error) {
	attrs := make(map[string]string)
	return attrs, nil
}
