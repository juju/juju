// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs

import (
	"fmt"
	"io/ioutil"
	"strings"
	"time"

	"github.com/errgo/errgo"

	"launchpad.net/juju-core/cert"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/environs/configstore"
	"launchpad.net/juju-core/environs/storage"
	"launchpad.net/juju-core/errors"
)

// File named `VerificationFilename` in the storage will contain
// `verificationContent`.  This is also used to differentiate between
// Python Juju and juju-core environments, so change the content with
// care (and update CheckEnvironment below when you do that).
const (
	VerificationFilename = "bootstrap-verify"
	verificationContent  = "juju-core storage writing verified: ok\n"
)

var (
	VerifyStorageError error = fmt.Errorf(
		"provider storage is not writable")
	InvalidEnvironmentError = fmt.Errorf(
		"environment is not a juju-core environment")
)

// ConfigSource represents where some configuration data
// has come from.
// TODO(rog) remove this when we don't have to support
// old environments with no configstore info. See lp#1235217
type ConfigSource int

const (
	ConfigFromNowhere ConfigSource = iota
	ConfigFromInfo
	ConfigFromEnvirons
)

// ConfigForName returns the configuration for the environment with the
// given name from the default environments file. If the name is blank,
// the default environment will be used. If the configuration is not
// found, an errors.NotFoundError is returned.
// If the given store contains an entry for the environment
// and it has associated bootstrap config, that configuration
// will be returned.
// ConfigForName also returns where the configuration
// was sourced from (this is also valid even when there
// is an error.
func ConfigForName(name string, store configstore.Storage) (*config.Config, ConfigSource, error) {
	envs, err := ReadEnvirons("")
	if err != nil {
		return nil, ConfigFromNowhere, err
	}
	if name == "" {
		name = envs.Default
	}
	// TODO(rog) 2013-10-04 https://bugs.launchpad.net/juju-core/+bug/1235217
	// Don't fall back to reading from environments.yaml
	// when we can be sure that everyone has a
	// .jenv file for their currently bootstrapped environments.
	if name != "" {
		info, err := store.ReadInfo(name)
		if err == nil {
			if len(info.BootstrapConfig()) == 0 {
				return nil, ConfigFromNowhere, fmt.Errorf("environment has no bootstrap configuration data")
			}
			logger.Debugf("ConfigForName found bootstrap config %#v", info.BootstrapConfig())
			cfg, err := config.New(config.NoDefaults, info.BootstrapConfig())
			return cfg, ConfigFromInfo, err
		}
		if err != nil && !errors.IsNotFoundError(err) {
			return nil, ConfigFromInfo, fmt.Errorf("cannot read environment info for %q: %v", name, err)
		}
	}
	cfg, err := envs.Config(name)
	return cfg, ConfigFromEnvirons, err
}

// NewFromName opens the environment with the given
// name from the default environments file. If the
// name is blank, the default environment will be used.
// If the given store contains an entry for the environment
// and it has associated bootstrap config, that configuration
// will be returned.
func NewFromName(name string, store configstore.Storage) (Environ, error) {
	// If we get an error when reading from a legacy
	// environments.yaml entry, we pretend it didn't exist
	// because the error is likely to be because
	// configuration attributes don't exist which
	// will be filled in by Prepare.
	cfg, source, err := ConfigForName(name, store)
	if err != nil && source == ConfigFromEnvirons {
		err = ErrNotBootstrapped
	}
	if err != nil {
		return nil, err
	}

	env, err := New(cfg)
	if err != nil && source == ConfigFromEnvirons {
		err = ErrNotBootstrapped
	}
	return env, err
}

// PrepareFromName is the same as NewFromName except
// that the environment is is prepared as well as opened,
// and environment information is created using the
// given store. If the environment is already prepared,
// it behaves like NewFromName.
func PrepareFromName(name string, ctx BootstrapContext, store configstore.Storage) (Environ, error) {
	cfg, _, err := ConfigForName(name, store)
	if err != nil {
		return nil, err
	}
	return Prepare(cfg, ctx, store)
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
func Prepare(cfg *config.Config, ctx BootstrapContext, store configstore.Storage) (Environ, error) {
	p, err := Provider(cfg.Type())
	if err != nil {
		return nil, err
	}
	info, err := store.CreateInfo(cfg.Name())
	if err == configstore.ErrEnvironInfoAlreadyExists {
		logger.Infof("environment info already exists; using New not Prepare")
		info, err := store.ReadInfo(cfg.Name())
		if err != nil {
			return nil, fmt.Errorf("error reading environment info %q: %v", cfg.Name(), err)
		}
		if !info.Initialized() {
			return nil, fmt.Errorf("found uninitialized environment info for %q; environment preparation probably in progress or interrupted", cfg.Name())
		}
		if len(info.BootstrapConfig()) == 0 {
			return nil, fmt.Errorf("found environment info but no bootstrap config")
		}
		cfg, err = config.New(config.NoDefaults, info.BootstrapConfig())
		if err != nil {
			return nil, fmt.Errorf("cannot parse bootstrap config: %v", err)
		}
		return New(cfg)
	}
	if err != nil {
		return nil, fmt.Errorf("cannot create new info for environment %q: %v", cfg.Name(), err)
	}
	env, err := prepare(ctx, cfg, info, p)
	if err != nil {
		if err := info.Destroy(); err != nil {
			logger.Warningf("cannot destroy newly created environment info: %v", err)
		}
		return nil, err
	}
	info.SetBootstrapConfig(env.Config().AllAttrs())
	if err := info.Write(); err != nil {
		return nil, fmt.Errorf("cannot create environment info %q: %v", env.Config().Name(), err)
	}
	return env, nil
}

func prepare(ctx BootstrapContext, cfg *config.Config, info configstore.EnvironInfo, p EnvironProvider) (Environ, error) {
	cfg, err := ensureAdminSecret(cfg)
	if err != nil {
		return nil, fmt.Errorf("cannot generate admin-secret: %v", err)
	}
	cfg, err = ensureCertificate(cfg)
	if err != nil {
		return nil, fmt.Errorf("cannot ensure CA certificate: %v", err)
	}
	return p.Prepare(ctx, cfg)
}

// ensureAdminSecret returns a config with a non-empty admin-secret.
func ensureAdminSecret(cfg *config.Config) (*config.Config, error) {
	if cfg.AdminSecret() != "" {
		return cfg, nil
	}
	return cfg.Apply(map[string]interface{}{
		"admin-secret": randomKey(),
	})
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
	return cfg.Apply(map[string]interface{}{
		"ca-cert":        string(caCert),
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
	return DestroyInfo(name, store)
}

// DestroyInfo destroys the configuration data for the named
// environment from the given store.
func DestroyInfo(envName string, store configstore.Storage) error {
	info, err := store.ReadInfo(envName)
	if err != nil {
		if errors.IsNotFoundError(err) {
			return nil
		}
		return err
	}
	if err := info.Destroy(); err != nil {
		return errgo.Annotate(err, "cannot destroy environment configuration information")
	}
	return nil
}

// VerifyStorage writes the bootstrap init file to the storage to indicate
// that the storage is writable.
func VerifyStorage(stor storage.Storage) error {
	reader := strings.NewReader(verificationContent)
	err := stor.Put(VerificationFilename, reader,
		int64(len(verificationContent)))
	if err != nil {
		logger.Warningf("failed to write bootstrap-verify file: %v", err)
		return VerifyStorageError
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
	reader, err := storage.Get(stor, VerificationFilename)
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
