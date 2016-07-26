// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"github.com/Azure/azure-sdk-for-go/Godeps/_workspace/src/github.com/Azure/go-autorest/autorest"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/clock"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
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

	// StorageAccountNameGenerator is a function returning storage
	// account names.
	StorageAccountNameGenerator func() string

	// RetryClock is used when retrying API calls due to rate-limiting.
	RetryClock clock.Clock
}

// Validate validates the Azure provider configuration.
func (cfg ProviderConfig) Validate() error {
	if cfg.NewStorageClient == nil {
		return errors.NotValidf("nil NewStorageClient")
	}
	if cfg.StorageAccountNameGenerator == nil {
		return errors.NotValidf("nil StorageAccountNameGenerator")
	}
	if cfg.RetryClock == nil {
		return errors.NotValidf("nil RetryClock")
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
	return &azureEnvironProvider{config: config}, nil
}

// Open is specified in the EnvironProvider interface.
func (prov *azureEnvironProvider) Open(args environs.OpenParams) (environs.Environ, error) {
	logger.Debugf("opening model %q", args.Config.Name())
	environ, err := newEnviron(prov, args.Config)
	if err != nil {
		return nil, errors.Annotate(err, "opening model")
	}
	return environ, nil
}

// RestrictedConfigAttributes is specified in the EnvironProvider interface.
//
// The result of RestrictedConfigAttributes is the names of attributes that
// will be copied across to a hosted environment's initial configuration.
func (prov *azureEnvironProvider) RestrictedConfigAttributes() []string {
	// TODO(axw) there should be no restricted attributes.
	return []string{
		configAttrLocation,
		configAttrEndpoint,
		configAttrStorageEndpoint,
	}
}

// PrepareConfig is specified in the EnvironProvider interface.
func (prov *azureEnvironProvider) PrepareConfig(args environs.PrepareConfigParams) (*config.Config, error) {
	attrs := map[string]interface{}{
		configAttrLocation:        args.Cloud.Region,
		configAttrEndpoint:        args.Cloud.Endpoint,
		configAttrStorageEndpoint: args.Cloud.StorageEndpoint,
	}
	switch authType := args.Cloud.Credential.AuthType(); authType {
	case cloud.UserPassAuthType:
		for k, v := range args.Cloud.Credential.Attributes() {
			attrs[k] = v
		}
	default:
		return nil, errors.NotSupportedf("%q auth-type", authType)
	}
	cfg, err := args.Config.Apply(attrs)
	if err != nil {
		return nil, errors.Annotate(err, "updating config")
	}
	return cfg, nil
}

// SecretAttrs is specified in the EnvironProvider interface.
func (prov *azureEnvironProvider) SecretAttrs(cfg *config.Config) (map[string]string, error) {
	unknownAttrs := cfg.UnknownAttrs()
	secretAttrs := map[string]string{
		configAttrAppPassword: unknownAttrs[configAttrAppPassword].(string),
	}
	return secretAttrs, nil
}

// verifyCredentials issues a cheap, non-modifying request to Azure to
// verify the configured credentials. If verification fails, a user-friendly
// error will be returned, and the original error will be logged at debug
// level.
var verifyCredentials = func(e *azureEnviron) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	// TODO(axw) user-friendly error message
	return e.config.token.EnsureFresh()
}
