// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs

import (
	"fmt"
	"time"

	"github.com/juju/errors"
	"github.com/juju/utils"
	"github.com/juju/utils/featureflag"

	"github.com/juju/juju/cert"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/configstore"
	"github.com/juju/juju/feature"
)

var (
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

// EmptyConfig indicates the .jenv file is empty.
type EmptyConfig struct {
	error
}

// IsEmptyConfig reports whether err is a EmptyConfig.
func IsEmptyConfig(err error) bool {
	_, ok := err.(EmptyConfig)
	return ok
}

// ConfigForName returns the configuration for the environment with
// the given name from the default environments file. If the name is
// blank, the default environment will be used. If the configuration
// is not found, an errors.NotFoundError is returned. If the given
// store contains an entry for the environment and it has associated
// bootstrap config, that configuration will be returned.
// ConfigForName also returns where the configuration was sourced from
// (this is also valid even when there is an error.
func ConfigForName(name string, store configstore.Storage) (*config.Config, ConfigSource, error) {
	envs, err := ReadEnvirons("")
	if err != nil {
		return nil, ConfigFromNowhere, err
	}
	if name == "" {
		name = envs.Default
	}

	info, err := store.ReadInfo(name)
	if err == nil {
		if len(info.BootstrapConfig()) == 0 {
			return nil, ConfigFromNowhere, EmptyConfig{fmt.Errorf("environment has no bootstrap configuration data")}
		}
		logger.Debugf("ConfigForName found bootstrap config %#v", info.BootstrapConfig())
		cfg, err := config.New(config.NoDefaults, info.BootstrapConfig())
		return cfg, ConfigFromInfo, err
	} else if !errors.IsNotFound(err) {
		return nil, ConfigFromInfo, fmt.Errorf("cannot read environment info for %q: %v", name, err)
	}

	cfg, err := envs.Config(name)
	return cfg, ConfigFromEnvirons, err
}

// maybeNotBootstrapped takes an error and source, returned by
// ConfigForName and returns ErrNotBootstrapped if it looks like the
// environment is not bootstrapped, or err as-is otherwise.
func maybeNotBootstrapped(err error, source ConfigSource) error {
	if err != nil && source == ConfigFromEnvirons {
		return ErrNotBootstrapped
	}
	return err
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
	if err := maybeNotBootstrapped(err, source); err != nil {
		return nil, err
	}
	if err != nil {
		return nil, err
	}

	env, err := New(cfg)
	if err := maybeNotBootstrapped(err, source); err != nil {
		return nil, err
	}
	return env, err
}

// PrepareFromName is the same as NewFromName except
// that the environment is is prepared as well as opened,
// and environment information is created using the
// given store. If the environment is already prepared,
// it behaves like NewFromName.
var PrepareFromName = prepareFromNameProductionFunc

func prepareFromNameProductionFunc(name string, ctx BootstrapContext, store configstore.Storage) (Environ, error) {
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
		return nil, errors.Trace(err)
	}
	return New(cfg)
}

// New returns a new environment based on the provided configuration.
func New(config *config.Config) (Environ, error) {
	p, err := Provider(config.Type())
	if err != nil {
		return nil, errors.Trace(err)
	}
	return p.Open(config)
}

// Prepare prepares a new environment based on the provided configuration.
// If the environment is already prepared, it behaves like New.
func Prepare(cfg *config.Config, ctx BootstrapContext, store configstore.Storage) (Environ, error) {

	if p, err := Provider(cfg.Type()); err != nil {
		return nil, errors.Trace(err)
	} else if info, err := store.ReadInfo(cfg.Name()); errors.IsNotFound(errors.Cause(err)) {
		info = store.CreateInfo(cfg.Name())
		if env, err := prepare(ctx, cfg, info, p); err == nil {
			return env, decorateAndWriteInfo(info, env.Config())
		} else {
			if err := info.Destroy(); err != nil {
				logger.Warningf("cannot destroy newly created environment info: %v", err)
			}
			return nil, errors.Trace(err)
		}
	} else if err != nil {
		return nil, errors.Annotatef(err, "error reading environment info %q", cfg.Name())
	} else if !info.Initialized() {
		return nil,
			errors.Errorf(
				"found uninitialized environment info for %q; environment preparation probably in progress or interrupted",
				cfg.Name(),
			)
	} else if len(info.BootstrapConfig()) == 0 {
		return nil, errors.New("found environment info but no bootstrap config")
	} else {
		cfg, err = config.New(config.NoDefaults, info.BootstrapConfig())
		if err != nil {
			return nil, errors.Annotate(err, "cannot parse bootstrap config")
		}
		return New(cfg)
	}
}

// decorateAndWriteInfo decorates the info struct with information
// from the given cfg, and the writes that out to the filesystem.
func decorateAndWriteInfo(info configstore.EnvironInfo, cfg *config.Config) error {

	// Sanity check our config.
	var endpoint configstore.APIEndpoint
	if cert, ok := cfg.CACert(); !ok {
		return errors.Errorf("CACert is not set")
	} else if uuid, ok := cfg.UUID(); !ok {
		return errors.Errorf("UUID is not set")
	} else if adminSecret := cfg.AdminSecret(); adminSecret == "" {
		return errors.Errorf("admin-secret is not set")
	} else {
		endpoint = configstore.APIEndpoint{
			CACert:      cert,
			EnvironUUID: uuid,
		}
	}

	creds := configstore.APICredentials{
		User:     configstore.DefaultAdminUsername,
		Password: cfg.AdminSecret(),
	}
	if featureflag.Enabled(feature.JES) {
		endpoint.ServerUUID = endpoint.EnvironUUID
	}
	info.SetAPICredentials(creds)
	info.SetAPIEndpoint(endpoint)
	info.SetBootstrapConfig(cfg.AllAttrs())

	if err := info.Write(); err != nil {
		return errors.Annotatef(err, "cannot create environment info %q", cfg.Name())
	}

	return nil
}

func prepare(ctx BootstrapContext, cfg *config.Config, info configstore.EnvironInfo, p EnvironProvider) (Environ, error) {
	cfg, err := ensureAdminSecret(cfg)
	if err != nil {
		return nil, errors.Annotate(err, "cannot generate admin-secret")
	}
	cfg, err = ensureCertificate(cfg)
	if err != nil {
		return nil, errors.Annotate(err, "cannot ensure CA certificate")
	}
	cfg, err = ensureUUID(cfg)
	if err != nil {
		return nil, errors.Annotate(err, "cannot ensure uuid")
	}

	return p.PrepareForBootstrap(ctx, cfg)
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

// ensureUUID generates a new uuid and attaches it to
// the given environment configuration, unless the
// configuration already has one.
func ensureUUID(cfg *config.Config) (*config.Config, error) {
	_, hasUUID := cfg.UUID()
	if hasUUID {
		return cfg, nil
	}
	uuid, err := utils.NewUUID()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return cfg.Apply(map[string]interface{}{
		"uuid": uuid.String(),
	})
}

// Destroy destroys the environment and, if successful,
// its associated configuration data from the given store.
func Destroy(env Environ, store configstore.Storage) error {
	name := env.Config().Name()
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
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}
	if err := info.Destroy(); err != nil {
		return errors.Annotate(err, "cannot destroy environment configuration information")
	}
	return nil
}
