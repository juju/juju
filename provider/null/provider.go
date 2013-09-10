// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package null

import (
    "errors"
	"fmt"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/utils"
)

type nullProvider struct{}

var errNoBootstrapHost = errors.New("bootstrap-host must be specified")

func (p nullProvider) Prepare(cfg *config.Config) (environs.Environ, error) {
	return p.Open(cfg)
}

func (p nullProvider) Open(cfg *config.Config) (environs.Environ, error) {
	envConfig, err := p.validate(cfg, nil)
	if err != nil {
		return nil, err
	}
	return &nullEnviron{cfg: envConfig}, nil
}

func checkImmutableString(cfg, old *environConfig, key string) error {
	if old.attrs[key] != "" && old.attrs[key] != cfg.attrs[key] {
		return fmt.Errorf("cannot change %s from %q to %q", key, old.attrs[key], cfg.attrs[key])
	}
	return nil
}

func (p nullProvider) validate(cfg, old *config.Config) (*environConfig, error) {
	// Check for valid changes for the base config values.
	if err := config.Validate(cfg, old); err != nil {
		return nil, err
	}
	validated, err := cfg.ValidateUnknownAttrs(configFields, configDefaults)
	if err != nil {
		return nil, err
	}
	envConfig := newEnvironConfig(cfg, validated)
	if envConfig.bootstrapHost() == "" {
		return nil, errNoBootstrapHost
	}
	// Check various immutable attributes.
	if old != nil {
		oldEnvConfig, err := p.validate(old, nil)
		if err != nil {
			return nil, err
		}
		for _, key := range [...]string{
			"bootstrap-user",
			"bootstrap-host",
			"storage-ip",
			"storage-dir",
		} {
			if err = checkImmutableString(envConfig, oldEnvConfig, key); err != nil {
				return nil, err
			}
		}
		oldPort, newPort := oldEnvConfig.storagePort(), envConfig.storagePort()
		if oldPort != newPort {
			return nil, fmt.Errorf("cannot change storage-port from %q to %q", oldPort, newPort)
		}
	}
	return envConfig, nil
}

func (p nullProvider) Validate(cfg, old *config.Config) (valid *config.Config, err error) {
	envConfig, err := p.validate(cfg, old)
	if err != nil {
		return nil, err
	}
	return cfg.Apply(envConfig.attrs)
}

func (_ nullProvider) BoilerplateConfig() string {
	return `
    "null":
        type: "null"
        admin-secret: {{rand}}
        # set bootstrap-host to the host where the bootstrap machine agent
        # should be provisioned.
        bootstrap-host:
        # bootstrap-user:
        # storage-ip:
        # storage-port: 8040
        # storage-dir: /var/lib/juju/storage
`
}

func (_ nullProvider) SecretAttrs(cfg *config.Config) (map[string]interface{}, error) {
	return make(map[string]interface{}), nil
}

func (_ nullProvider) PublicAddress() (string, error) {
	// TODO(axw) 2013-09-10 bug #1222643
	//
	// eth0 may not be the desired interface for traffic to route
	// through. We should somehow make this configurable, and
	// possibly also record the IP resolved during manual bootstrap.
	return utils.GetAddressForInterface("eth0")
}

func (p nullProvider) PrivateAddress() (string, error) {
	return p.PublicAddress()
}
