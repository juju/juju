// Copyright 2011-2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"github.com/juju/errors"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/jujuclient"
)

const (
	// ControllerModelName is the name of the admin model in each controller.
	ControllerModelName = "controller"

	// ControllerCharmName is the name of the controller charm.
	ControllerCharmName = "juju-controller"

	// ControllerApplicationName is the name of the controller application.
	ControllerApplicationName = "controller"

	// ControllerCharmArchive is the name of the controller charm archive.
	ControllerCharmArchive = "controller.charm"
)

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
	Cloud environscloudspec.CloudSpec

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

// PrepareController prepares a new controller based on the provided configuration.
// It is an error to prepare a controller if there already exists an
// entry in the client store with the same name.
//
// Upon success, Prepare will update the ClientStore with the details of
// the controller, admin account, and admin model.
func PrepareController(
	isCAASController bool,
	ctx environs.BootstrapContext,
	store jujuclient.ClientStore,
	args PrepareParams,
) (environs.BootstrapEnviron, error) {

	if err := args.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	_, err := store.ControllerByName(args.ControllerName)
	if err == nil {
		return nil, errors.AlreadyExistsf("controller %q", args.ControllerName)
	} else if !errors.Is(err, errors.NotFound) {
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

	cfg, details, err := prepare(ctx, p, args)
	if err != nil {
		return nil, errors.Trace(err)
	}

	do := func() error {
		if err := decorateAndWriteInfo(
			store, details, args.ControllerName, cfg.Name(),
		); err != nil {
			return errors.Annotatef(err, "cannot create controller %q info", args.ControllerName)
		}
		return nil
	}

	var env environs.BootstrapEnviron
	openParams := environs.OpenParams{
		ControllerUUID: args.ControllerConfig.ControllerUUID(),
		Cloud:          args.Cloud,
		Config:         cfg,
	}
	if isCAASController {
		details.ModelType = model.CAAS
		env, err = caas.Open(ctx, p, openParams, environs.NoopCredentialInvalidator())
	} else {
		details.ModelType = model.IAAS
		env, err = environs.Open(ctx, p, openParams, environs.NoopCredentialInvalidator())
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	if err := env.PrepareForBootstrap(ctx, args.ControllerName); err != nil {
		return nil, errors.Trace(err)
	}
	if err := do(); err != nil {
		return nil, errors.Trace(err)
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
	qualifiedModelName := jujuclient.QualifyModelName(
		details.AccountDetails.User,
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
) (*config.Config, prepareDetails, error) {
	var details prepareDetails

	cfg, err := config.New(config.NoDefaults, args.ModelConfig)
	if err != nil {
		return cfg, details, errors.Trace(err)
	}

	err = p.ValidateCloud(ctx, args.Cloud)
	if err != nil {
		return cfg, details, errors.Trace(err)
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
		return cfg, details, errors.New("controller config is missing CA certificate")
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
	details.ControllerDetails.CloudType = args.Cloud.Type
	details.BootstrapConfig.CloudType = args.Cloud.Type
	details.BootstrapConfig.Cloud = args.Cloud.Name
	details.BootstrapConfig.CloudRegion = args.Cloud.Region
	details.BootstrapConfig.CloudCACertificates = args.Cloud.CACertificates
	details.BootstrapConfig.SkipTLSVerify = args.Cloud.SkipTLSVerify
	details.CloudEndpoint = args.Cloud.Endpoint
	details.CloudIdentityEndpoint = args.Cloud.IdentityEndpoint
	details.CloudStorageEndpoint = args.Cloud.StorageEndpoint
	details.Credential = args.CredentialName

	if args.Cloud.SkipTLSVerify {
		if len(args.Cloud.CACertificates) > 0 && args.Cloud.CACertificates[0] != "" {
			return cfg, details, errors.NotValidf("cloud with both skip-TLS-verify=true and CA certificates")
		}
		logger.Warningf(ctx, "controller %v is configured to skip validity checks on the server's certificate", args.ControllerName)
	}

	return cfg, details, nil
}

type prepareDetails struct {
	jujuclient.ControllerDetails
	jujuclient.BootstrapConfig
	jujuclient.AccountDetails
	jujuclient.ModelDetails
}
