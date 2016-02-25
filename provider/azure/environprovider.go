// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"github.com/Azure/azure-sdk-for-go/Godeps/_workspace/src/github.com/Azure/go-autorest/autorest"
	"github.com/juju/errors"
	"github.com/juju/loggo"

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
}

// Validate validates the Azure provider configuration.
func (cfg ProviderConfig) Validate() error {
	if cfg.NewStorageClient == nil {
		return errors.NotValidf("nil NewStorageClient")
	}
	if cfg.StorageAccountNameGenerator == nil {
		return errors.NotValidf("nil StorageAccountNameGenerator")
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
func (prov *azureEnvironProvider) Open(cfg *config.Config) (environs.Environ, error) {
	logger.Debugf("opening model %q", cfg.Name())
	environ, err := newEnviron(prov, cfg)
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
	return []string{
		configAttrSubscriptionId,
		configAttrTenantId,
		configAttrAppId,
		configAttrAppPassword,
		configAttrLocation,
		configAttrControllerResourceGroup,
		configAttrStorageAccountType,
	}
}

// PrepareForCreateEnvironment is specified in the EnvironProvider interface.
func (prov *azureEnvironProvider) PrepareForCreateEnvironment(cfg *config.Config) (*config.Config, error) {
	env, err := newEnviron(prov, cfg)
	if err != nil {
		return nil, errors.Annotate(err, "opening model")
	}
	return env.initResourceGroup()
}

// PrepareForBootstrap is specified in the EnvironProvider interface.
func (prov *azureEnvironProvider) PrepareForBootstrap(ctx environs.BootstrapContext, args environs.PrepareForBootstrapParams) (environs.Environ, error) {

	// Ensure that internal configuration is not specified, and then set
	// what we can now. We only need to do this during bootstrap. Validate
	// will check for changes later.
	unknownAttrs := args.Config.UnknownAttrs()
	for _, key := range internalConfigAttributes {
		if _, ok := unknownAttrs[key]; ok {
			return nil, errors.Errorf(`internal config %q must not be specified`, key)
		}
	}

	attrs := map[string]interface{}{
		configAttrLocation:        args.CloudRegion,
		configAttrEndpoint:        args.CloudEndpoint,
		configAttrStorageEndpoint: args.CloudStorageEndpoint,

		// Record the UUID that will be used for the controller
		// model, which contains shared resources.
		configAttrControllerResourceGroup: resourceGroupName(args.Config),
	}
	switch authType := args.Credentials.AuthType(); authType {
	case cloud.UserPassAuthType:
		for k, v := range args.Credentials.Attributes() {
			attrs[k] = v
		}
	default:
		return nil, errors.NotSupportedf("%q auth-type", authType)
	}
	cfg, err := args.Config.Apply(attrs)
	if err != nil {
		return nil, errors.Annotate(err, "updating config")
	}

	env, err := prov.Open(cfg)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if ctx.ShouldVerifyCredentials() {
		if err := verifyCredentials(env.(*azureEnviron)); err != nil {
			return nil, errors.Trace(err)
		}
	}
	return env, nil
}

// SecretAttrs is specified in the EnvironProvider interface.
func (prov *azureEnvironProvider) SecretAttrs(cfg *config.Config) (map[string]string, error) {
	unknownAttrs := cfg.UnknownAttrs()
	secretAttrs := map[string]string{
		configAttrAppPassword: unknownAttrs[configAttrAppPassword].(string),
	}
	if storageAccountKey, ok := unknownAttrs[configAttrStorageAccountKey].(string); ok {
		secretAttrs[configAttrStorageAccountKey] = storageAccountKey
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
