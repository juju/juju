// Copyright 2011-2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/permission"
)

// ControllerModelName is the name of the admin model in each controller.
const ControllerModelName = "controller"

// PrepareParams contains the parameters for preparing a controller Environ
// for bootstrapping.
type PrepareParams struct {
	// ModelConfig contains the base configuration for the controller model.
	//
	// This includes the model name, cloud type, any user-supplied
	// configuration, config inherited from controller, and any defaults.
	ModelConfig map[string]interface{}

	// ControllerConfig is the configuration of the controller being prepared.
	ControllerConfig controller.Config

	// ControllerName is the name of the controller being prepared.
	ControllerName string

	// Cloud is the specification of the cloud that the controller is
	// being prepared for.
	Cloud environs.CloudSpec

	// CredentialName is the name of the credential to use to bootstrap.
	// This will be empty for auto-detected credentials.
	CredentialName string

	// AdminSecret contains the password for the admin user.
	AdminSecret string
}

// Validate validates the PrepareParams.
func (p PrepareParams) Validate() error {
	if err := p.ControllerConfig.Validate(); err != nil {
		return errors.Annotate(err, "validating controller config")
	}
	if p.ControllerName == "" {
		return errors.NotValidf("empty controller name")
	}
	if p.Cloud.Name == "" {
		return errors.NotValidf("empty cloud name")
	}
	if p.AdminSecret == "" {
		return errors.NotValidf("empty admin-secret")
	}
	return nil
}

// Prepare prepares a new controller based on the provided configuration.
// It is an error to prepare a controller if there already exists an
// entry in the client store with the same name.
//
// Upon success, Prepare will update the ClientStore with the details of
// the controller, admin account, and admin model.
func Prepare(
	ctx environs.BootstrapContext,
	store jujuclient.ClientStore,
	args PrepareParams,
) (environs.Environ, error) {

	if err := args.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	_, err := store.ControllerByName(args.ControllerName)
	if err == nil {
		return nil, errors.AlreadyExistsf("controller %q", args.ControllerName)
	} else if !errors.IsNotFound(err) {
		return nil, errors.Annotatef(err, "error reading controller %q info", args.ControllerName)
	}

	cloudType, ok := args.ModelConfig["type"].(string)
	if !ok {
		return nil, errors.NotFoundf("cloud type in base configuration")
	}

	p, err := environs.Provider(cloudType)
	if err != nil {
		return nil, errors.Trace(err)
	}

	env, details, err := prepare(ctx, p, args)
	if err != nil {
		return nil, errors.Trace(err)
	}

	if err := decorateAndWriteInfo(
		store, details, args.ControllerName, env.Config().Name(),
	); err != nil {
		return nil, errors.Annotatef(err, "cannot create controller %q info", args.ControllerName)
	}
	return env, nil
}

// decorateAndWriteInfo decorates the info struct with information
// from the given cfg, and the writes that out to the filesystem.
func decorateAndWriteInfo(
	store jujuclient.ClientStore,
	details prepareDetails,
	controllerName, modelName string,
) error {
	qualifiedModelName := jujuclient.JoinOwnerModelName(
		names.NewUserTag(details.AccountDetails.User),
		modelName,
	)
	if err := store.AddController(controllerName, details.ControllerDetails); err != nil {
		return errors.Trace(err)
	}
	if err := store.UpdateBootstrapConfig(controllerName, details.BootstrapConfig); err != nil {
		return errors.Trace(err)
	}
	if err := store.UpdateAccount(controllerName, details.AccountDetails); err != nil {
		return errors.Trace(err)
	}
	if err := store.UpdateModel(controllerName, qualifiedModelName, details.ModelDetails); err != nil {
		return errors.Trace(err)
	}
	if err := store.SetCurrentModel(controllerName, qualifiedModelName); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func prepare(
	ctx environs.BootstrapContext,
	p environs.EnvironProvider,
	args PrepareParams,
) (environs.Environ, prepareDetails, error) {
	var details prepareDetails

	cfg, err := config.New(config.NoDefaults, args.ModelConfig)
	if err != nil {
		return nil, details, errors.Trace(err)
	}

	cfg, err = p.PrepareConfig(environs.PrepareConfigParams{args.Cloud, cfg})
	if err != nil {
		return nil, details, errors.Trace(err)
	}
	env, err := p.Open(environs.OpenParams{
		Cloud:  args.Cloud,
		Config: cfg,
	})
	if err != nil {
		return nil, details, errors.Trace(err)
	}
	if err := env.PrepareForBootstrap(ctx); err != nil {
		return nil, details, errors.Trace(err)
	}

	// We store the base configuration only; we don't want the
	// default attributes, generated secrets/certificates, or
	// UUIDs stored in the bootstrap config. Make a copy, so
	// we don't disturb the caller's config map.
	details.Config = make(map[string]interface{})
	for k, v := range args.ModelConfig {
		details.Config[k] = v
	}
	delete(details.Config, config.UUIDKey)

	// TODO(axw) change signature of CACert() to not return a bool.
	// It's no longer possible to have a controller config without
	// a CA certificate.
	caCert, ok := args.ControllerConfig.CACert()
	if !ok {
		return nil, details, errors.New("controller config is missing CA certificate")
	}

	// We want to store attributes describing how a controller has been configured.
	// These do not include the CACert or UUID since they will be replaced with new
	// values when/if we need to use this configuration.
	details.ControllerConfig = make(controller.Config)
	for k, v := range args.ControllerConfig {
		if k == controller.CACertKey || k == controller.ControllerUUIDKey {
			continue
		}
		details.ControllerConfig[k] = v
	}
	for k, v := range args.ControllerConfig {
		if k == controller.CACertKey || k == controller.ControllerUUIDKey {
			continue
		}
		details.ControllerConfig[k] = v
	}
	details.CACert = caCert
	details.ControllerUUID = args.ControllerConfig.ControllerUUID()
	details.ControllerModelUUID = args.ModelConfig[config.UUIDKey].(string)
	details.User = environs.AdminUser
	details.Password = args.AdminSecret
	details.LastKnownAccess = string(permission.SuperuserAccess)
	details.ModelUUID = cfg.UUID()
	details.ControllerDetails.Cloud = args.Cloud.Name
	details.ControllerDetails.CloudRegion = args.Cloud.Region
	details.BootstrapConfig.CloudType = args.Cloud.Type
	details.BootstrapConfig.Cloud = args.Cloud.Name
	details.BootstrapConfig.CloudRegion = args.Cloud.Region
	details.CloudEndpoint = args.Cloud.Endpoint
	details.CloudIdentityEndpoint = args.Cloud.IdentityEndpoint
	details.CloudStorageEndpoint = args.Cloud.StorageEndpoint
	details.Credential = args.CredentialName

	return env, details, nil
}

type prepareDetails struct {
	jujuclient.ControllerDetails
	jujuclient.BootstrapConfig
	jujuclient.AccountDetails
	jujuclient.ModelDetails
}
