// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs

import (
	"fmt"
	"time"

	"github.com/juju/errors"
	"github.com/juju/utils"

	"github.com/juju/juju/cert"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/configstore"
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

// New returns a new environment based on the provided configuration.
func New(config *config.Config) (Environ, error) {
	p, err := Provider(config.Type())
	if err != nil {
		return nil, errors.Trace(err)
	}
	return p.Open(config)
}

// Prepare prepares a new environment based on the provided configuration.
// It is an error to prepare a environment if there already exists an
// entry in the config store with that name.
//
// TODO(axw) Prepare will be changed to accept PrepareForBootstrapParams
//           instead of just a Config.
func Prepare(
	ctx BootstrapContext,
	store configstore.Storage,
	controllerName string,
	args PrepareForBootstrapParams,
) (Environ, error) {
	info, err := store.ReadInfo(controllerName)
	if err == nil {
		return nil, errors.AlreadyExistsf("controller %q", controllerName)
	} else if !errors.IsNotFound(err) {
		return nil, errors.Annotatef(err, "error reading controller %q info", controllerName)
	}

	p, err := Provider(args.Config.Type())
	if err != nil {
		return nil, errors.Trace(err)
	}
	info = store.CreateInfo(controllerName)
	env, err := prepare(ctx, info, p, args)
	if err != nil {
		if err := info.Destroy(); err != nil {
			logger.Warningf(
				"cannot destroy newly created controller %q info: %v",
				controllerName, err,
			)
		}
		return nil, errors.Trace(err)
	}
	if err := decorateAndWriteInfo(info, env.Config()); err != nil {
		return nil, errors.Annotatef(err, "cannot create controller %q info", controllerName)
	}
	return env, nil
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
	endpoint.ServerUUID = endpoint.EnvironUUID
	info.SetAPICredentials(creds)
	info.SetAPIEndpoint(endpoint)
	info.SetBootstrapConfig(cfg.AllAttrs())
	return errors.Trace(info.Write())
}

func prepare(ctx BootstrapContext, info configstore.EnvironInfo, p EnvironProvider, args PrepareForBootstrapParams) (Environ, error) {
	cfg, err := ensureAdminSecret(args.Config)
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
	args.Config = cfg
	return p.PrepareForBootstrap(ctx, args)
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
//
// TODO(axw) the info should be stored against the name
// of the controller, not the environment name. For now
// we just make sure all the tests use the same name
// for the controller and the environment.
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
