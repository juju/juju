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
func Prepare(
	ctx BootstrapContext,
	legacyStore configstore.Storage,
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

	info := legacyStore.CreateInfo(
		configstore.EnvironInfoName(controllerName, args.Config.Name()),
	)
	defer func() {
		if resultErr == nil {
			return
		}
		if err := info.Destroy(); err != nil {
			logger.Warningf(
				"cannot destroy newly created controller %q info: %v",
				controllerName, err,
			)
		}
	}()

	env, details, err := prepare(ctx, info, p, args)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if err := decorateAndWriteInfo(info, store, details, controllerName, env.Config()); err != nil {
		return nil, errors.Annotatef(err, "cannot create controller %q info", controllerName)
	}
	return env, nil
}

// decorateAndWriteInfo decorates the info struct with information
// from the given cfg, and the writes that out to the filesystem.
func decorateAndWriteInfo(
	info configstore.EnvironInfo,
	store jujuclient.ClientStore,
	details prepareDetails,
	controllerName string,
	cfg *config.Config,
) error {

	// TODO(axw) drop this when the tests are all updated to rely only on
	// the jujuclient store.
	endpoint := configstore.APIEndpoint{
		CACert:     details.CACert,
		ModelUUID:  details.ControllerUUID,
		ServerUUID: details.ControllerUUID,
	}
	creds := configstore.APICredentials{
		User:     details.User,
		Password: details.Password,
	}
	info.SetAPICredentials(creds)
	info.SetAPIEndpoint(endpoint)
	info.SetBootstrapConfig(cfg.AllAttrs())
	if err := info.Write(); err != nil {
		return errors.Trace(err)
	}

	accountName := details.User
	modelName := cfg.Name()
	modelDetails := jujuclient.ModelDetails{details.ControllerUUID}
	if err := store.UpdateController(controllerName, details.ControllerDetails); err != nil {
		return errors.Trace(err)
	}
	if err := store.UpdateAccount(controllerName, accountName, details.AccountDetails); err != nil {
		return errors.Trace(err)
	}
	if err := store.SetCurrentAccount(controllerName, accountName); err != nil {
		return errors.Trace(err)
	}
	if err := store.UpdateModel(controllerName, accountName, modelName, modelDetails); err != nil {
		return errors.Trace(err)
	}
	if err := store.SetCurrentModel(controllerName, accountName, modelName); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func prepare(
	ctx BootstrapContext,
	info configstore.EnvironInfo,
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
	cfg, uuid, err := ensureUUID(cfg)
	if err != nil {
		return nil, details, errors.Annotate(err, "cannot ensure uuid")
	}
	args.Config = cfg
	env, err := p.PrepareForBootstrap(ctx, args)
	if err != nil {
		return nil, details, errors.Trace(err)
	}

	details.CACert = caCert
	details.ControllerUUID = uuid
	details.User = adminUser
	details.Password = adminSecret

	return env, details, nil
}

type prepareDetails struct {
	jujuclient.ControllerDetails
	jujuclient.AccountDetails
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

// ensureUUID generates a new uuid and attaches it to
// the given environment configuration, unless the
// configuration already has one.
func ensureUUID(cfg *config.Config) (*config.Config, string, error) {
	if uuid, hasUUID := cfg.UUID(); hasUUID {
		return cfg, uuid, nil
	}
	uuid, err := utils.NewUUID()
	if err != nil {
		return nil, "", errors.Trace(err)
	}
	cfg, err = cfg.Apply(map[string]interface{}{
		"uuid": uuid.String(),
	})
	if err != nil {
		return nil, "", errors.Trace(err)
	}
	return cfg, uuid.String(), nil
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
