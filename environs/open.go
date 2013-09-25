// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs

import (
	"fmt"
	"io/ioutil"

	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/environs/configstore"
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
// that the environment is is prepared as well as opened,
// and environment information is created using the
// given store.
func PrepareFromName(name string, store configstore.Storage) (Environ, error) {
	cfg, err := ConfigForName(name)
	if err != nil {
		return nil, err
	}
	return Prepare(cfg, store)
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
// If the environment is already prepared, it behaves like New.
func Prepare(config *config.Config, store configstore.Storage) (Environ, error) {
	p, err := Provider(config.Type())
	if err != nil {
		return nil, err
	}
	info, err := store.CreateInfo(config.Name())
	if err == configstore.ErrEnvironInfoAlreadyExists {
		logger.Infof("environment info already exists; using New not Prepare")
		info, err := store.ReadInfo(config.Name())
		if err != nil {
			return nil, fmt.Errorf("error reading environment info %q: %v", err)
		}
		if !info.Initialized() {
			return nil, fmt.Errorf("found uninitialized environment info for %q; environment preparation probably in progress or interrupted", config.Name())
		}
		return New(config)
	}
	if err != nil {
		return nil, fmt.Errorf("cannot create new info for environment %q: %v", config.Name(), err)
	}
	env, err := p.Prepare(config)
	if err != nil {
		if err := info.Destroy(); err != nil {
			logger.Warningf("cannot destroy newly created environment info: %v", err)
		}
		return nil, err
	}
//	oldAttrs := config.AllAttrs()
//	newAttrs := make(map[string]interface{})
//	for name, newAttr := range env.Config().AllAttrs() {
//		if _, ok := oldAttrs[name]; !ok {
//			newAttrs[name] = newAttr
//		}
//	}
	info.SetExtraConfig(env.Config().AllAttrs())
	if err := info.Write(); err != nil {
		return nil, fmt.Errorf("cannot create environment info %q: %v", err)
	}
	return env, nil
}

// Destroy destroys the environment and, if successful,
// its associated configuration data from the given store.
func Destroy(env Environ, store configstore.Storage) error {
	name := env.Name()
	if err := env.Destroy(); err != nil {
		return err
	}
	info, err := store.ReadInfo(name)
	if err != nil {
		if errors.IsNotFoundError(err) {
			return nil
		}
		return err
	}
	if err := info.Destroy(); err != nil {
		return fmt.Errorf("cannot destroy environment configuration information: %v", err)
	}
	return nil
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
	reader, err := storage.Get(stor, verificationFilename)
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
