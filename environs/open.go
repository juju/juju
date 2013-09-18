// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs

import (
	"fmt"
	"io/ioutil"

	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/environs/storage"
	"launchpad.net/juju-core/errors"
)

var InvalidEnvironmentError = fmt.Errorf("environment is not a juju-core environment")

// ConfigForName returns the configuration for the environment with the
// given name from the default environments file. If the name is blank,
// the default environment will be used. If the configuration is not
// found, an errors.NotFoundError is returned.
func ConfigForName(name string) (*config.Config, error) {
	envs, err := ReadEnvirons("")
	if err != nil {
		return nil, err
	}
	return envs.Config(name)
}

// NewFromName opens the environment with the given
// name from the default environments file. If the
// name is blank, the default environment will be used.
func NewFromName(name string) (Environ, error) {
	cfg, err := ConfigForName(name)
	if err != nil {
		return nil, err
	}
	return New(cfg)
}

// PrepareFromName is the same as NewFromName except
// that the environment is is prepared as well as opened.
func PrepareFromName(name string) (Environ, error) {
	cfg, err := ConfigForName(name)
	if err != nil {
		return nil, err
	}
	return Prepare(cfg)
}

// NewFromAttrs returns a new environment based on the provided configuration
// attributes.
func NewFromAttrs(attrs map[string]interface{}) (Environ, error) {
	cfg, err := config.New(config.NoDefaults, attrs)
	if err != nil {
		return nil, err
	}
	return New(cfg)
}

// New returns a new environment based on the provided configuration.
func New(config *config.Config) (Environ, error) {
	p, err := Provider(config.Type())
	if err != nil {
		return nil, err
	}
	return p.Open(config)
}

// Prepare prepares a new environment based on the provided configuration.
func Prepare(config *config.Config) (Environ, error) {
	p, err := Provider(config.Type())
	if err != nil {
		return nil, err
	}
	return p.Prepare(config)
}

// CheckEnvironment checks if an environment has a bootstrap-verify
// that is written by juju-core commands (as compared to one being
// written by Python juju).
//
// If there is no bootstrap-verify file in the storage, it is still
// considered to be a Juju-core environment since early versions have
// not written it out.
//
// Returns InvalidEnvironmentError on failure, nil otherwise.
func CheckEnvironment(environ Environ) error {
	stor := environ.Storage()
	reader, err := storage.GetWithDefaultRetry(stor, verificationFilename)
	if errors.IsNotFoundError(err) {
		// When verification file does not exist, this is a juju-core
		// environment.
		return nil
	} else if err != nil {
		return err
	} else if content, err := ioutil.ReadAll(reader); err != nil {
		return err
	} else if string(content) != verificationContent {
		return InvalidEnvironmentError
	}
	return nil
}
