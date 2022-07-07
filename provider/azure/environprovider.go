// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	stdcontext "context"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/jsonschema"
	"github.com/juju/loggo"

	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/provider/azure/internal/errorutils"
)

const (
	// provider version 1 introduces the "common" deployment,
	// which contains common resources such as the virtual
	// network and network security group.
	providerVersion1 = 1

	currentProviderVersion = providerVersion1
)

// Logger for the Azure provider.
var logger = loggo.GetLogger("juju.provider.azure")

// ProviderConfig contains configuration for the Azure providers.
type ProviderConfig struct {
	// Sender is the autorest.Sender that will be used by Azure
	// clients. If sender is nil, the default HTTP client sender
	// will be used. Used for testing.
	Sender policy.Transporter

	// RequestInspector will be used to inspect Azure requests
	// if it is non-nil. Used for testing.
	RequestInspector policy.Policy

	// Retry is set by tests to limit the default retries.
	Retry policy.RetryOptions

	// CreateTokenCredential is set by tests to create a token.
	CreateTokenCredential func(appId, appPassword, tenantID string, opts azcore.ClientOptions) (azcore.TokenCredential, error)

	// RetryClock is used for retrying some operations, like
	// waiting for deployments to complete.
	//
	// Retries due to rate-limiting are handled by the go-autorest
	// package, which uses "time" directly. We cannot mock the
	// waiting in that case.
	RetryClock clock.Clock

	// GneerateSSHKey is a functio nused to generate a new SSH
	// key pair for provisioning Linux machines.
	GenerateSSHKey func(comment string) (private, public string, _ error)

	// ServicePrincipalCreator is the interface used to create service principals.
	ServicePrincipalCreator ServicePrincipalCreator

	// AzureCLI is the interface the to Azure CLI (az) command.
	AzureCLI AzureCLI

	// LoadBalancerSkuName is the load balancer SKU name.
	// Legal values are determined by the Azure SDK.
	LoadBalancerSkuName string
}

// Validate validates the Azure provider configuration.
func (cfg ProviderConfig) Validate() error {
	if cfg.RetryClock == nil {
		return errors.NotValidf("nil RetryClock")
	}
	if cfg.GenerateSSHKey == nil {
		return errors.NotValidf("nil GenerateSSHKey")
	}
	if cfg.ServicePrincipalCreator == nil {
		return errors.NotValidf("nil ServicePrincipalCreator")
	}
	if cfg.AzureCLI == nil {
		return errors.NotValidf("nil AzureCLI")
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
			servicePrincipalCreator: config.ServicePrincipalCreator,
			azureCLI:                config.AzureCLI,
			transporter:             config.Sender,
		},
		config: config,
	}, nil
}

// Version is part of the EnvironProvider interface.
func (prov *azureEnvironProvider) Version() int {
	return currentProviderVersion
}

// Open is part of the EnvironProvider interface.
func (prov *azureEnvironProvider) Open(ctx stdcontext.Context, args environs.OpenParams) (environs.Environ, error) {
	logger.Debugf("opening model %q", args.Config.Name())
	environ := &azureEnviron{
		provider: prov,
	}
	// Config is needed before cloud spec.
	if err := environ.SetConfig(args.Config); err != nil {
		return nil, errors.Trace(err)
	}

	if err := environ.SetCloudSpec(ctx, args.Cloud); err != nil {
		return nil, errors.Trace(err)
	}
	return environ, nil
}

// CloudSchema returns the schema used to validate input for add-cloud.  Since
// this provider does not support custom clouds, this always returns nil.
func (p azureEnvironProvider) CloudSchema() *jsonschema.Schema {
	return nil
}

// Ping tests the connection to the cloud, to verify the endpoint is valid.
func (p azureEnvironProvider) Ping(ctx context.ProviderCallContext, endpoint string) error {
	return errors.NotImplementedf("Ping")
}

// PrepareConfig is part of the EnvironProvider interface.
func (prov *azureEnvironProvider) PrepareConfig(args environs.PrepareConfigParams) (*config.Config, error) {
	if err := validateCloudSpec(args.Cloud); err != nil {
		return nil, errors.Annotate(err, "validating cloud spec")
	}
	// Set the default block-storage source.
	attrs := make(map[string]interface{})
	if _, ok := args.Config.StorageDefaultBlockSource(); !ok {
		attrs[config.StorageDefaultBlockSourceKey] = azureStorageProviderType
	}
	if len(attrs) == 0 {
		return args.Config, nil
	}
	return args.Config.Apply(attrs)
}

func validateCloudSpec(spec environscloudspec.CloudSpec) error {
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
var verifyCredentials = func(e *azureEnviron, ctx context.ProviderCallContext) error {
	// This is used at bootstrap - the ctx invalid credential callback will log
	// a suitable message.
	_, err := e.credential.GetToken(ctx, policy.TokenRequestOptions{
		Scopes: []string{"https://management.core.windows.net/.default"},
	})
	return errorutils.HandleCredentialError(err, ctx)
}
