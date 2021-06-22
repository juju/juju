// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/Azure/go-autorest/autorest"
	"github.com/juju/errors"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/provider/azure/internal/azureauth"
	"github.com/juju/juju/provider/azure/internal/azurecli"
)

const (
	credAttrAppId          = "application-id"
	credAttrSubscriptionId = "subscription-id"
	credAttrAppPassword    = "application-password"

	// clientCredentialsAuthType is the auth-type for the
	// "client credentials" OAuth flow, which requires a
	// service principal with a password.
	clientCredentialsAuthType cloud.AuthType = "service-principal-secret"

	// deviceCodeAuthType is the auth-type for the interactive
	// "device code" OAuth flow.
	deviceCodeAuthType cloud.AuthType = cloud.InteractiveAuthType
)

type ServicePrincipalCreator interface {
	InteractiveCreate(sdkCtx context.Context, stderr io.Writer, params azureauth.ServicePrincipalParams) (appid, password string, _ error)
	Create(sdkCtx context.Context, params azureauth.ServicePrincipalParams) (appid, password string, _ error)
}

type AzureCLI interface {
	ListAccounts() ([]azurecli.Account, error)
	FindAccountsWithCloudName(name string) ([]azurecli.Account, error)
	ShowAccount(subscription string) (*azurecli.Account, error)
	GetAccessToken(subscription, resource string) (*azurecli.AccessToken, error)
	FindCloudsWithResourceManagerEndpoint(url string) ([]azurecli.Cloud, error)
	ListClouds() ([]azurecli.Cloud, error)
}

// environPoviderCredentials is an implementation of
// environs.ProviderCredentials for the Azure Resource
// Manager cloud provider.
type environProviderCredentials struct {
	servicePrincipalCreator ServicePrincipalCreator
	azureCLI                AzureCLI
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
				credAttrSubscriptionId, cloud.CredentialAttr{Description: "Azure subscription ID"},
			}, {
				credAttrAppPassword, cloud.CredentialAttr{
					Description: "Azure Active Directory application password",
					Hidden:      true,
				},
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
		cred, err := c.accountCredential(acc, cloudInfo)
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
		if subscriptionId != "" {
			// If a subscription ID was specified then fall
			// back to the interactive device login. attempt
			// to get subscription details from Azure CLI.
			graphResourceId := azureauth.TokenResource(args.CloudIdentityEndpoint)
			resourceManagerResourceId, err := azureauth.ResourceManagerResourceId(args.CloudStorageEndpoint)
			if err != nil {
				return nil, errors.Trace(err)
			}
			return c.deviceCodeCredential(ctx, args, azureauth.ServicePrincipalParams{
				GraphEndpoint:             args.CloudIdentityEndpoint,
				GraphResourceId:           graphResourceId,
				ResourceManagerEndpoint:   args.CloudEndpoint,
				ResourceManagerResourceId: resourceManagerResourceId,
				SubscriptionId:            subscriptionId,
			})
		}
		params, err := c.getServicePrincipalParams(args.CloudEndpoint)
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
	applicationId, password, err := c.servicePrincipalCreator.InteractiveCreate(sdkCtx, ctx.GetStderr(), params)
	if err != nil {
		return nil, errors.Trace(err)
	}
	out := cloud.NewCredential(clientCredentialsAuthType, map[string]string{
		credAttrSubscriptionId: params.SubscriptionId,
		credAttrAppId:          applicationId,
		credAttrAppPassword:    password,
	})
	out.Label = args.Credential.Label
	return &out, nil
}

func (c environProviderCredentials) azureCLICredential(
	ctx environs.FinalizeCredentialContext,
	args environs.FinalizeCredentialParams,
	params azureauth.ServicePrincipalParams,
) (*cloud.Credential, error) {
	graphToken, err := c.azureCLI.GetAccessToken(params.SubscriptionId, params.GraphResourceId)
	if err != nil {
		// The version of Azure CLI may not support
		// get-access-token so fallback to using device
		// authentication.
		logger.Debugf("error getting access token: %s", err)
		return c.deviceCodeCredential(ctx, args, params)
	}
	params.GraphAuthorizer = autorest.NewBearerAuthorizer(graphToken.Token())

	resourceManagerAuthorizer, err := c.azureCLI.GetAccessToken(params.SubscriptionId, params.ResourceManagerResourceId)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot get access token for %s", params.SubscriptionId)
	}
	params.ResourceManagerAuthorizer = autorest.NewBearerAuthorizer(resourceManagerAuthorizer.Token())

	sdkCtx := context.Background()
	applicationId, password, err := c.servicePrincipalCreator.Create(sdkCtx, params)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot get service principal")
	}
	out := cloud.NewCredential(clientCredentialsAuthType, map[string]string{
		credAttrSubscriptionId: params.SubscriptionId,
		credAttrAppId:          applicationId,
		credAttrAppPassword:    password,
	})
	out.Label = args.Credential.Label
	return &out, nil
}

func (c environProviderCredentials) accountCredential(
	acc azurecli.Account,
	cloudInfo azurecli.Cloud,
) (cloud.Credential, error) {
	graphToken, err := c.azureCLI.GetAccessToken(acc.ID, cloudInfo.Endpoints.ActiveDirectoryGraphResourceID)
	if err != nil {
		return cloud.Credential{}, errors.Annotatef(err, "cannot get access token for %s", acc.ID)
	}
	armToken, err := c.azureCLI.GetAccessToken(acc.ID, cloudInfo.Endpoints.ResourceManager)
	if err != nil {
		return cloud.Credential{}, errors.Annotatef(err, "cannot get access token for %s", acc.ID)
	}
	sdkCtx := context.Background()
	applicationId, password, err := c.servicePrincipalCreator.Create(sdkCtx, azureauth.ServicePrincipalParams{
		GraphEndpoint:             cloudInfo.Endpoints.ActiveDirectoryGraphResourceID,
		GraphResourceId:           cloudInfo.Endpoints.ActiveDirectoryGraphResourceID,
		GraphAuthorizer:           autorest.NewBearerAuthorizer(graphToken.Token()),
		ResourceManagerEndpoint:   cloudInfo.Endpoints.ResourceManager,
		ResourceManagerResourceId: cloudInfo.Endpoints.ResourceManager,
		ResourceManagerAuthorizer: autorest.NewBearerAuthorizer(armToken.Token()),
		SubscriptionId:            acc.ID,
		TenantId:                  graphToken.Tenant,
	})
	if err != nil {
		return cloud.Credential{}, errors.Annotate(err, "cannot get service principal")
	}

	return cloud.NewCredential(clientCredentialsAuthType, map[string]string{
		credAttrSubscriptionId: acc.ID,
		credAttrAppId:          applicationId,
		credAttrAppPassword:    password,
	}), nil
}

func (c environProviderCredentials) getServicePrincipalParams(cloudEndpoint string) (azureauth.ServicePrincipalParams, error) {
	if !strings.HasSuffix(cloudEndpoint, "/") {
		cloudEndpoint += "/"
	}
	clouds, err := c.azureCLI.FindCloudsWithResourceManagerEndpoint(cloudEndpoint)
	if err != nil {
		return azureauth.ServicePrincipalParams{}, errors.Annotatef(err, "cannot list clouds")
	}
	if len(clouds) != 1 {
		return azureauth.ServicePrincipalParams{}, errors.Errorf("cannot find cloud for %s", cloudEndpoint)
	}
	accounts, err := c.azureCLI.FindAccountsWithCloudName(clouds[0].Name)
	if err != nil {
		return azureauth.ServicePrincipalParams{}, errors.Annotatef(err, "cannot get accounts")
	}
	if len(accounts) < 1 {
		return azureauth.ServicePrincipalParams{}, errors.Errorf("no %s accounts found", clouds[0].Name)
	}
	acc := accounts[0]
	for _, a := range accounts[1:] {
		if a.IsDefault {
			acc = a
		}
	}
	return azureauth.ServicePrincipalParams{
		GraphEndpoint:             clouds[0].Endpoints.ActiveDirectoryGraphResourceID,
		GraphResourceId:           clouds[0].Endpoints.ActiveDirectoryGraphResourceID,
		ResourceManagerEndpoint:   clouds[0].Endpoints.ResourceManager,
		ResourceManagerResourceId: clouds[0].Endpoints.ResourceManager,
		SubscriptionId:            acc.ID,
		TenantId:                  acc.TenantId,
	}, nil

}
