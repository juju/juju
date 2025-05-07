// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure_test

import (
	"context"
	"io"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/provider/azure"
	"github.com/juju/juju/internal/provider/azure/internal/azureauth"
	"github.com/juju/juju/internal/provider/azure/internal/azurecli"
	"github.com/juju/juju/internal/provider/azure/internal/azuretesting"
)

type credentialsSuite struct {
	testing.IsolationSuite
	servicePrincipalCreator servicePrincipalCreator
	azureCLI                azureCLI
	provider                environs.EnvironProvider
	sender                  azuretesting.Senders
}

var _ = tc.Suite(&credentialsSuite{})

func (s *credentialsSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.servicePrincipalCreator = servicePrincipalCreator{}
	s.azureCLI = azureCLI{}
	s.provider = newProvider(c, azure.ProviderConfig{
		ServicePrincipalCreator: &s.servicePrincipalCreator,
		AzureCLI:                &s.azureCLI,
		Sender:                  azuretesting.NewSerialSender(&s.sender),
	})
}

func (s *credentialsSuite) TestCredentialSchemas(c *tc.C) {
	envtesting.AssertProviderAuthTypes(c, s.provider,
		"interactive",
		"service-principal-secret",
		"managed-identity",
	)
}

func (s *credentialsSuite) TestServicePrincipalSecretCredentialsValid(c *tc.C) {
	envtesting.AssertProviderCredentialsValid(c, s.provider, "service-principal-secret", map[string]string{
		"application-id":          "application",
		"application-password":    "password",
		"subscription-id":         "subscription",
		"managed-subscription-id": "managed-subscription",
	})
}

func (s *credentialsSuite) TestManagedIdentityCredentialsValid(c *tc.C) {
	envtesting.AssertProviderCredentialsValid(c, s.provider, "managed-identity", map[string]string{
		"managed-identity-path": "some-identity",
		"subscription-id":       "subscription",
	})
}

func (s *credentialsSuite) TestServicePrincipalSecretHiddenAttributes(c *tc.C) {
	envtesting.AssertProviderCredentialsAttributesHidden(c, s.provider, "service-principal-secret", "application-password")
}

func (s *credentialsSuite) TestDetectCredentialsNoAccounts(c *tc.C) {
	_, err := s.provider.DetectCredentials("")
	c.Assert(err, jc.ErrorIs, errors.NotFound)
	calls := s.azureCLI.Calls()
	c.Assert(calls, tc.HasLen, 1)
	c.Assert(calls[0].FuncName, tc.Equals, "ListAccounts")
}

func (s *credentialsSuite) TestDetectCredentialsListError(c *tc.C) {
	s.azureCLI.SetErrors(errors.New("test error"))
	_, err := s.provider.DetectCredentials("")
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *credentialsSuite) TestDetectCredentialsOneAccount(c *tc.C) {
	s.azureCLI.Accounts = []azurecli.Account{{
		CloudName:    "AzureCloud",
		ID:           "test-account-id",
		IsDefault:    true,
		Name:         "test-account",
		State:        "Enabled",
		TenantId:     "tenant-id",
		HomeTenantId: "home-tenant-id",
	}}
	s.azureCLI.Clouds = []azurecli.Cloud{{
		IsActive: true,
		Name:     "AzureCloud",
	}}
	cred, err := s.provider.DetectCredentials("")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cred, tc.Not(tc.IsNil))
	c.Assert(cred.DefaultCredential, tc.Equals, "test-account")
	c.Assert(cred.DefaultRegion, tc.Equals, "")
	c.Assert(cred.AuthCredentials, tc.HasLen, 1)
	c.Assert(cred.AuthCredentials["test-account"].Label, tc.Equals, "AzureCloud subscription test-account")

	calls := s.azureCLI.Calls()
	c.Assert(calls, tc.HasLen, 2)
	c.Assert(calls[0].FuncName, tc.Equals, "ListAccounts")
	c.Assert(calls[1].FuncName, tc.Equals, "ListClouds")

	calls = s.servicePrincipalCreator.Calls()
	c.Assert(calls, tc.HasLen, 1)
	c.Assert(calls[0].FuncName, tc.Equals, "Create")
	params, ok := calls[0].Args[1].(azureauth.ServicePrincipalParams)
	c.Assert(ok, jc.IsTrue)
	params.Credential = nil
	c.Assert(params, jc.DeepEquals, azureauth.ServicePrincipalParams{
		SubscriptionId: "test-account-id",
		TenantId:       "tenant-id",
	})
}

func (s *credentialsSuite) TestDetectCredentialsCloudError(c *tc.C) {
	s.azureCLI.Accounts = []azurecli.Account{{
		CloudName: "AzureCloud",
		ID:        "test-account-id",
		IsDefault: true,
		Name:      "test-account",
		State:     "Enabled",
		TenantId:  "tenant-id",
	}}
	s.azureCLI.Clouds = []azurecli.Cloud{{
		IsActive: true,
		Name:     "AzureCloud",
	}}
	s.azureCLI.SetErrors(nil, errors.New("test error"))
	cred, err := s.provider.DetectCredentials("")
	c.Assert(err, jc.ErrorIs, errors.NotFound)
	c.Assert(cred, tc.IsNil)

	calls := s.azureCLI.Calls()
	c.Assert(calls, tc.HasLen, 2)
	c.Assert(calls[0].FuncName, tc.Equals, "ListAccounts")
	c.Assert(calls[1].FuncName, tc.Equals, "ListClouds")

	calls = s.servicePrincipalCreator.Calls()
	c.Assert(calls, tc.HasLen, 0)
}

func (s *credentialsSuite) TestDetectCredentialsTwoAccounts(c *tc.C) {
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
		TenantId:  "tenant-id2",
	}}
	s.azureCLI.Clouds = []azurecli.Cloud{{
		IsActive: true,
		Name:     "AzureCloud",
	}}
	cred, err := s.provider.DetectCredentials("")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cred, tc.Not(tc.IsNil))
	c.Assert(cred.DefaultCredential, tc.Equals, "test-account1")
	c.Assert(cred.DefaultRegion, tc.Equals, "")
	c.Assert(cred.AuthCredentials, tc.HasLen, 2)
	c.Assert(cred.AuthCredentials["test-account1"].Label, tc.Equals, "AzureCloud subscription test-account1")
	c.Assert(cred.AuthCredentials["test-account2"].Label, tc.Equals, "AzureCloud subscription test-account2")

	calls := s.azureCLI.Calls()
	c.Assert(calls, tc.HasLen, 2)
	c.Assert(calls[0].FuncName, tc.Equals, "ListAccounts")
	c.Assert(calls[1].FuncName, tc.Equals, "ListClouds")

	calls = s.servicePrincipalCreator.Calls()
	c.Assert(calls, tc.HasLen, 2)
	c.Assert(calls[0].FuncName, tc.Equals, "Create")
	params, ok := calls[0].Args[1].(azureauth.ServicePrincipalParams)
	c.Assert(ok, jc.IsTrue)
	params.Credential = nil
	c.Assert(params, jc.DeepEquals, azureauth.ServicePrincipalParams{
		SubscriptionId: "test-account1-id",
		TenantId:       "tenant-id",
	})
	c.Assert(calls[1].FuncName, tc.Equals, "Create")
	params, ok = calls[1].Args[1].(azureauth.ServicePrincipalParams)
	c.Assert(ok, jc.IsTrue)
	params.Credential = nil
	c.Assert(params, jc.DeepEquals, azureauth.ServicePrincipalParams{
		SubscriptionId: "test-account2-id",
		TenantId:       "tenant-id2",
	})
}

func (s *credentialsSuite) TestDetectCredentialsTwoAccountsOneError(c *tc.C) {
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
		TenantId:  "tenant-id2",
	}}
	s.azureCLI.Clouds = []azurecli.Cloud{{
		IsActive: true,
		Name:     "AzureCloud",
	}}
	s.servicePrincipalCreator.SetErrors(nil, errors.New("test error"))
	cred, err := s.provider.DetectCredentials("")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cred, tc.Not(tc.IsNil))
	c.Assert(cred.DefaultCredential, tc.Equals, "")
	c.Assert(cred.DefaultRegion, tc.Equals, "")
	c.Assert(cred.AuthCredentials, tc.HasLen, 1)
	c.Assert(cred.AuthCredentials["test-account1"].Label, tc.Equals, "AzureCloud subscription test-account1")

	calls := s.azureCLI.Calls()
	c.Assert(calls, tc.HasLen, 2)
	c.Assert(calls[0].FuncName, tc.Equals, "ListAccounts")
	c.Assert(calls[1].FuncName, tc.Equals, "ListClouds")

	calls = s.servicePrincipalCreator.Calls()
	c.Assert(calls, tc.HasLen, 2)
	c.Assert(calls[0].FuncName, tc.Equals, "Create")
	params, ok := calls[0].Args[1].(azureauth.ServicePrincipalParams)
	c.Assert(ok, jc.IsTrue)
	params.Credential = nil
	c.Assert(params, jc.DeepEquals, azureauth.ServicePrincipalParams{
		SubscriptionId: "test-account1-id",
		TenantId:       "tenant-id",
	})
	c.Assert(calls[1].FuncName, tc.Equals, "Create")
	params, ok = calls[1].Args[1].(azureauth.ServicePrincipalParams)
	c.Assert(ok, jc.IsTrue)
	params.Credential = nil
	c.Assert(params, jc.DeepEquals, azureauth.ServicePrincipalParams{
		SubscriptionId: "test-account2-id",
		TenantId:       "tenant-id2",
	})
}

func (s *credentialsSuite) TestFinalizeCredentialInteractive(c *tc.C) {
	s.sender = azuretesting.Senders{discoverAuthSender()}
	in := cloud.NewCredential("interactive", map[string]string{"subscription-id": fakeSubscriptionId})
	ctx := cmdtesting.Context(c)
	out, err := s.provider.FinalizeCredential(ctx, environs.FinalizeCredentialParams{
		CloudName:             "azure",
		Credential:            in,
		CloudEndpoint:         "https://arm.invalid",
		CloudStorageEndpoint:  "https://core.invalid",
		CloudIdentityEndpoint: "https://graph.invalid",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out, tc.NotNil)
	c.Assert(out.AuthType(), tc.Equals, cloud.AuthType("service-principal-secret"))
	c.Assert(out.Attributes(), jc.DeepEquals, map[string]string{
		"application-id":        "appid",
		"application-password":  "service-principal-password",
		"application-object-id": "application-object-id",
		"subscription-id":       fakeSubscriptionId,
	})

	s.servicePrincipalCreator.CheckCallNames(c, "InteractiveCreate")
	args := s.servicePrincipalCreator.Calls()[0].Args
	c.Assert(args[2], jc.DeepEquals, azureauth.ServicePrincipalParams{
		CloudName:      "AzureCloud",
		SubscriptionId: fakeSubscriptionId,
		TenantId:       fakeTenantId,
	})
}

func (s *credentialsSuite) TestFinalizeCredentialInteractiveError(c *tc.C) {
	s.sender = azuretesting.Senders{discoverAuthSender()}
	in := cloud.NewCredential("interactive", map[string]string{"subscription-id": fakeSubscriptionId})
	s.servicePrincipalCreator.SetErrors(errors.New("blargh"))
	ctx := cmdtesting.Context(c)
	_, err := s.provider.FinalizeCredential(ctx, environs.FinalizeCredentialParams{
		CloudName:             "azure",
		Credential:            in,
		CloudEndpoint:         "https://arm.invalid",
		CloudIdentityEndpoint: "https://graph.invalid",
	})
	c.Assert(err, tc.ErrorMatches, "blargh")
}

func (s *credentialsSuite) TestFinalizeCredentialInstanceRole(c *tc.C) {
	s.sender = azuretesting.Senders{discoverAuthSender()}
	in := cloud.NewCredential("managed-identity", map[string]string{
		"subscription-id":       fakeSubscriptionId,
		"managed-identity-path": "mymid",
	})
	ctx := cmdtesting.Context(c)
	out, err := s.provider.FinalizeCredential(ctx, environs.FinalizeCredentialParams{
		CloudName:             "azure",
		Credential:            in,
		CloudEndpoint:         "https://arm.invalid",
		CloudStorageEndpoint:  "https://core.invalid",
		CloudIdentityEndpoint: "https://graph.invalid",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out, tc.NotNil)
	c.Assert(out.AuthType(), tc.Equals, cloud.AuthType("managed-identity"))
	c.Assert(out.Attributes(), jc.DeepEquals, map[string]string{
		"managed-identity-path": "mymid",
		"subscription-id":       fakeSubscriptionId,
	})
}

func (s *credentialsSuite) TestFinalizeCredentialInstanceRoleError(c *tc.C) {
	s.sender = azuretesting.Senders{discoverAuthSender()}
	in := cloud.NewCredential("managed-identity", map[string]string{"subscription-id": fakeSubscriptionId})
	ctx := cmdtesting.Context(c)
	_, err := s.provider.FinalizeCredential(ctx, environs.FinalizeCredentialParams{
		CloudName:             "azure",
		Credential:            in,
		CloudEndpoint:         "https://arm.invalid",
		CloudIdentityEndpoint: "https://graph.invalid",
	})
	c.Assert(err, tc.ErrorMatches, "managed identity path must be <name> or <resourcegroup>/<name>")
}

func (s *credentialsSuite) TestFinalizeCredentialAzureCLI(c *tc.C) {
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
		IsActive: true,
		Name:     "AzureCloud",
	}}
	in := cloud.NewCredential("interactive", nil)
	ctx := cmdtesting.Context(c)
	cred, err := s.provider.FinalizeCredential(ctx, environs.FinalizeCredentialParams{
		Credential: in,
		CloudName:  "azure",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cred, tc.Not(tc.IsNil))
	c.Assert(cred.AuthType(), tc.Equals, cloud.AuthType("service-principal-secret"))
	attrs := cred.Attributes()
	c.Assert(attrs["subscription-id"], tc.Equals, "test-account1-id")
	c.Assert(attrs["application-id"], tc.Equals, "appid")
	c.Assert(attrs["application-password"], tc.Equals, "service-principal-password")
	c.Assert(attrs["application-object-id"], tc.Equals, "application-object-id")
}

func (s *credentialsSuite) TestFinalizeCredentialAzureCLIShowAccountError(c *tc.C) {
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
		IsActive: true,
		Name:     "AzureCloud",
	}}
	s.azureCLI.SetErrors(errors.New("test error"))
	in := cloud.NewCredential("interactive", nil)
	ctx := cmdtesting.Context(c)
	cred, err := s.provider.FinalizeCredential(ctx, environs.FinalizeCredentialParams{
		Credential: in,
		CloudName:  "azure",
	})
	c.Assert(err, tc.ErrorMatches, `cannot get accounts: test error`)
	c.Assert(cred, tc.IsNil)
}

func (s *credentialsSuite) TestFinalizeCredentialAzureCLIGraphTokenError(c *tc.C) {
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
		IsActive: true,
		Name:     "AzureCloud",
	}}
	s.servicePrincipalCreator.SetErrors(errors.New("test error"))
	in := cloud.NewCredential("interactive", nil)
	ctx := cmdtesting.Context(c)
	cred, err := s.provider.FinalizeCredential(ctx, environs.FinalizeCredentialParams{
		Credential: in,
		CloudName:  "azure",
	})
	c.Assert(err, tc.ErrorMatches, `cannot create service principal: test error`)
	c.Assert(cred, tc.IsNil)
}

func (s *credentialsSuite) TestFinalizeCredentialAzureCLIServicePrincipalError(c *tc.C) {
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
		IsActive: true,
		Name:     "AzureCloud",
	}}
	s.servicePrincipalCreator.SetErrors(errors.New("test error"))
	in := cloud.NewCredential("interactive", nil)
	ctx := cmdtesting.Context(c)
	cred, err := s.provider.FinalizeCredential(ctx, environs.FinalizeCredentialParams{
		Credential: in,
		CloudName:  "azure",
	})
	c.Assert(err, tc.ErrorMatches, `cannot create service principal: test error`)
	c.Assert(cred, tc.IsNil)
}

type servicePrincipalCreator struct {
	testing.Stub
}

func (c *servicePrincipalCreator) InteractiveCreate(sdkCtx context.Context, stderr io.Writer, params azureauth.ServicePrincipalParams) (appId, spId, password string, _ error) {
	c.MethodCall(c, "InteractiveCreate", sdkCtx, stderr, params)
	return "appid", "application-object-id", "service-principal-password", c.NextErr()
}

func (c *servicePrincipalCreator) Create(sdkCtx context.Context, params azureauth.ServicePrincipalParams) (appId, spId, password string, _ error) {
	c.MethodCall(c, "Create", sdkCtx, params)
	return "appid", "application-object-id", "service-principal-password", c.NextErr()
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

func (e *azureCLI) findAccount(tenant string) (*azurecli.Account, error) {
	for _, acc := range e.Accounts {
		if acc.AuthTenantId() == tenant {
			return &acc, nil
		}
		if tenant == "" && acc.IsDefault {
			return &acc, nil
		}
	}
	return nil, errors.New("account not found")
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

func (e *azureCLI) ListClouds() ([]azurecli.Cloud, error) {
	e.MethodCall(e, "ListClouds")
	if err := e.NextErr(); err != nil {
		return nil, err
	}
	return e.Clouds, nil
}
