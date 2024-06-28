// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"context"
	"fmt"
	"io"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/juju/errors"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/internal/provider/azure/internal/azureauth"
	"github.com/juju/juju/internal/provider/azure/internal/azurecli"
)

const (
	credAttrAppId                 = "application-id"
	credAttrApplicationObjectId   = "application-object-id"
	credAttrSubscriptionId        = "subscription-id"
	credAttrManagedSubscriptionId = "managed-subscription-id"
	credAttrAppPassword           = "application-password"
	credManagedIdentity           = "managed-identity"

	// clientCredentialsAuthType is the auth-type for the
	// "client credentials" OAuth flow, which requires a
	// service principal with a password.
	clientCredentialsAuthType cloud.AuthType = "service-principal-secret"

	// deviceCodeAuthType is the auth-type for the interactive
	// "device code" OAuth flow.
	deviceCodeAuthType cloud.AuthType = cloud.InteractiveAuthType
)

type ServicePrincipalCreator interface {
	InteractiveCreate(sdkCtx context.Context, stderr io.Writer, params azureauth.ServicePrincipalParams) (appid, spid, password string, _ error)
	Create(sdkCtx context.Context, params azureauth.ServicePrincipalParams) (appid, spid, password string, _ error)
}

type AzureCLI interface {
	ListAccounts() ([]azurecli.Account, error)
	FindAccountsWithCloudName(name string) ([]azurecli.Account, error)
	ShowAccount(subscription string) (*azurecli.Account, error)
	ListClouds() ([]azurecli.Cloud, error)
}

// environPoviderCredentials is an implementation of
// environs.ProviderCredentials for the Azure Resource
// Manager cloud provider.
type environProviderCredentials struct {
	servicePrincipalCreator ServicePrincipalCreator
	azureCLI                AzureCLI
	transporter             policy.Transporter
}

// CredentialSchemas is part of the environs.ProviderCredentials interface.
func (c environProviderCredentials) CredentialSchemas() map[cloud.AuthType]cloud.CredentialSchema {
	interactiveSchema := cloud.CredentialSchema{{
		credAttrSubscriptionId, cloud.CredentialAttr{Description: "Azure subscription ID"},
	}}
	if _, err := c.azureCLI.ShowAccount(""); err == nil {
		// If az account show returns successfully then we can
		// use that to get at least some login details, otherwise
		// we need the user to supply their subscription ID.
		interactiveSchema[0].CredentialAttr.Optional = true
	}
	return map[cloud.AuthType]cloud.CredentialSchema{
		// deviceCodeAuthType is the interactive device-code oauth
		// flow. This is only supported on the client side; it will
		// be used to generate a service principal, and transformed
		// into clientCredentialsAuthType.
		deviceCodeAuthType: interactiveSchema,

		// clientCredentialsAuthType is the "client credentials"
		// oauth flow, which requires a service principal with a
		// password.
		clientCredentialsAuthType: {
			{
				credAttrAppId, cloud.CredentialAttr{Description: "Azure Active Directory application ID"},
			}, {
				credAttrApplicationObjectId, cloud.CredentialAttr{
					Description: "Azure Active Directory application Object ID",
					Optional:    true,
				},
			}, {
				credAttrSubscriptionId, cloud.CredentialAttr{Description: "Azure subscription ID"},
			}, {
				credAttrManagedSubscriptionId, cloud.CredentialAttr{
					Description: "Azure managed subscription ID",
					Optional:    true,
				},
			}, {
				credAttrAppPassword, cloud.CredentialAttr{
					Description: "Azure Active Directory application password",
					Hidden:      true,
				},
			},
		},
		// InstanceRoleAuthType is an authentication type used by sourcing
		// credentials from within the machine's context in a given cloud provider.
		// You only get these credentials by running within that machine.
		cloud.InstanceRoleAuthType: {
			{
				credManagedIdentity,
				cloud.CredentialAttr{
					Description: "The Azure Managed Identity ID",
				},
			}, {
				credAttrSubscriptionId, cloud.CredentialAttr{Description: "Azure subscription ID"},
			},
		},
	}
}

// DetectCredentials is part of the environs.ProviderCredentials
// interface. It attempts to detect subscription IDs from accounts
// configured in the Azure CLI.
func (c environProviderCredentials) DetectCredentials(cloudName string) (*cloud.CloudCredential, error) {
	// Attempt to get accounts from az.
	accounts, err := c.azureCLI.ListAccounts()
	if err != nil {
		logger.Debugf("error getting accounts from az: %s", err)
		return nil, errors.NotFoundf("credentials")
	}
	if len(accounts) < 1 {
		return nil, errors.NotFoundf("credentials")
	}
	clouds, err := c.azureCLI.ListClouds()
	if err != nil {
		logger.Debugf("error getting clouds from az: %s", err)
		return nil, errors.NotFoundf("credentials")
	}
	cloudMap := make(map[string]azurecli.Cloud, len(clouds))
	for _, cloud := range clouds {
		cloudMap[cloud.Name] = cloud
	}
	var defaultCredential string
	authCredentials := make(map[string]cloud.Credential)
	for _, acc := range accounts {
		cloudInfo, ok := cloudMap[acc.CloudName]
		if !ok {
			continue
		}
		cred, err := c.accountCredential(acc)
		if err != nil {
			logger.Debugf("cannot get credential for %s: %s", acc.Name, err)
			continue
		}
		cred.Label = fmt.Sprintf("%s subscription %s", cloudInfo.Name, acc.Name)
		authCredentials[acc.Name] = cred
		if acc.IsDefault {
			defaultCredential = acc.Name
		}
	}
	if len(authCredentials) < 1 {
		return nil, errors.NotFoundf("credentials")
	}
	return &cloud.CloudCredential{
		DefaultCredential: defaultCredential,
		AuthCredentials:   authCredentials,
	}, nil
}

// FinalizeCredential is part of the environs.ProviderCredentials interface.
func (c environProviderCredentials) FinalizeCredential(
	ctx environs.FinalizeCredentialContext,
	args environs.FinalizeCredentialParams,
) (*cloud.Credential, error) {
	switch authType := args.Credential.AuthType(); authType {
	case deviceCodeAuthType:
		subscriptionId := args.Credential.Attributes()[credAttrSubscriptionId]

		var azCloudName string
		switch args.CloudName {
		case "azure":
			azCloudName = azureauth.AzureCloud
		case "azure-china":
			azCloudName = azureauth.AzureChinaCloud
		case "azure-gov":
			azCloudName = azureauth.AzureUSGovernment
		default:
			return nil, errors.Errorf("unknown Azure cloud name %q", args.CloudName)
		}

		if subscriptionId != "" {
			opts := azcore.ClientOptions{
				Cloud:     azureCloud(args.CloudName, args.CloudEndpoint, args.CloudIdentityEndpoint),
				Transport: c.transporter,
			}
			clientOpts := arm.ClientOptions{ClientOptions: opts}
			sdkCtx := context.Background()
			tenantID, err := azureauth.DiscoverTenantID(sdkCtx, subscriptionId, clientOpts)
			if err != nil {
				return nil, errors.Trace(err)
			}
			return c.deviceCodeCredential(ctx, args, azureauth.ServicePrincipalParams{
				CloudName:      azCloudName,
				SubscriptionId: subscriptionId,
				TenantId:       tenantID,
			})
		}

		params, err := c.getServicePrincipalParams(azCloudName)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return c.azureCLICredential(ctx, args, params)
	case clientCredentialsAuthType:
		return &args.Credential, nil
	default:
		return nil, errors.NotSupportedf("%q auth-type", authType)
	}
}

func (c environProviderCredentials) deviceCodeCredential(
	ctx environs.FinalizeCredentialContext,
	args environs.FinalizeCredentialParams,
	params azureauth.ServicePrincipalParams,
) (*cloud.Credential, error) {
	sdkCtx := context.Background()
	applicationId, spObjectId, password, err := c.servicePrincipalCreator.InteractiveCreate(sdkCtx, ctx.GetStderr(), params)
	if err != nil {
		return nil, errors.Trace(err)
	}
	out := cloud.NewCredential(clientCredentialsAuthType, map[string]string{
		credAttrSubscriptionId:      params.SubscriptionId,
		credAttrAppId:               applicationId,
		credAttrApplicationObjectId: spObjectId,
		credAttrAppPassword:         password,
	})
	out.Label = args.Credential.Label
	return &out, nil
}

func (c environProviderCredentials) azureCLICredential(
	ctx environs.FinalizeCredentialContext,
	args environs.FinalizeCredentialParams,
	params azureauth.ServicePrincipalParams,
) (*cloud.Credential, error) {
	cred, err := azidentity.NewAzureCLICredential(&azidentity.AzureCLICredentialOptions{
		TenantID: params.TenantId,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	params.Credential = cred

	if err != nil {
		return nil, errors.Annotatef(err, "cannot get access token for %s", params.SubscriptionId)
	}

	sdkCtx := context.Background()
	applicationId, spObjectId, password, err := c.servicePrincipalCreator.Create(sdkCtx, params)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot create service principal")
	}
	out := cloud.NewCredential(clientCredentialsAuthType, map[string]string{
		credAttrSubscriptionId:      params.SubscriptionId,
		credAttrAppId:               applicationId,
		credAttrApplicationObjectId: spObjectId,
		credAttrAppPassword:         password,
	})
	out.Label = args.Credential.Label
	return &out, nil
}

func (c environProviderCredentials) accountCredential(
	acc azurecli.Account,
) (cloud.Credential, error) {
	cred, err := azidentity.NewAzureCLICredential(&azidentity.AzureCLICredentialOptions{
		TenantID:                   acc.AuthTenantId(),
		AdditionallyAllowedTenants: []string{"*"},
	})
	if err != nil {
		return cloud.Credential{}, errors.Annotate(err, "cannot get az cli credential")
	}
	sdkCtx := context.Background()
	applicationId, spObjectId, password, err := c.servicePrincipalCreator.Create(sdkCtx, azureauth.ServicePrincipalParams{
		Credential:     cred,
		SubscriptionId: acc.ID,
		TenantId:       acc.TenantId,
	})
	if err != nil {
		return cloud.Credential{}, errors.Annotate(err, "cannot get service principal")
	}

	return cloud.NewCredential(clientCredentialsAuthType, map[string]string{
		credAttrSubscriptionId:      acc.ID,
		credAttrAppId:               applicationId,
		credAttrApplicationObjectId: spObjectId,
		credAttrAppPassword:         password,
	}), nil
}

func (c environProviderCredentials) getServicePrincipalParams(cloudName string) (azureauth.ServicePrincipalParams, error) {
	accounts, err := c.azureCLI.FindAccountsWithCloudName(cloudName)
	if err != nil {
		return azureauth.ServicePrincipalParams{}, errors.Annotatef(err, "cannot get accounts")
	}
	if len(accounts) < 1 {
		return azureauth.ServicePrincipalParams{}, errors.Errorf("no %s accounts found", cloudName)
	}
	acc := accounts[0]
	for _, a := range accounts[1:] {
		if a.IsDefault {
			acc = a
		}
	}
	return azureauth.ServicePrincipalParams{
		CloudName:      cloudName,
		SubscriptionId: acc.ID,
		TenantId:       acc.AuthTenantId(),
	}, nil

}
