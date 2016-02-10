// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs

import (
	"crypto/rand"
	"fmt"
	"io"
	"time"

	"github.com/juju/errors"
	"github.com/juju/utils"

	"github.com/juju/juju/cert"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/configstore"
	"github.com/juju/juju/jujuclient"
)

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
func Prepare(
	ctx BootstrapContext,
	store configstore.Storage,
	controllerStore jujuclient.ControllerStore,
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
	if err := decorateAndWriteInfo(info, controllerName, controllerStore, env.Config()); err != nil {
		return nil, errors.Annotatef(err, "cannot create controller %q info", controllerName)
	}
	return env, nil
}

// decorateAndWriteInfo decorates the info struct with information
// from the given cfg, and the writes that out to the filesystem.
func decorateAndWriteInfo(info configstore.EnvironInfo, controllerName string, controllerStore jujuclient.ControllerStore, cfg *config.Config) error {

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
			CACert:    cert,
			ModelUUID: uuid,
		}
	}

	creds := configstore.APICredentials{
		User:     configstore.DefaultAdminUsername,
		Password: cfg.AdminSecret(),
	}
	endpoint.ServerUUID = endpoint.ModelUUID
	info.SetAPICredentials(creds)
	info.SetAPIEndpoint(endpoint)
	info.SetBootstrapConfig(cfg.AllAttrs())

	connectionDetails := info.APIEndpoint()
	c := jujuclient.ControllerDetails{
		connectionDetails.Hostnames,
		endpoint.ServerUUID,
		connectionDetails.Addresses,
		endpoint.CACert,
	}
	if err := controllerStore.UpdateController(controllerName, c); err != nil {
		return errors.Trace(err)
	}

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

func randomKey() string {
	buf := make([]byte, 16)
	_, err := io.ReadFull(rand.Reader, buf)
	if err != nil {
		panic(fmt.Errorf("error from crypto rand: %v", err))
	}
	return fmt.Sprintf("%x", buf)
}

// ensureCertificate generates a new CA certificate and
// attaches it to the given controller configuration,
// unless the configuration already has one.
func ensureCertificate(cfg *config.Config) (*config.Config, error) {
	_, hasCACert := cfg.CACert()
	_, hasCAKey := cfg.CAPrivateKey()
	if hasCACert && hasCAKey {
		return cfg, nil
	}
	if hasCACert && !hasCAKey {
		return nil, fmt.Errorf("controller configuration with a certificate but no CA private key")
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

// Destroy destroys the controller and, if successful,
// its associated configuration data from the given store.
func Destroy(controllerName string, env Environ, store configstore.Storage) error {
	if err := env.Destroy(); err != nil {
		return err
	}
	return DestroyInfo(controllerName, store)
}

// DestroyInfo destroys the configuration data for the named
// controller from the given store.
func DestroyInfo(controllerName string, store configstore.Storage) error {
	info, err := store.ReadInfo(controllerName)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}
	if err := info.Destroy(); err != nil {
		return errors.Annotate(err, "cannot destroy controller information")
	}
	return nil
}
