// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual

import (
	"errors"
	"fmt"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/utils"
)

type manualProvider struct{}

func init() {
	p := manualProvider{}
	environs.RegisterProvider("manual", p, "null")
}

var errNoBootstrapHost = errors.New("bootstrap-host must be specified")

func (p manualProvider) Prepare(cfg *config.Config) (environs.Environ, error) {
	if _, ok := cfg.UnknownAttrs()["storage-auth-key"].(string); !ok {
		uuid, err := utils.NewUUID()
		if err != nil {
			return nil, err
		}
		cfg, err = cfg.Apply(map[string]interface{}{
			"storage-auth-key": uuid.String(),
		})
		if err != nil {
			return nil, err
		}
	}
	return p.Open(cfg)
}

func (p manualProvider) Open(cfg *config.Config) (environs.Environ, error) {
	envConfig, err := p.validate(cfg, nil)
	if err != nil {
		return nil, err
	}
	return p.open(envConfig)
}

func (p manualProvider) open(cfg *environConfig) (environs.Environ, error) {
	return &manualEnviron{cfg: cfg}, nil
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
			"storage-listen-ip",
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

func (p manualProvider) Validate(cfg, old *config.Config) (valid *config.Config, err error) {
	envConfig, err := p.validate(cfg, old)
	if err != nil {
		return nil, err
	}
	return cfg.Apply(envConfig.attrs)
}

func (_ manualProvider) BoilerplateConfig() string {
	return `
manual:
    type: manual
    # bootstrap-host holds the host name of the machine where the
    # bootstrap machine agent will be started.
    bootstrap-host: somehost.example.com
    
    # bootstrap-user specifies the user to authenticate as when
    # connecting to the bootstrap machine. If defaults to
    # the current user.
    # bootstrap-user: joebloggs
    
    # storage-listen-ip specifies the IP address that the
    # bootstrap machine's Juju storage server will listen
    # on. By default, storage will be served on all
    # network interfaces.
    # storage-listen-ip:
    
    # storage-port specifes the TCP port that the
    # bootstrap machine's Juju storage server will listen
    # on. It defaults to ` + fmt.Sprint(defaultStoragePort) + `
    # storage-port: ` + fmt.Sprint(defaultStoragePort) + `


`[1:]
}

func (p manualProvider) SecretAttrs(cfg *config.Config) (map[string]string, error) {
	envConfig, err := p.validate(cfg, nil)
	if err != nil {
		return nil, err
	}
	attrs := make(map[string]string)
	attrs["storage-auth-key"] = envConfig.storageAuthKey()
	return attrs, nil
}

func (_ manualProvider) PublicAddress() (string, error) {
	// TODO(axw) 2013-09-10 bug #1222643
	//
	// eth0 may not be the desired interface for traffic to route
	// through. We should somehow make this configurable, and
	// possibly also record the IP resolved during manual bootstrap.
	return utils.GetAddressForInterface("eth0")
}

func (p manualProvider) PrivateAddress() (string, error) {
	return p.PublicAddress()
}
