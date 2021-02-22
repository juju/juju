// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure_test

import (
	"context"
	"fmt"
	"io"

	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/adal"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/provider/azure"
	"github.com/juju/juju/provider/azure/internal/azureauth"
	"github.com/juju/juju/provider/azure/internal/azurecli"
)

type credentialsSuite struct {
	testing.IsolationSuite
	servicePrincipalCreator servicePrincipalCreator
	azureCLI                azureCLI
	provider                environs.EnvironProvider
}

var _ = gc.Suite(&credentialsSuite{})

func (s *credentialsSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.servicePrincipalCreator = servicePrincipalCreator{}
	s.azureCLI = azureCLI{}
	s.provider = newProvider(c, azure.ProviderConfig{
		ServicePrincipalCreator: &s.servicePrincipalCreator,
		AzureCLI:                &s.azureCLI,
	})
}

func (s *credentialsSuite) TestCredentialSchemas(c *gc.C) {
	envtesting.AssertProviderAuthTypes(c, s.provider,
		"interactive",
		"service-principal-secret",
	)
}

func (s *credentialsSuite) TestServicePrincipalSecretCredentialsValid(c *gc.C) {
	envtesting.AssertProviderCredentialsValid(c, s.provider, "service-principal-secret", map[string]string{
		"application-id":       "application",
		"application-password": "password",
		"subscription-id":      "subscription",
	})
}

func (s *credentialsSuite) TestServicePrincipalSecretHiddenAttributes(c *gc.C) {
	envtesting.AssertProviderCredentialsAttributesHidden(c, s.provider, "service-principal-secret", "application-password")
}

func (s *credentialsSuite) TestDetectCredentialsNoAccounts(c *gc.C) {
	_, err := s.provider.DetectCredentials()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	calls := s.azureCLI.Calls()
	c.Assert(calls, gc.HasLen, 1)
	c.Assert(calls[0].FuncName, gc.Equals, "ListAccounts")
}

func (s *credentialsSuite) TestDetectCredentialsListError(c *gc.C) {
	s.azureCLI.SetErrors(errors.New("test error"))
	_, err := s.provider.DetectCredentials()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *credentialsSuite) TestDetectCredentialsOneAccount(c *gc.C) {
	s.azureCLI.Accounts = []azurecli.Account{{
		CloudName: "AzureCloud",
		ID:        "test-account-id",
		IsDefault: true,
		Name:      "test-account",
		State:     "Enabled",
		TenantId:  "tenant-id",
	}}
	s.azureCLI.Clouds = []azurecli.Cloud{{
		Endpoints: azurecli.CloudEndpoints{
			ActiveDirectoryGraphResourceID: "https://graph.invalid/",
			ResourceManager:                "https://arm.invalid/",
		},
		IsActive: true,
		Name:     "AzureCloud",
	}}
	cred, err := s.provider.DetectCredentials()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cred, gc.Not(gc.IsNil))
	c.Assert(cred.DefaultCredential, gc.Equals, "test-account")
	c.Assert(cred.DefaultRegion, gc.Equals, "")
	c.Assert(cred.AuthCredentials, gc.HasLen, 1)
	c.Assert(cred.AuthCredentials["test-account"].Label, gc.Equals, "AzureCloud subscription test-account")

	calls := s.azureCLI.Calls()
	c.Assert(calls, gc.HasLen, 4)
	c.Assert(calls[0].FuncName, gc.Equals, "ListAccounts")
	c.Assert(calls[1].FuncName, gc.Equals, "ListClouds")
	c.Assert(calls[2].FuncName, gc.Equals, "GetAccessToken")
	c.Assert(calls[2].Args, jc.DeepEquals, []interface{}{"test-account-id", "https://graph.invalid/"})
	c.Assert(calls[3].FuncName, gc.Equals, "GetAccessToken")
	c.Assert(calls[3].Args, jc.DeepEquals, []interface{}{"test-account-id", "https://arm.invalid/"})

	calls = s.servicePrincipalCreator.Calls()
	c.Assert(calls, gc.HasLen, 1)
	c.Assert(calls[0].FuncName, gc.Equals, "Create")
	c.Assert(calls[0].Args[1], jc.DeepEquals, azureauth.ServicePrincipalParams{
		GraphEndpoint:   "https://graph.invalid/",
		GraphResourceId: "https://graph.invalid/",
		GraphAuthorizer: autorest.NewBearerAuthorizer(&adal.Token{
			AccessToken: "test-account-id|https://graph.invalid/|access-token",
			Type:        "Bearer",
		}),
		ResourceManagerEndpoint:   "https://arm.invalid/",
		ResourceManagerResourceId: "https://arm.invalid/",
		ResourceManagerAuthorizer: autorest.NewBearerAuthorizer(&adal.Token{
			AccessToken: "test-account-id|https://arm.invalid/|access-token",
			Type:        "Bearer",
		}),
		SubscriptionId: "test-account-id",
		TenantId:       "tenant-id",
	})
}

func (s *credentialsSuite) TestDetectCredentialsCloudError(c *gc.C) {
	s.azureCLI.Accounts = []azurecli.Account{{
		CloudName: "AzureCloud",
		ID:        "test-account-id",
		IsDefault: true,
		Name:      "test-account",
		State:     "Enabled",
		TenantId:  "tenant-id",
	}}
	s.azureCLI.Clouds = []azurecli.Cloud{{
		Endpoints: azurecli.CloudEndpoints{
			ActiveDirectoryGraphResourceID: "https://graph.invalid",
			ResourceManager:                "https://arm.invalid",
		},
		IsActive: true,
		Name:     "AzureCloud",
	}}
	s.azureCLI.SetErrors(nil, errors.New("test error"))
	cred, err := s.provider.DetectCredentials()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(cred, gc.IsNil)

	calls := s.azureCLI.Calls()
	c.Assert(calls, gc.HasLen, 2)
	c.Assert(calls[0].FuncName, gc.Equals, "ListAccounts")
	c.Assert(calls[1].FuncName, gc.Equals, "ListClouds")

	calls = s.servicePrincipalCreator.Calls()
	c.Assert(calls, gc.HasLen, 0)
}

func (s *credentialsSuite) TestDetectCredentialsTwoAccounts(c *gc.C) {
	s.azureCLI.Accounts = []azurecli.Account{{
		CloudName: "AzureCloud",
		ID:        "test-account1-id",
		IsDefault: true,
		Name:      "test-account1",
		State:     "Enabled",
		TenantId:  "tenant-id",
	}, {
		CloudName: "AzureCloud",
		ID:        "test-account2-id",
		IsDefault: false,
		Name:      "test-account2",
		State:     "Enabled",
		TenantId:  "tenant-id",
	}}
	s.azureCLI.Clouds = []azurecli.Cloud{{
		Endpoints: azurecli.CloudEndpoints{
			ActiveDirectoryGraphResourceID: "https://graph.invalid/",
			ResourceManager:                "https://arm.invalid/",
		},
		IsActive: true,
		Name:     "AzureCloud",
	}}
	cred, err := s.provider.DetectCredentials()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cred, gc.Not(gc.IsNil))
	c.Assert(cred.DefaultCredential, gc.Equals, "test-account1")
	c.Assert(cred.DefaultRegion, gc.Equals, "")
	c.Assert(cred.AuthCredentials, gc.HasLen, 2)
	c.Assert(cred.AuthCredentials["test-account1"].Label, gc.Equals, "AzureCloud subscription test-account1")
	c.Assert(cred.AuthCredentials["test-account2"].Label, gc.Equals, "AzureCloud subscription test-account2")

	calls := s.azureCLI.Calls()
	c.Assert(calls, gc.HasLen, 6)
	c.Assert(calls[0].FuncName, gc.Equals, "ListAccounts")
	c.Assert(calls[1].FuncName, gc.Equals, "ListClouds")
	c.Assert(calls[2].FuncName, gc.Equals, "GetAccessToken")
	c.Assert(calls[2].Args, jc.DeepEquals, []interface{}{"test-account1-id", "https://graph.invalid/"})
	c.Assert(calls[3].FuncName, gc.Equals, "GetAccessToken")
	c.Assert(calls[3].Args, jc.DeepEquals, []interface{}{"test-account1-id", "https://arm.invalid/"})
	c.Assert(calls[4].FuncName, gc.Equals, "GetAccessToken")
	c.Assert(calls[4].Args, jc.DeepEquals, []interface{}{"test-account2-id", "https://graph.invalid/"})
	c.Assert(calls[5].FuncName, gc.Equals, "GetAccessToken")
	c.Assert(calls[5].Args, jc.DeepEquals, []interface{}{"test-account2-id", "https://arm.invalid/"})

	calls = s.servicePrincipalCreator.Calls()
	c.Assert(calls, gc.HasLen, 2)
	c.Assert(calls[0].FuncName, gc.Equals, "Create")
	c.Assert(calls[0].Args[1], jc.DeepEquals, azureauth.ServicePrincipalParams{
		GraphEndpoint:   "https://graph.invalid/",
		GraphResourceId: "https://graph.invalid/",
		GraphAuthorizer: autorest.NewBearerAuthorizer(&adal.Token{
			AccessToken: "test-account1-id|https://graph.invalid/|access-token",
			Type:        "Bearer",
		}),
		ResourceManagerEndpoint:   "https://arm.invalid/",
		ResourceManagerResourceId: "https://arm.invalid/",
		ResourceManagerAuthorizer: autorest.NewBearerAuthorizer(&adal.Token{
			AccessToken: "test-account1-id|https://arm.invalid/|access-token",
			Type:        "Bearer",
		}),
		SubscriptionId: "test-account1-id",
		TenantId:       "tenant-id",
	})
	c.Assert(calls[1].FuncName, gc.Equals, "Create")
	c.Assert(calls[1].Args[1], jc.DeepEquals, azureauth.ServicePrincipalParams{
		GraphEndpoint:   "https://graph.invalid/",
		GraphResourceId: "https://graph.invalid/",
		GraphAuthorizer: autorest.NewBearerAuthorizer(&adal.Token{
			AccessToken: "test-account2-id|https://graph.invalid/|access-token",
			Type:        "Bearer",
		}),
		ResourceManagerEndpoint:   "https://arm.invalid/",
		ResourceManagerResourceId: "https://arm.invalid/",
		ResourceManagerAuthorizer: autorest.NewBearerAuthorizer(&adal.Token{
			AccessToken: "test-account2-id|https://arm.invalid/|access-token",
			Type:        "Bearer",
		}),
		SubscriptionId: "test-account2-id",
		TenantId:       "tenant-id",
	})
}

func (s *credentialsSuite) TestDetectCredentialsTwoAccountsOneError(c *gc.C) {
	s.azureCLI.Accounts = []azurecli.Account{{
		CloudName: "AzureCloud",
		ID:        "test-account1-id",
		IsDefault: false,
		Name:      "test-account1",
		State:     "Enabled",
		TenantId:  "tenant-id",
	}, {
		CloudName: "AzureCloud",
		ID:        "test-account2-id",
		IsDefault: true,
		Name:      "test-account2",
		State:     "Enabled",
		TenantId:  "tenant-id",
	}}
	s.azureCLI.Clouds = []azurecli.Cloud{{
		Endpoints: azurecli.CloudEndpoints{
			ActiveDirectoryGraphResourceID: "https://graph.invalid/",
			ResourceManager:                "https://arm.invalid/",
		},
		IsActive: true,
		Name:     "AzureCloud",
	}}
	s.azureCLI.SetErrors(nil, nil, nil, nil, errors.New("test error"))
	cred, err := s.provider.DetectCredentials()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cred, gc.Not(gc.IsNil))
	c.Assert(cred.DefaultCredential, gc.Equals, "")
	c.Assert(cred.DefaultRegion, gc.Equals, "")
	c.Assert(cred.AuthCredentials, gc.HasLen, 1)
	c.Assert(cred.AuthCredentials["test-account1"].Label, gc.Equals, "AzureCloud subscription test-account1")

	calls := s.azureCLI.Calls()
	c.Assert(calls, gc.HasLen, 5)
	c.Assert(calls[0].FuncName, gc.Equals, "ListAccounts")
	c.Assert(calls[1].FuncName, gc.Equals, "ListClouds")
	c.Assert(calls[2].FuncName, gc.Equals, "GetAccessToken")
	c.Assert(calls[2].Args, jc.DeepEquals, []interface{}{"test-account1-id", "https://graph.invalid/"})
	c.Assert(calls[3].FuncName, gc.Equals, "GetAccessToken")
	c.Assert(calls[3].Args, jc.DeepEquals, []interface{}{"test-account1-id", "https://arm.invalid/"})
	c.Assert(calls[4].FuncName, gc.Equals, "GetAccessToken")
	c.Assert(calls[4].Args, jc.DeepEquals, []interface{}{"test-account2-id", "https://graph.invalid/"})

	calls = s.servicePrincipalCreator.Calls()
	c.Assert(calls, gc.HasLen, 1)
	c.Assert(calls[0].FuncName, gc.Equals, "Create")
	c.Assert(calls[0].Args[1], jc.DeepEquals, azureauth.ServicePrincipalParams{
		GraphEndpoint:   "https://graph.invalid/",
		GraphResourceId: "https://graph.invalid/",
		GraphAuthorizer: autorest.NewBearerAuthorizer(&adal.Token{
			AccessToken: "test-account1-id|https://graph.invalid/|access-token",
			Type:        "Bearer",
		}),
		ResourceManagerEndpoint:   "https://arm.invalid/",
		ResourceManagerResourceId: "https://arm.invalid/",
		ResourceManagerAuthorizer: autorest.NewBearerAuthorizer(&adal.Token{
			AccessToken: "test-account1-id|https://arm.invalid/|access-token",
			Type:        "Bearer",
		}),
		SubscriptionId: "test-account1-id",
		TenantId:       "tenant-id",
	})
}

func (s *credentialsSuite) TestFinalizeCredentialInteractive(c *gc.C) {
	in := cloud.NewCredential("interactive", map[string]string{"subscription-id": "subscription"})
	ctx := cmdtesting.Context(c)
	out, err := s.provider.FinalizeCredential(ctx, environs.FinalizeCredentialParams{
		Credential:            in,
		CloudEndpoint:         "https://arm.invalid",
		CloudStorageEndpoint:  "https://core.invalid",
		CloudIdentityEndpoint: "https://graph.invalid",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out, gc.NotNil)
	c.Assert(out.AuthType(), gc.Equals, cloud.AuthType("service-principal-secret"))
	c.Assert(out.Attributes(), jc.DeepEquals, map[string]string{
		"application-id":       "appid",
		"application-password": "service-principal-password",
		"subscription-id":      "subscription",
	})

	s.servicePrincipalCreator.CheckCallNames(c, "InteractiveCreate")
	args := s.servicePrincipalCreator.Calls()[0].Args
	c.Assert(args[2], jc.DeepEquals, azureauth.ServicePrincipalParams{
		GraphEndpoint:             "https://graph.invalid",
		GraphResourceId:           "https://graph.invalid/",
		ResourceManagerEndpoint:   "https://arm.invalid",
		ResourceManagerResourceId: "https://management.core.invalid/",
		SubscriptionId:            "subscription",
	})
}

func (s *credentialsSuite) TestFinalizeCredentialInteractiveError(c *gc.C) {
	in := cloud.NewCredential("interactive", map[string]string{"subscription-id": "subscription"})
	s.servicePrincipalCreator.SetErrors(errors.New("blargh"))
	ctx := cmdtesting.Context(c)
	_, err := s.provider.FinalizeCredential(ctx, environs.FinalizeCredentialParams{
		Credential:            in,
		CloudEndpoint:         "https://arm.invalid",
		CloudIdentityEndpoint: "https://graph.invalid",
	})
	c.Assert(err, gc.ErrorMatches, "blargh")
}

func (s *credentialsSuite) TestFinalizeCredentialAzureCLI(c *gc.C) {
	s.azureCLI.Accounts = []azurecli.Account{{
		CloudName: "AzureCloud",
		ID:        "test-account1-id",
		IsDefault: true,
		Name:      "test-account1",
		State:     "Enabled",
		TenantId:  "tenant-id",
	}, {
		CloudName: "AzureCloud",
		ID:        "test-account2-id",
		IsDefault: false,
		Name:      "test-account2",
		State:     "Enabled",
		TenantId:  "tenant-id",
	}}
	s.azureCLI.Clouds = []azurecli.Cloud{{
		Endpoints: azurecli.CloudEndpoints{
			ActiveDirectoryGraphResourceID: "https://graph.invalid/",
			ResourceManager:                "https://arm.invalid/",
		},
		IsActive: true,
		Name:     "AzureCloud",
	}}
	in := cloud.NewCredential("interactive", nil)
	ctx := cmdtesting.Context(c)
	cred, err := s.provider.FinalizeCredential(ctx, environs.FinalizeCredentialParams{
		Credential:            in,
		CloudEndpoint:         "https://arm.invalid",
		CloudIdentityEndpoint: "https://graph.invalid",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cred, gc.Not(gc.IsNil))
	c.Assert(cred.AuthType(), gc.Equals, cloud.AuthType("service-principal-secret"))
	attrs := cred.Attributes()
	c.Assert(attrs["subscription-id"], gc.Equals, "test-account1-id")
	c.Assert(attrs["application-id"], gc.Equals, "appid")
	c.Assert(attrs["application-password"], gc.Equals, "service-principal-password")
}

func (s *credentialsSuite) TestFinalizeCredentialAzureCLIShowAccountError(c *gc.C) {
	s.azureCLI.Accounts = []azurecli.Account{{
		CloudName: "AzureCloud",
		ID:        "test-account1-id",
		IsDefault: true,
		Name:      "test-account1",
		State:     "Enabled",
		TenantId:  "tenant-id",
	}, {
		CloudName: "AzureCloud",
		ID:        "test-account2-id",
		IsDefault: false,
		Name:      "test-account2",
		State:     "Enabled",
		TenantId:  "tenant-id",
	}}
	s.azureCLI.Clouds = []azurecli.Cloud{{
		Endpoints: azurecli.CloudEndpoints{
			ActiveDirectoryGraphResourceID: "https://graph.invalid/",
			ResourceManager:                "https://arm.invalid/",
		},
		IsActive: true,
		Name:     "AzureCloud",
	}}
	s.azureCLI.SetErrors(nil, errors.New("test error"))
	in := cloud.NewCredential("interactive", nil)
	ctx := cmdtesting.Context(c)
	cred, err := s.provider.FinalizeCredential(ctx, environs.FinalizeCredentialParams{
		Credential:            in,
		CloudEndpoint:         "https://arm.invalid",
		CloudIdentityEndpoint: "https://graph.invalid",
	})
	c.Assert(err, gc.ErrorMatches, `cannot get accounts: test error`)
	c.Assert(cred, gc.IsNil)
}

func (s *credentialsSuite) TestFinalizeCredentialAzureCLIGraphTokenError(c *gc.C) {
	s.azureCLI.Accounts = []azurecli.Account{{
		CloudName: "AzureCloud",
		ID:        "test-account1-id",
		IsDefault: true,
		Name:      "test-account1",
		State:     "Enabled",
		TenantId:  "tenant-id",
	}, {
		CloudName: "AzureCloud",
		ID:        "test-account2-id",
		IsDefault: false,
		Name:      "test-account2",
		State:     "Enabled",
		TenantId:  "tenant-id",
	}}
	s.azureCLI.Clouds = []azurecli.Cloud{{
		Endpoints: azurecli.CloudEndpoints{
			ActiveDirectoryGraphResourceID: "https://graph.invalid/",
			ResourceManager:                "https://arm.invalid/",
		},
		IsActive: true,
		Name:     "AzureCloud",
	}}
	s.azureCLI.SetErrors(nil, nil, nil, errors.New("test error"))
	in := cloud.NewCredential("interactive", nil)
	ctx := cmdtesting.Context(c)
	cred, err := s.provider.FinalizeCredential(ctx, environs.FinalizeCredentialParams{
		Credential:            in,
		CloudEndpoint:         "https://arm.invalid",
		CloudIdentityEndpoint: "https://graph.invalid",
	})
	c.Assert(err, gc.ErrorMatches, `cannot get access token for test-account1-id: test error`)
	c.Assert(cred, gc.IsNil)
}

func (s *credentialsSuite) TestFinalizeCredentialAzureCLIResourceManagerTokenError(c *gc.C) {
	s.azureCLI.Accounts = []azurecli.Account{{
		CloudName: "AzureCloud",
		ID:        "test-account1-id",
		IsDefault: true,
		Name:      "test-account1",
		State:     "Enabled",
		TenantId:  "tenant-id",
	}, {
		CloudName: "AzureCloud",
		ID:        "test-account2-id",
		IsDefault: false,
		Name:      "test-account2",
		State:     "Enabled",
		TenantId:  "tenant-id",
	}}
	s.azureCLI.Clouds = []azurecli.Cloud{{
		Endpoints: azurecli.CloudEndpoints{
			ActiveDirectoryGraphResourceID: "https://graph.invalid/",
			ResourceManager:                "https://arm.invalid/",
		},
		IsActive: true,
		Name:     "AzureCloud",
	}}
	s.azureCLI.SetErrors(nil, nil, nil, errors.New("test error"))
	in := cloud.NewCredential("interactive", nil)
	ctx := cmdtesting.Context(c)
	cred, err := s.provider.FinalizeCredential(ctx, environs.FinalizeCredentialParams{
		Credential:            in,
		CloudEndpoint:         "https://arm.invalid",
		CloudIdentityEndpoint: "https://graph.invalid",
	})
	c.Assert(err, gc.ErrorMatches, `cannot get access token for test-account1-id: test error`)
	c.Assert(cred, gc.IsNil)
}

func (s *credentialsSuite) TestFinalizeCredentialAzureCLIServicePrincipalError(c *gc.C) {
	s.azureCLI.Accounts = []azurecli.Account{{
		CloudName: "AzureCloud",
		ID:        "test-account1-id",
		IsDefault: true,
		Name:      "test-account1",
		State:     "Enabled",
		TenantId:  "tenant-id",
	}, {
		CloudName: "AzureCloud",
		ID:        "test-account2-id",
		IsDefault: false,
		Name:      "test-account2",
		State:     "Enabled",
		TenantId:  "tenant-id",
	}}
	s.azureCLI.Clouds = []azurecli.Cloud{{
		Endpoints: azurecli.CloudEndpoints{
			ActiveDirectoryGraphResourceID: "https://graph.invalid/",
			ResourceManager:                "https://arm.invalid/",
		},
		IsActive: true,
		Name:     "AzureCloud",
	}}
	s.servicePrincipalCreator.SetErrors(errors.New("test error"))
	in := cloud.NewCredential("interactive", nil)
	ctx := cmdtesting.Context(c)
	cred, err := s.provider.FinalizeCredential(ctx, environs.FinalizeCredentialParams{
		Credential:            in,
		CloudEndpoint:         "https://arm.invalid",
		CloudIdentityEndpoint: "https://graph.invalid",
	})
	c.Assert(err, gc.ErrorMatches, `cannot get service principal: test error`)
	c.Assert(cred, gc.IsNil)
}

func (s *credentialsSuite) TestFinalizeCredentialAzureCLIDeviceFallback(c *gc.C) {
	s.azureCLI.Accounts = []azurecli.Account{{
		CloudName: "AzureCloud",
		ID:        "test-account1-id",
		IsDefault: true,
		Name:      "test-account1",
		State:     "Enabled",
		TenantId:  "tenant-id",
	}, {
		CloudName: "AzureCloud",
		ID:        "test-account2-id",
		IsDefault: false,
		Name:      "test-account2",
		State:     "Enabled",
		TenantId:  "tenant-id",
	}}
	s.azureCLI.Clouds = []azurecli.Cloud{{
		Endpoints: azurecli.CloudEndpoints{
			ActiveDirectoryGraphResourceID: "https://graph.invalid/",
			ResourceManager:                "https://arm.invalid/",
		},
		IsActive: true,
		Name:     "AzureCloud",
	}}
	s.azureCLI.SetErrors(nil, nil, errors.New("test error"))
	in := cloud.NewCredential("interactive", nil)
	ctx := cmdtesting.Context(c)
	cred, err := s.provider.FinalizeCredential(ctx, environs.FinalizeCredentialParams{
		Credential:            in,
		CloudEndpoint:         "https://arm.invalid",
		CloudIdentityEndpoint: "https://graph.invalid",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cred, gc.Not(gc.IsNil))
	c.Assert(cred.AuthType(), gc.Equals, cloud.AuthType("service-principal-secret"))
	attrs := cred.Attributes()
	c.Assert(attrs["subscription-id"], gc.Equals, "test-account1-id")
	c.Assert(attrs["application-id"], gc.Equals, "appid")
	c.Assert(attrs["application-password"], gc.Equals, "service-principal-password")
	s.servicePrincipalCreator.CheckCallNames(c, "InteractiveCreate")
}

type servicePrincipalCreator struct {
	testing.Stub
}

func (c *servicePrincipalCreator) InteractiveCreate(sdkCtx context.Context, stderr io.Writer, params azureauth.ServicePrincipalParams) (appId, password string, _ error) {
	c.MethodCall(c, "InteractiveCreate", sdkCtx, stderr, params)
	return "appid", "service-principal-password", c.NextErr()
}

func (c *servicePrincipalCreator) Create(sdkCtx context.Context, params azureauth.ServicePrincipalParams) (appId, password string, _ error) {
	c.MethodCall(c, "Create", sdkCtx, params)
	return "appid", "service-principal-password", c.NextErr()
}

type azureCLI struct {
	testing.Stub
	Accounts []azurecli.Account
	Clouds   []azurecli.Cloud
}

func (e *azureCLI) ListAccounts() ([]azurecli.Account, error) {
	e.MethodCall(e, "ListAccounts")
	if err := e.NextErr(); err != nil {
		return nil, err
	}
	return e.Accounts, nil
}

func (e *azureCLI) FindAccountsWithCloudName(name string) ([]azurecli.Account, error) {
	e.MethodCall(e, "FindAccountsWithCloudName", name)
	if err := e.NextErr(); err != nil {
		return nil, err
	}
	var accs []azurecli.Account
	for _, acc := range e.Accounts {
		if acc.CloudName == name {
			accs = append(accs, acc)
		}
	}
	return accs, nil
}

func (e *azureCLI) ShowAccount(subscription string) (*azurecli.Account, error) {
	e.MethodCall(e, "ShowAccount", subscription)
	if err := e.NextErr(); err != nil {
		return nil, err
	}
	return e.findAccount(subscription)
}

func (e *azureCLI) findAccount(subscription string) (*azurecli.Account, error) {
	for _, acc := range e.Accounts {
		if acc.ID == subscription {
			return &acc, nil
		}
		if subscription == "" && acc.IsDefault {
			return &acc, nil
		}
	}
	return nil, errors.New("account not found")
}

func (e *azureCLI) GetAccessToken(subscription, resource string) (*azurecli.AccessToken, error) {
	e.MethodCall(e, "GetAccessToken", subscription, resource)
	if err := e.NextErr(); err != nil {
		return nil, err
	}
	acc, err := e.findAccount(subscription)
	if err != nil {
		return nil, err
	}
	return &azurecli.AccessToken{
		AccessToken: fmt.Sprintf("%s|%s|access-token", subscription, resource),
		Tenant:      acc.TenantId,
		TokenType:   "Bearer",
	}, nil
}

func (e *azureCLI) ShowCloud(name string) (*azurecli.Cloud, error) {
	e.MethodCall(e, "ShowCloud", name)
	if err := e.NextErr(); err != nil {
		return nil, err
	}
	for _, cloud := range e.Clouds {
		if cloud.Name == name || name == "" {
			return &cloud, nil
		}
	}
	return nil, errors.New("cloud not found")
}

func (e *azureCLI) FindCloudsWithResourceManagerEndpoint(url string) ([]azurecli.Cloud, error) {
	e.MethodCall(e, "FindCloudsWithResourceManagerEndpoint", url)
	if err := e.NextErr(); err != nil {
		return nil, err
	}
	for _, cloud := range e.Clouds {
		if cloud.Endpoints.ResourceManager == url {
			return []azurecli.Cloud{cloud}, nil
		}
	}
	return nil, errors.New("cloud not found")
}

func (e *azureCLI) ListClouds() ([]azurecli.Cloud, error) {
	e.MethodCall(e, "ListClouds")
	if err := e.NextErr(); err != nil {
		return nil, err
	}
	return e.Clouds, nil
}
