// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"github.com/Azure/go-autorest/autorest"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/clock"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/provider/azure/internal/azureauth"
	"github.com/juju/juju/provider/azure/internal/azurestorage"
)

// Logger for the Azure provider.
var logger = loggo.GetLogger("juju.provider.azure")

// ProviderConfig contains configuration for the Azure providers.
type ProviderConfig struct {
	// Sender is the autorest.Sender that will be used by Azure
	// clients. If sender is nil, the default HTTP client sender
	// will be used.
	Sender autorest.Sender

	// RequestInspector will be used to inspect Azure requests
	// if it is non-nil.
	RequestInspector autorest.PrepareDecorator

	// NewStorageClient will be used to construct new storage
	// clients.
	NewStorageClient azurestorage.NewClientFunc

	// RetryClock is used when retrying API calls due to rate-limiting.
	RetryClock clock.Clock

	// RandomWindowsAdminPassword is a function used to generate
	// a random password for the Windows admin user.
	RandomWindowsAdminPassword func() string

	// InteractiveCreateServicePrincipal is a function used to
	// interactively create/update service principals with
	// password credentials.
	InteractiveCreateServicePrincipal azureauth.InteractiveCreateServicePrincipalFunc
}

// Validate validates the Azure provider configuration.
func (cfg ProviderConfig) Validate() error {
	if cfg.NewStorageClient == nil {
		return errors.NotValidf("nil NewStorageClient")
	}
	if cfg.RetryClock == nil {
		return errors.NotValidf("nil RetryClock")
	}
	if cfg.RandomWindowsAdminPassword == nil {
		return errors.NotValidf("nil RandomWindowsAdminPassword")
	}
	if cfg.InteractiveCreateServicePrincipal == nil {
		return errors.NotValidf("nil InteractiveCreateServicePrincipal")
	}
	return nil
}

type azureEnvironProvider struct {
	environProviderCredentials

	config ProviderConfig
}

// NewEnvironProvider returns a new EnvironProvider for Azure.
func NewEnvironProvider(config ProviderConfig) (*azureEnvironProvider, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Annotate(err, "validating environ provider configuration")
	}
	return &azureEnvironProvider{
		environProviderCredentials: environProviderCredentials{
			sender:                            config.Sender,
			requestInspector:                  config.RequestInspector,
			interactiveCreateServicePrincipal: config.InteractiveCreateServicePrincipal,
		},
		config: config,
	}, nil
}

// Open is part of the EnvironProvider interface.
func (prov *azureEnvironProvider) Open(args environs.OpenParams) (environs.Environ, error) {
	logger.Debugf("opening model %q", args.Config.Name())
	if err := validateCloudSpec(args.Cloud); err != nil {
		return nil, errors.Annotate(err, "validating cloud spec")
	}
	environ, err := newEnviron(prov, args.Cloud, args.Config)
	if err != nil {
		return nil, errors.Annotate(err, "opening model")
	}
	return environ, nil
}

// PrepareConfig is part of the EnvironProvider interface.
func (prov *azureEnvironProvider) PrepareConfig(args environs.PrepareConfigParams) (*config.Config, error) {
	if err := validateCloudSpec(args.Cloud); err != nil {
		return nil, errors.Annotate(err, "validating cloud spec")
	}
	return args.Config, nil
}

func validateCloudSpec(spec environs.CloudSpec) error {
	if err := spec.Validate(); err != nil {
		return errors.Trace(err)
	}
	if spec.Credential == nil {
		return errors.NotValidf("missing credential")
	}
	if authType := spec.Credential.AuthType(); authType != clientCredentialsAuthType {
		return errors.NotSupportedf("%q auth-type", authType)
	}
	return nil
}

// verifyCredentials issues a cheap, non-modifying request to Azure to
// verify the configured credentials. If verification fails, a user-friendly
// error will be returned, and the original error will be logged at debug
// level.
var verifyCredentials = func(e *azureEnviron) error {
	// TODO(axw) user-friendly error message
	return e.authorizer.refresh()
}
