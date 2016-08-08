// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxd

import (
	"github.com/juju/errors"
	"gopkg.in/juju/environschema.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/provider/lxd/lxdnames"
)

type environProvider struct {
	environProviderCredentials
}

var providerInstance environProvider

// Open implements environs.EnvironProvider.
func (environProvider) Open(args environs.OpenParams) (environs.Environ, error) {
	// TODO(ericsnow) verify prerequisites (see provider/local/prereq.go)?
	// TODO(ericsnow) do something similar to correctLocalhostURLs()
	// (in provider/local/environprovider.go)?

	env, err := newEnviron(args.Config, newRawProvider)
	return env, errors.Trace(err)
}

// PrepareConfig implements environs.EnvironProvider.
func (p environProvider) PrepareConfig(args environs.PrepareConfigParams) (*config.Config, error) {
	return args.Config, nil
}

// RestrictedConfigAttributes is specified in the EnvironProvider interface.
func (environProvider) RestrictedConfigAttributes() []string {
	return []string{
		"remote-url",
		"client-cert",
		"client-key",
		"server-cert",
	}
}

// Validate implements environs.EnvironProvider.
func (environProvider) Validate(cfg, old *config.Config) (valid *config.Config, err error) {
	if old == nil {
		ecfg, err := newValidConfig(cfg, configDefaults)
		if err != nil {
			return nil, errors.Annotate(err, "invalid config")
		}
		return ecfg.Config, nil
	}

	// The defaults should be set already, so we pass nil.
	ecfg, err := newValidConfig(old, nil)
	if err != nil {
		return nil, errors.Annotate(err, "invalid base config")
	}

	if err := ecfg.update(cfg); err != nil {
		return nil, errors.Annotate(err, "invalid config change")
	}

	return ecfg.Config, nil
}

// SecretAttrs implements environs.EnvironProvider.
func (environProvider) SecretAttrs(cfg *config.Config) (map[string]string, error) {
	// The defaults should be set already, so we pass nil.
	ecfg, err := newValidConfig(cfg, nil)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return ecfg.secret(), nil
}

// DetectRegions implements environs.CloudRegionDetector.
func (environProvider) DetectRegions() ([]cloud.Region, error) {
	// For now we just return a hard-coded "localhost" region,
	// i.e. the local LXD daemon. We may later want to detect
	// locally-configured remotes.
	return []cloud.Region{{Name: lxdnames.DefaultRegion}}, nil
}

// Schema returns the configuration schema for an environment.
func (environProvider) Schema() environschema.Fields {
	fields, err := config.Schema(configSchema)
	if err != nil {
		panic(err)
	}
	return fields
}
