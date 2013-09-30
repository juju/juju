// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs

import (
	"fmt"
	"io/ioutil"
	"time"

	"launchpad.net/juju-core/cert"
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
// If the given store contains an entry for the environment
// and it has associated bootstrap config, that configuration
// will be returned.
func ConfigForName(name string, store configstore.Storage) (*config.Config, error) {
	envs, err := ReadEnvirons("")
	if err != nil {
		return nil, err
	}
	if name == "" {
		name = envs.Default
	}
	if name != "" {
		info, err := store.ReadInfo(name)
		if err == nil && len(info.BootstrapConfig()) > 0 {
			logger.Debugf("ConfigForName found bootstrap config %#v", info.BootstrapConfig())
			return config.New(config.NoDefaults, info.BootstrapConfig())
		}
		if err != nil && !errors.IsNotFoundError(err) {
			return nil, fmt.Errorf("cannot read environment info for %q: %v", name, err)
		}
		if err == nil {
			logger.Debugf("ConfigForName found info but no bootstrap config")
		}
	}
	return envs.Config(name)
}

// NewFromName opens the environment with the given
// name from the default environments file. If the
// name is blank, the default environment will be used.
// If the given store contains an entry for the environment
// and it has associated bootstrap config, that configuration
// will be returned.
func NewFromName(name string, store configstore.Storage) (Environ, error) {
	cfg, err := ConfigForName(name, store)
	if err != nil {
		return nil, err
	}
	return New(cfg)
}

// PrepareFromName is the same as NewFromName except
// that the environment is is prepared as well as opened,
// and environment information is created using the
// given store. If the environment is already prepared,
// it behaves like NewFromName.
func PrepareFromName(name string, store configstore.Storage) (Environ, error) {
	cfg, err := ConfigForName(name, store)
	if err != nil {
		return nil, err
	}
	return Prepare(cfg, store)
}

// NewFromAttrs returns a new environment based on the provided configuration
// attributes.
// TODO(rog) remove this function - it's almost always wrong to use it.
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
	config, err = ensureCertificate(config)
	if err != nil {
		return nil, fmt.Errorf("cannot ensure CA certificate: %v", err)
	}
	env, err := p.Prepare(config)
	if err != nil {
		if err := info.Destroy(); err != nil {
			logger.Warningf("cannot destroy newly created environment info: %v", err)
		}
		return nil, err
	}
	info.SetBootstrapConfig(env.Config().AllAttrs())
	if err := info.Write(); err != nil {
		return nil, fmt.Errorf("cannot create environment info %q: %v", err)
	}
	return env, nil
}

// ensureCertificate generates a new CA certificate and
// attaches it to the given environment configuration,
// unless the configuration already has one.
func ensureCertificate(cfg *config.Config) (*config.Config, error) {
	_, hasCACert := cfg.CACert()
	_, hasCAKey := cfg.CAPrivateKey()
	if hasCACert && hasCAKey {
		return cfg, nil
	}
	if hasCACert && !hasCAKey {
		return nil, fmt.Errorf("environment configuration with a certificate but no CA private key")
	}

	caCert, caKey, err := cert.NewCA(cfg.Name(), time.Now().UTC().AddDate(10, 0, 0))
	if err != nil {
		return nil, err
	}
	return cfg.Apply(map[string]interface{} {
		"ca-cert": string(caCert),
		"ca-private-key": string(caKey),
	})
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
