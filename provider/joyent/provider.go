// Copyright 2013 Joyent Inc.
// Licensed under the AGPLv3, see LICENCE file for details.

package joyent

import (
	"errors"
	"fmt"

	"launchpad.net/loggo"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
)

var logger = loggo.GetLogger("juju.provider.joyent")

type environProvider struct{}

var providerInstance = environProvider{}
var _ environs.EnvironProvider = providerInstance

func init() {
	// This will only happen in binaries that actually import this provider
	// somewhere. To enable a provider, import it in the "providers/all"
	// package; please do *not* import individual providers anywhere else,
	// except for tests for that provider.
	environs.RegisterProvider("joyent", providerInstance)
}

var errNotImplemented = errors.New("not implemented in Joyent provider")

func (environProvider) Prepare(cfg *config.Config) (environs.Environ, error) {
	// This method may be called with an incomplete cfg. It should make every
	// reasonable effort to create a valid configuration based on the supplied,
	// and open the resulting environment.
	// You should implement this method to the best of your ability before
	// expecting non-developers to use your provider, but it shouldn't be your
	// first priority.
	preparedCfg, err := prepareConfig(cfg)
	if err != nil {
		return nil, err
	}
	return providerInstance.Open(preparedCfg)
}

func (environProvider) Open(cfg *config.Config) (environs.Environ, error) {
	env := &environ{name: cfg.Name()}
	if err := env.SetConfig(cfg); err != nil {
		return nil, err
	}
	return env, nil
}

func (environProvider) Validate(cfg, old *config.Config) (valid *config.Config, err error) {
	// You should almost certainly not change this method; if you need to change
	// how configs are validated, you should edit validateConfig itself, to ensure
	// that your checks are always applied.
	newEcfg, err := validateConfig(cfg, nil)
	if err != nil {
		return nil, fmt.Errorf("invalid Joyent provider config: %v", err)
	}
	if old != nil {
		oldEcfg, err := validateConfig(old, nil)
		if err != nil {
			return nil, fmt.Errorf("original Joyent provider config is invalid: %v", err)
		}
		if newEcfg, err = validateConfig(cfg, oldEcfg); err != nil {
			return nil, fmt.Errorf("invalid Joyent provider config change: %v", err)
		}
	}
	return newEcfg.Config, nil
}

func (environProvider) SecretAttrs(cfg *config.Config) (map[string]string, error) {
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
				return nil, fmt.Errorf(
					"secret %q field must have a string value; got %v",
					field, value,
				)
			}
		}
	}
	return secretAttrs, nil
}

func (environProvider) BoilerplateConfig() string {
	return boilerplateConfig

}
func (environProvider) PublicAddress() (string, error) {
	// Don't bother implementing this method until you're ready to deploy units.
	// You probably won't need to by that stage; it's due for retirement. If it
	// turns out that you do need to, remember that this method will *only* be
	// called in code running on an instance in an environment using this
	// provider; and it needs to return the address of *that* instance.
	return "", errNotImplemented
}

func (environProvider) PrivateAddress() (string, error) {
	// Don't bother implementing this method until you're ready to deploy units.
	// You probably won't need to by that stage; it's due for retirement. If it
	// turns out that you do need to, remember that this method will *only* be
	// called in code running on an instance in an environment using this
	// provider; and it needs to return the address of *that* instance.
	return "", errNotImplemented
}
