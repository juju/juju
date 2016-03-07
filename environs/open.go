// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs

import (
	"crypto/rand"
	"fmt"
	"io"
	"time"

	"github.com/juju/errors"

	"github.com/juju/juju/cert"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/configstore"
	"github.com/juju/juju/jujuclient"
)

// ControllerModelName is the name of the admin model in each controller.
const ControllerModelName = "admin"

// adminUser is the initial admin user created for all controllers.
const adminUser = "admin@local"

// New returns a new environment based on the provided configuration.
func New(config *config.Config) (Environ, error) {
	p, err := Provider(config.Type())
	if err != nil {
		return nil, errors.Trace(err)
	}
	return p.Open(config)
}

// Prepare prepares a new controller based on the provided configuration.
// It is an error to prepare a controller if there already exists an
// entry in the client store with the same name.
//
// Upon success, Prepare will update the ClientStore with the details of
// the controller, admin account, and admin model.
func Prepare(
	ctx BootstrapContext,
	store jujuclient.ClientStore,
	controllerName string,
	args PrepareForBootstrapParams,
) (_ Environ, resultErr error) {

	_, err := store.ControllerByName(controllerName)
	if err == nil {
		return nil, errors.AlreadyExistsf("controller %q", controllerName)
	} else if !errors.IsNotFound(err) {
		return nil, errors.Annotatef(err, "error reading controller %q info", controllerName)
	}

	p, err := Provider(args.Config.Type())
	if err != nil {
		return nil, errors.Trace(err)
	}

	env, details, err := prepare(ctx, p, args)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if err := decorateAndWriteInfo(store, details, controllerName, env.Config()); err != nil {
		return nil, errors.Annotatef(err, "cannot create controller %q info", controllerName)
	}
	return env, nil
}

// decorateAndWriteInfo decorates the info struct with information
// from the given cfg, and the writes that out to the filesystem.
func decorateAndWriteInfo(
	store jujuclient.ClientStore,
	details prepareDetails,
	controllerName string,
	cfg *config.Config,
) error {
	accountName := details.User
	modelName := cfg.Name()
	if err := store.UpdateController(controllerName, details.ControllerDetails); err != nil {
		return errors.Trace(err)
	}
	if err := store.UpdateAccount(controllerName, accountName, details.AccountDetails); err != nil {
		return errors.Trace(err)
	}
	if err := store.SetCurrentAccount(controllerName, accountName); err != nil {
		return errors.Trace(err)
	}
	if err := store.UpdateModel(controllerName, accountName, modelName, details.ModelDetails); err != nil {
		return errors.Trace(err)
	}
	if err := store.SetCurrentModel(controllerName, accountName, modelName); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func prepare(
	ctx BootstrapContext,
	p EnvironProvider,
	args PrepareForBootstrapParams,
) (Environ, prepareDetails, error) {
	var details prepareDetails
	cfg, adminSecret, err := ensureAdminSecret(args.Config)
	if err != nil {
		return nil, details, errors.Annotate(err, "cannot generate admin-secret")
	}
	cfg, caCert, err := ensureCertificate(cfg)
	if err != nil {
		return nil, details, errors.Annotate(err, "cannot ensure CA certificate")
	}
	args.Config = cfg
	env, err := p.PrepareForBootstrap(ctx, args)
	if err != nil {
		return nil, details, errors.Trace(err)
	}

	details.CACert = caCert
	details.ControllerUUID = cfg.ControllerUUID()
	details.User = adminUser
	details.Password = adminSecret
	details.ModelUUID = cfg.UUID()

	return env, details, nil
}

type prepareDetails struct {
	jujuclient.ControllerDetails
	jujuclient.AccountDetails
	jujuclient.ModelDetails
}

// ensureAdminSecret returns a config with a non-empty admin-secret.
func ensureAdminSecret(cfg *config.Config) (*config.Config, string, error) {
	if secret := cfg.AdminSecret(); secret != "" {
		return cfg, secret, nil
	}

	// Generate a random string.
	buf := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, buf); err != nil {
		return nil, "", errors.Annotate(err, "generating random secret")
	}
	secret := fmt.Sprintf("%x", buf)

	cfg, err := cfg.Apply(map[string]interface{}{"admin-secret": secret})
	if err != nil {
		return nil, "", errors.Trace(err)
	}
	return cfg, secret, nil
}

// ensureCertificate generates a new CA certificate and
// attaches it to the given controller configuration,
// unless the configuration already has one.
func ensureCertificate(cfg *config.Config) (*config.Config, string, error) {
	caCert, hasCACert := cfg.CACert()
	_, hasCAKey := cfg.CAPrivateKey()
	if hasCACert && hasCAKey {
		return cfg, caCert, nil
	}
	if hasCACert && !hasCAKey {
		return nil, "", errors.Errorf("controller configuration with a certificate but no CA private key")
	}

	caCert, caKey, err := cert.NewCA(cfg.Name(), time.Now().UTC().AddDate(10, 0, 0))
	if err != nil {
		return nil, "", errors.Trace(err)
	}
	cfg, err = cfg.Apply(map[string]interface{}{
		"ca-cert":        string(caCert),
		"ca-private-key": string(caKey),
	})
	if err != nil {
		return nil, "", errors.Trace(err)
	}
	return cfg, string(caCert), nil
}

// Destroy destroys the controller and, if successful,
// its associated configuration data from the given store.
func Destroy(
	controllerName string,
	env Environ,
	legacyStore configstore.Storage,
	store jujuclient.ControllerRemover,
) error {
	err := store.RemoveController(controllerName)
	if err != nil && !errors.IsNotFound(err) {
		return errors.Trace(err)
	}
	modelName := env.Config().Name()
	if err := env.Destroy(); err != nil {
		return errors.Trace(err)
	}
	info, err := legacyStore.ReadInfo(
		configstore.EnvironInfoName(controllerName, modelName),
	)
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
