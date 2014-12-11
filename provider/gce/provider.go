// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
)

var logger = loggo.GetLogger("juju.provider.gce")

type environProvider struct{}

var providerInstance = environProvider{}
var _ environs.EnvironProvider = providerInstance

func init() {
	// This will only happen in binaries that actually import this provider
	// somewhere. To enable a provider, import it in the "providers/all"
	// package; please do *not* import individual providers anywhere else,
	// except in direct tests for that provider.
	environs.RegisterProvider("gce", providerInstance)
}

var errNotImplemented = errors.NotImplementedf("in gce provider")

func (environProvider) Open(cfg *config.Config) (environs.Environ, error) {
	// You should probably not change this method; prefer to cause SetConfig
	// to completely configure an environment, regardless of the initial state.
	env := &environ{name: cfg.Name()}
	if err := env.SetConfig(cfg); err != nil {
		return nil, err
	}
	return env, nil
}

func (environProvider) Prepare(ctx environs.BootstrapContext, cfg *config.Config) (environs.Environ, error) {
	return providerInstance.Open(cfg)
}

func (environProvider) Validate(cfg, old *config.Config) (valid *config.Config, err error) {
	// You should almost certainly not change this method; if you need to change
	// how configs are validated, you should edit validateConfig itself, to ensure
	// that your checks are always applied.
	newEcfg, err := validateConfig(cfg, nil)
	if err != nil {
		return nil, fmt.Errorf("invalid config: %v", err)
	}
	if old != nil {
		oldEcfg, err := validateConfig(old, nil)
		if err != nil {
			return nil, fmt.Errorf("invalid base config: %v", err)
		}
		if newEcfg, err = validateConfig(cfg, oldEcfg.Config); err != nil {
			return nil, fmt.Errorf("invalid config change: %v", err)
		}
	}
	return cfg.Apply(newEcfg.attrs)
}

func (environProvider) SecretAttrs(cfg *config.Config) (map[string]string, error) {
	ecfg, err := validateConfig(cfg, nil)
	if err != nil {
		return nil, err
	}
	secretAttrs := map[string]string{}
	secretAttrs[cfgPrivateKey] = ecfg.privateKey()
	return secretAttrs, nil
}

func (environProvider) BoilerplateConfig() string {
	// boilerplateConfig is kept in config.go, in the hope that people editing
	// config will keep it up to date.
	return boilerplateConfig
}
