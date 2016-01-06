// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"strings"

	"github.com/Azure/azure-sdk-for-go/Godeps/_workspace/src/github.com/Azure/go-autorest/autorest/azure"
	"github.com/Azure/azure-sdk-for-go/arm/storage"
	"github.com/juju/errors"
	"github.com/juju/schema"

	"github.com/juju/juju/environs/config"
)

const (
	configAttrAppId              = "application-id"
	configAttrSubscriptionId     = "subscription-id"
	configAttrTenantId           = "tenant-id"
	configAttrAppPassword        = "application-password"
	configAttrLocation           = "location"
	configAttrStorageAccountType = "storage-account-type"

	// The below bits are internal book-keeping things, rather than
	// configuration. Config is just what we have to work with.

	// configAttrStorageAccount is the name of the storage account. We
	// can't just use a well-defined name for the storage acocunt because
	// storage account names must be globally unique; each storage account
	// has an associated public DNS entry.
	configAttrStorageAccount = "storage-account"

	// configAttrStorageAccountKey is the primary key for the storage
	// account.
	configAttrStorageAccountKey = "storage-account-key"

	// configAttrControllerResourceGroup is the resource group
	// corresponding to the controller environment. Each environment needs
	// to know this because some resources are shared, and live in the
	// controller environment's resource group.
	configAttrControllerResourceGroup = "controller-resource-group"
)

var configFields = schema.Fields{
	configAttrLocation:                schema.String(),
	configAttrAppId:                   schema.String(),
	configAttrSubscriptionId:          schema.String(),
	configAttrTenantId:                schema.String(),
	configAttrAppPassword:             schema.String(),
	configAttrStorageAccount:          schema.String(),
	configAttrStorageAccountKey:       schema.String(),
	configAttrStorageAccountType:      schema.String(),
	configAttrControllerResourceGroup: schema.String(),
}

var configDefaults = schema.Defaults{
	configAttrStorageAccount:          schema.Omit,
	configAttrStorageAccountKey:       schema.Omit,
	configAttrControllerResourceGroup: schema.Omit,
	configAttrStorageAccountType:      string(storage.StandardLRS),
}

var requiredConfigAttributes = []string{
	configAttrAppId,
	configAttrAppPassword,
	configAttrSubscriptionId,
	configAttrTenantId,
	configAttrLocation,
	configAttrControllerResourceGroup,
}

var immutableConfigAttributes = []string{
	configAttrSubscriptionId,
	configAttrTenantId,
	configAttrControllerResourceGroup,
	configAttrStorageAccount,
	configAttrStorageAccountType,
}

var internalConfigAttributes = []string{
	configAttrStorageAccount,
	configAttrStorageAccountKey,
	configAttrControllerResourceGroup,
}

type azureEnvironConfig struct {
	*config.Config
	token                   *azure.ServicePrincipalToken
	subscriptionId          string
	location                string // canonicalized
	storageAccount          string
	storageAccountKey       string
	storageAccountType      storage.AccountType
	controllerResourceGroup string
}

var knownStorageAccountTypes = []string{
	"Standard_LRS", "Standard_GRS", "Standard_RAGRS", "Standard_ZRS", "Premium_LRS",
}

func (prov *azureEnvironProvider) newConfig(cfg *config.Config) (*azureEnvironConfig, error) {
	azureConfig, err := validateConfig(cfg, nil)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return azureConfig, nil
}

// Validate ensures that the provided configuration is valid for this
// provider, and that changes between the old (if provided) and new
// configurations are valid.
func (*azureEnvironProvider) Validate(newCfg, oldCfg *config.Config) (*config.Config, error) {
	_, err := validateConfig(newCfg, oldCfg)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return newCfg, nil
}

func validateConfig(newCfg, oldCfg *config.Config) (*azureEnvironConfig, error) {
	err := config.Validate(newCfg, oldCfg)
	if err != nil {
		return nil, err
	}

	validated, err := newCfg.ValidateUnknownAttrs(configFields, configDefaults)
	if err != nil {
		return nil, err
	}

	// Ensure required configuration is provided.
	for _, key := range requiredConfigAttributes {
		if value, ok := validated[key].(string); !ok || value == "" {
			return nil, errors.Errorf("%q config not specified", key)
		}
	}
	if oldCfg != nil {
		// Ensure immutable configuration isn't changed.
		oldUnknownAttrs := oldCfg.UnknownAttrs()
		for _, key := range immutableConfigAttributes {
			oldValue, hadValue := oldUnknownAttrs[key].(string)
			if hadValue {
				newValue, haveValue := validated[key].(string)
				if !haveValue {
					return nil, errors.Errorf(
						"cannot remove immutable %q config", key,
					)
				}
				if newValue != oldValue {
					return nil, errors.Errorf(
						"cannot change immutable %q config (%v -> %v)",
						key, oldValue, newValue,
					)
				}
			}
			// It's valid to go from not having to having.
		}
		// TODO(axw) figure out how we intend to handle changing
		// secrets, such as application key
	}

	location := canonicalLocation(validated[configAttrLocation].(string))
	appId := validated[configAttrAppId].(string)
	subscriptionId := validated[configAttrSubscriptionId].(string)
	tenantId := validated[configAttrTenantId].(string)
	appPassword := validated[configAttrAppPassword].(string)
	storageAccount, _ := validated[configAttrStorageAccount].(string)
	storageAccountKey, _ := validated[configAttrStorageAccountKey].(string)
	storageAccountType := validated[configAttrStorageAccountType].(string)
	controllerResourceGroup := validated[configAttrControllerResourceGroup].(string)

	if newCfg.FirewallMode() == config.FwGlobal {
		// We do not currently support the "global" firewall mode.
		return nil, errNoFwGlobal
	}

	if !isKnownStorageAccountType(storageAccountType) {
		return nil, errors.Errorf(
			"invalid storage account type %q, expected one of: %q",
			storageAccountType, knownStorageAccountTypes,
		)
	}

	token, err := azure.NewServicePrincipalToken(
		appId, appPassword, tenantId,
		azure.AzureResourceManagerScope,
	)
	if err != nil {
		return nil, errors.Annotate(err, "constructing service principal token")
	}

	azureConfig := &azureEnvironConfig{
		newCfg,
		token,
		subscriptionId,
		location,
		storageAccount,
		storageAccountKey,
		storage.AccountType(storageAccountType),
		controllerResourceGroup,
	}

	return azureConfig, nil
}

// isKnownStorageAccountType reports whether or not the given string identifies
// a known storage account type.
func isKnownStorageAccountType(t string) bool {
	for _, knownStorageAccountType := range knownStorageAccountTypes {
		if t == knownStorageAccountType {
			return true
		}
	}
	return false
}

// canonicalLocation returns the canonicalized location string. This involves
// stripping whitespace, and lowercasing. The ARM APIs do not support embedded
// whitespace, whereas the old Service Management APIs used to; we allow the
// user to provide either, and canonicalize them to one form that ARM allows.
func canonicalLocation(s string) string {
	s = strings.Replace(s, " ", "", -1)
	return strings.ToLower(s)
}

// TODO(axw) update with prose re credentials
var boilerplateYAML = `
# https://juju.ubuntu.com/docs/config-azure.html
azure:
    type: azure

    # NOTE: below we refer to the "Azure CLI", which is a CLI for Azure
    # provided by Microsoft. Please see the documentation for this at:
    #   https://azure.microsoft.com/en-us/documentation/articles/xplat-cli/

    # application-id is the ID of an application you create in Azure Active
    # Directory for Juju to use. For instructions on how to do this, see:
    #   https://azure.microsoft.com/en-us/documentation/articles/resource-group-authenticate-service-principal
    application-id: 00000000-0000-0000-0000-000000000000

    # application-password is the password specified when creating the
    # application in Azure Active Directory.
    application-password: XXX

    # subscription-id defines the Azure account subscription ID to
    # manage resources in. You can list your account subscriptions
    # with the Azure CLI's "account list" action: "azure account list".
    # The ID associated with each account is the subscription ID.
    subscription-id: 00000000-0000-0000-0000-000000000000

    # tenant-id is the ID of the Azure tenant, which identifies the Azure
    # Active Directory instance. You can obtain this ID by using the Azure
    # CLI's "account show" action. First list your accounts with
    # "azure account list", and then feed the account ID to
    # "azure account show" to obtain the properties of the account, including
    # the tenant ID.
    tenant-id: 00000000-0000-0000-0000-000000000000

    # storage-account-type specifies the type of the storage account,
    # which defines the replication strategy and support for different
    # disk types.
    storage-account-type: Standard_LRS

    # location specifies the Azure data center ("location") where
    # instances will be started, for example: West US, North Europe.
    location: West US

    # image-stream chooses an stream from which to select OS images. This
    # can be "released" (default), or "daily".
    #
    # image-stream: "released"

    # agent-stream chooses a simplestreams stream from which to select tools,
    # for example released or proposed tools (or any other stream available
    # on simplestreams).
    #
    # agent-stream: "released"

    # Whether or not to refresh the list of available updates for an
    # OS. The default option of true is recommended for use in
    # production systems, but disabling this can speed up local
    # deployments for development or testing.
    #
    # enable-os-refresh-update: true

    # Whether or not to perform OS upgrades when machines are
    # provisioned. The default option of true is recommended for use
    # in production systems, but disabling this can speed up local
    # deployments for development or testing.
    #
    # enable-os-upgrade: true

`[1:]
