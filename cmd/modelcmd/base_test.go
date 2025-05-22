// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelcmd_test

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	stdtesting "testing"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	cookiejar "github.com/juju/persistent-cookiejar"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"
	"gopkg.in/macaroon-bakery.v2/httpbakery"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/cmd/modelcmd/mocks"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/pki"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/rpc/params"
)

type BaseCommandSuite struct {
	testhelpers.IsolationSuite
	store *jujuclient.MemStore
}

func TestBaseCommandSuite(t *stdtesting.T) {
	tc.Run(t, &BaseCommandSuite{})
}

func (s *BaseCommandSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.store = jujuclient.NewMemStore()
	s.store.CurrentControllerName = "foo"
	s.store.Controllers["foo"] = jujuclient.ControllerDetails{
		APIEndpoints: []string{"testing.invalid:1234"},
	}
	s.store.Models["foo"] = &jujuclient.ControllerModels{
		Models: map[string]jujuclient.ModelDetails{
			"admin/badmodel":  {ModelUUID: "deadbeef", ModelType: model.IAAS},
			"admin/goodmodel": {ModelUUID: "deadbeef2", ModelType: model.IAAS},
		},
		CurrentModel: "admin/badmodel",
	}
	s.store.Accounts["foo"] = jujuclient.AccountDetails{
		User: "bar", Password: "hunter2",
	}
}

func (s *BaseCommandSuite) assertUnknownModel(c *tc.C, baseCmd *modelcmd.ModelCommandBase, current, expectedCurrent string) {
	s.store.Models["foo"].CurrentModel = current
	apiOpen := func(context.Context, *api.Info, api.DialOpts) (api.Connection, error) {
		return nil, errors.Trace(&params.Error{Code: params.CodeModelNotFound, Message: "model deaddeaf not found"})
	}
	baseCmd.SetClientStore(s.store)
	baseCmd.SetAPIOpen(apiOpen)
	modelcmd.InitContexts(&cmd.Context{Stderr: io.Discard}, baseCmd)
	modelcmd.SetRunStarted(baseCmd)
	baseCmd.SetModelIdentifier("foo:admin/badmodel", false)
	conn, err := baseCmd.NewAPIRoot(c.Context())
	c.Assert(conn, tc.IsNil)
	msg := strings.Replace(err.Error(), "\n", "", -1)
	c.Assert(msg, tc.Equals, `model "admin/badmodel" has been removed from the controller, run 'juju models' and switch to one of them.`)
	c.Assert(s.store.Models["foo"].Models, tc.HasLen, 1)
	c.Assert(s.store.Models["foo"].Models["admin/goodmodel"], tc.DeepEquals,
		jujuclient.ModelDetails{ModelUUID: "deadbeef2", ModelType: model.IAAS})
	c.Assert(s.store.Models["foo"].CurrentModel, tc.Equals, expectedCurrent)
}

func (s *BaseCommandSuite) TestUnknownModel(c *tc.C) {
	s.assertUnknownModel(c, new(modelcmd.ModelCommandBase), "admin/badmodel", "admin/badmodel")
}

func (s *BaseCommandSuite) TestUnknownUncachedModel(c *tc.C) {
	baseCmd := new(modelcmd.ModelCommandBase)
	baseCmd.CanClearCurrentModel = false
	baseCmd.RemoveModelFromClientStore(s.store, "foo", "admin/nonexistent")
}

func (s *BaseCommandSuite) TestUnknownModelCanRemoveCachedCurrent(c *tc.C) {
	baseCmd := new(modelcmd.ModelCommandBase)
	baseCmd.CanClearCurrentModel = true
	s.assertUnknownModel(c, baseCmd, "admin/badmodel", "")
}

func (s *BaseCommandSuite) TestUnknownModelNotCurrent(c *tc.C) {
	s.assertUnknownModel(c, new(modelcmd.ModelCommandBase), "admin/goodmodel", "admin/goodmodel")
}

func (s *BaseCommandSuite) TestUnknownModelNotCurrentCanRemoveCachedCurrent(c *tc.C) {
	baseCmd := new(modelcmd.ModelCommandBase)
	baseCmd.CanClearCurrentModel = true
	s.assertUnknownModel(c, baseCmd, "admin/goodmodel", "admin/goodmodel")
}

func (s *BaseCommandSuite) TestMigratedModelErrorHandling(c *tc.C) {
	var callCount int
	apiOpen := func(context.Context, *api.Info, api.DialOpts) (api.Connection, error) {
		var alias string
		if callCount > 0 {
			alias = "brand-new-controller"
		}
		callCount++

		nhp, _ := network.ParseMachineHostPort("1.2.3.4:5555")
		redirErr := &api.RedirectError{
			CACert:          coretesting.CACert,
			Servers:         []network.MachineHostPorts{{*nhp}},
			ControllerAlias: alias,
		}
		return nil, errors.Trace(redirErr)
	}

	baseCmd := new(modelcmd.ModelCommandBase)
	baseCmd.SetClientStore(s.store)
	baseCmd.SetAPIOpen(apiOpen)
	modelcmd.InitContexts(&cmd.Context{Stderr: io.Discard}, baseCmd)
	modelcmd.SetRunStarted(baseCmd)

	c.Assert(baseCmd.SetModelIdentifier("foo:admin/badmodel", false), tc.ErrorIsNil)

	fingerprint, _, err := pki.Fingerprint([]byte(coretesting.CACert))
	c.Assert(err, tc.ErrorIsNil)

	specs := []struct {
		descr   string
		expErr  string
		setupFn func()
	}{
		{
			descr: "model migration document does not contain a controller alias (simulate an older controller)",
			expErr: `Model "admin/badmodel" has been migrated to another controller.
To access it run one of the following commands (you can replace the -c argument with your own preferred controller name):
  'juju login 1.2.3.4:5555 -c new-controller'

New controller fingerprint [` + fingerprint + `]`,
		},
		{
			descr: "model migration document contains a controller alias",
			expErr: `Model "admin/badmodel" has been migrated to another controller.
To access it run one of the following commands (you can replace the -c argument with your own preferred controller name):
  'juju login 1.2.3.4:5555 -c brand-new-controller'

New controller fingerprint [` + fingerprint + `]`,
		},
		{
			descr: "model migration docuemnt contains a controller alias but we already know the controller locally by a different name",
			expErr: `Model "admin/badmodel" has been migrated to controller "bar".
To access it run 'juju switch bar:admin/badmodel'.`,
			setupFn: func() {
				// Model has been migrated to a controller which does exist in the local
				// cache. This can happen if we have 2 users A and B (on separate machines)
				// and:
				// - both A and B have SRC and DST controllers in their local cache
				// - A migrates the model from SRC -> DST
				// - B tries to invoke any model-related command on SRC for the migrated model.
				s.store.Controllers["bar"] = jujuclient.ControllerDetails{
					APIEndpoints: []string{"1.2.3.4:5555"},
				}
			},
		},
	}

	for specIndex, spec := range specs {
		c.Logf("test %d: %s", specIndex, spec.descr)

		if spec.setupFn != nil {
			spec.setupFn()
		}

		_, err := baseCmd.NewAPIRoot(c.Context())
		c.Assert(err, tc.Not(tc.IsNil))
		c.Assert(err.Error(), tc.Equals, spec.expErr)
	}
}

func (s *BaseCommandSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	return ctrl
}

func (s *BaseCommandSuite) TestNewAPIRootExternalUser(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()
	conn := mocks.NewMockConnection(ctrl)
	apiOpen := func(ctx context.Context, info *api.Info, opts api.DialOpts) (api.Connection, error) {
		return conn, nil
	}
	externalName := "alastair@external"
	conn.EXPECT().AuthTag().Return(names.NewUserTag(externalName)).MinTimes(1)
	conn.EXPECT().APIHostPorts()
	conn.EXPECT().ServerVersion()
	conn.EXPECT().Addr()
	conn.EXPECT().IPAddr()
	conn.EXPECT().PublicDNSName()
	conn.EXPECT().IsProxied()
	conn.EXPECT().ControllerAccess().MinTimes(1)

	s.store.Accounts["foo"] = jujuclient.AccountDetails{
		User: externalName,
	}

	baseCmd := new(modelcmd.ModelCommandBase)
	baseCmd.SetClientStore(s.store)
	baseCmd.SetAPIOpen(apiOpen)
	modelcmd.InitContexts(&cmd.Context{Stderr: io.Discard}, baseCmd)
	modelcmd.SetRunStarted(baseCmd)

	c.Assert(baseCmd.SetModelIdentifier("foo:admin/badmodel", false), tc.ErrorIsNil)

	_, err := baseCmd.NewAPIRoot(c.Context())
	c.Assert(err, tc.ErrorIsNil)
}

// TestLoginWithOIDC verifies that when we have a controller supporting
// OAuth/OIDC login (i.e. JAAS) that the login provider can return a new
// session token which is then saved in the client's account store.
// This specifically tests all commands *besides* `juju login`
// since `juju login` uses a different code path.
func (s *BaseCommandSuite) TestLoginWithOIDC(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()
	conn := mocks.NewMockConnection(ctrl)
	sessionLoginFactory := mocks.NewMockSessionLoginFactory(ctrl)
	sessionLoginProvider := mocks.NewMockLoginProvider(ctrl)

	apiOpen := func(ctx context.Context, info *api.Info, opts api.DialOpts) (api.Connection, error) {
		_, err := opts.LoginProvider.Login(ctx, conn)
		c.Check(err, tc.ErrorIsNil)
		return conn, nil
	}
	externalName := "kian@external"

	conn.EXPECT().AuthTag().Return(names.NewUserTag(externalName)).MinTimes(1)
	conn.EXPECT().APIHostPorts()
	conn.EXPECT().ServerVersion()
	conn.EXPECT().Addr()
	conn.EXPECT().IPAddr()
	conn.EXPECT().PublicDNSName()
	conn.EXPECT().IsProxied()
	conn.EXPECT().ControllerAccess().MinTimes(1)

	var tokenCallbackFunc func(string)
	sessionLoginFactory.EXPECT().NewLoginProvider("test-token", gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ string, _ io.Writer, f func(string)) api.LoginProvider {
			tokenCallbackFunc = f
			return sessionLoginProvider
		})

	sessionLoginProvider.EXPECT().Login(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _ base.APICaller) (*api.LoginResultParams, error) {
			tokenCallbackFunc("new-token")
			return nil, nil
		})

	s.store.Controllers["foo"] = jujuclient.ControllerDetails{
		APIEndpoints: []string{"testing.invalid:1234"},
		OIDCLogin:    true,
	}

	s.store.Accounts["foo"] = jujuclient.AccountDetails{
		User:         externalName,
		SessionToken: "test-token",
	}

	baseCmd := new(modelcmd.ModelCommandBase)
	baseCmd.SetClientStore(s.store)
	baseCmd.SetAPIOpen(apiOpen)
	baseCmd.SetSessionLoginFactory(sessionLoginFactory)
	modelcmd.InitContexts(&cmd.Context{Stderr: io.Discard}, baseCmd)
	modelcmd.SetRunStarted(baseCmd)

	c.Assert(baseCmd.SetModelIdentifier("foo:admin/badmodel", false), tc.ErrorIsNil)

	_, err := baseCmd.NewAPIRoot(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(s.store.Accounts["foo"].SessionToken, tc.Equals, "new-token")
}

// TestNewAPIConnectionParams checks that the connection
// parameters used to establish a connection are valid,
// currently only the login provider is verified.
func (s *BaseCommandSuite) TestNewAPIConnectionParams(c *tc.C) {
	baseCmd := new(modelcmd.ModelCommandBase)
	modelcmd.InitContexts(&cmd.Context{Stderr: io.Discard}, baseCmd)
	modelcmd.SetRunStarted(baseCmd)
	s.store.Accounts["foo"] = jujuclient.AccountDetails{
		User: "bar", Password: "hunter2",
	}
	account := s.store.Accounts["foo"]
	params, err := baseCmd.NewAPIConnectionParams(s.store, s.store.CurrentControllerName, "", &account)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(params.DialOpts.LoginProvider, tc.IsNil)
}

// TestNewAPIConnectionParamsWithOAuthController is similar
// to TestNewAPIConnectionParams but verifies that when
// connecting to a controller supporting OIDC, we default
// to a specific kind of login provider.
func (s *BaseCommandSuite) TestNewAPIConnectionParamsWithOAuthController(c *tc.C) {
	newController, err := s.store.ControllerByName(s.store.CurrentControllerName)
	c.Assert(err, tc.ErrorIsNil)
	newController.OIDCLogin = true
	s.store.Controllers["oauth-controller"] = *newController

	baseCmd := new(modelcmd.ModelCommandBase)
	modelcmd.InitContexts(&cmd.Context{Stderr: io.Discard}, baseCmd)
	modelcmd.SetRunStarted(baseCmd)
	s.store.Accounts["foo"] = jujuclient.AccountDetails{
		User: "bar", Password: "hunter2",
	}
	account := s.store.Accounts["foo"]
	params, err := baseCmd.NewAPIConnectionParams(s.store, "oauth-controller", "", &account)
	c.Assert(err, tc.ErrorIsNil)
	sessionTokenLogin := api.NewSessionTokenLoginProvider("", nil, nil)
	c.Assert(params.DialOpts.LoginProvider, tc.FitsTypeOf, sessionTokenLogin)
}

type NewGetBootstrapConfigParamsFuncSuite struct {
	testhelpers.IsolationSuite
}

func TestNewGetBootstrapConfigParamsFuncSuite(t *stdtesting.T) {
	tc.Run(t, &NewGetBootstrapConfigParamsFuncSuite{})
}
func (s *NewGetBootstrapConfigParamsFuncSuite) TestDetectCredentials(c *tc.C) {
	clientStore := jujuclient.NewMemStore()
	clientStore.Controllers["foo"] = jujuclient.ControllerDetails{}
	clientStore.BootstrapConfig["foo"] = jujuclient.BootstrapConfig{
		Cloud:               "cloud",
		CloudType:           "cloud-type",
		ControllerModelUUID: coretesting.ModelTag.Id(),
		Config: map[string]interface{}{
			"name":           "foo",
			"type":           "cloud-type",
			"secret-backend": "auto",
		},
	}
	var registry mockProviderRegistry

	f := modelcmd.NewGetBootstrapConfigParamsFunc(
		cmdtesting.Context(c),
		clientStore,
		&registry,
	)
	_, spec, _, err := f("foo")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(spec.Credential.Label, tc.Equals, "finalized")
}

func (s *NewGetBootstrapConfigParamsFuncSuite) TestCloudCACert(c *tc.C) {
	fakeCert := coretesting.CACert
	clientStore := jujuclient.NewMemStore()
	clientStore.Controllers["foo"] = jujuclient.ControllerDetails{}
	clientStore.BootstrapConfig["foo"] = jujuclient.BootstrapConfig{
		Cloud:               "cloud",
		CloudType:           "cloud-type",
		ControllerModelUUID: coretesting.ModelTag.Id(),
		Config: map[string]interface{}{
			"name":           "foo",
			"type":           "cloud-type",
			"secret-backend": "auto",
		},
		CloudCACertificates: []string{fakeCert},
		SkipTLSVerify:       true,
	}
	var registry mockProviderRegistry

	f := modelcmd.NewGetBootstrapConfigParamsFunc(
		cmdtesting.Context(c),
		clientStore,
		&registry,
	)
	_, spec, _, err := f("foo")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(spec.CACertificates, tc.SameContents, []string{fakeCert})
	c.Assert(spec.SkipTLSVerify, tc.IsTrue)
}

type mockProviderRegistry struct {
	environs.ProviderRegistry
}

func (r *mockProviderRegistry) Provider(t string) (environs.EnvironProvider, error) {
	return &mockEnvironProvider{}, nil
}

type mockEnvironProvider struct {
	environs.EnvironProvider
}

func (p *mockEnvironProvider) DetectCredentials(cloudName string) (*cloud.CloudCredential, error) {
	return cloud.NewEmptyCloudCredential(), nil
}

func (p *mockEnvironProvider) FinalizeCredential(
	_ environs.FinalizeCredentialContext,
	args environs.FinalizeCredentialParams,
) (*cloud.Credential, error) {
	out := args.Credential
	out.Label = "finalized"
	return &out, nil
}

func (p *mockEnvironProvider) CredentialSchemas() map[cloud.AuthType]cloud.CredentialSchema {
	return map[cloud.AuthType]cloud.CredentialSchema{cloud.EmptyAuthType: {}}
}

type OpenAPIFuncSuite struct {
	testhelpers.IsolationSuite
	store *jujuclient.MemStore
}

func TestOpenAPIFuncSuite(t *stdtesting.T) {
	tc.Run(t, &OpenAPIFuncSuite{})
}

func (s *OpenAPIFuncSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.store = jujuclient.NewMemStore()
}

func (s *OpenAPIFuncSuite) TestOpenAPIFunc(c *tc.C) {
	var (
		expected = &api.Info{
			Password:  "meshuggah",
			Macaroons: []macaroon.Slice{{}},
		}
		received *api.Info
	)
	origin := func(ctx context.Context, info *api.Info, dialOpts api.DialOpts) (api.Connection, error) {
		received = info
		return nil, nil
	}
	openFunc := modelcmd.OpenAPIFuncWithMacaroons(origin, s.store, "foo")
	_, err := openFunc(c.Context(), expected, api.DialOpts{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(received, tc.DeepEquals, expected)
}

func (s *OpenAPIFuncSuite) TestOpenAPIFuncWithNoPassword(c *tc.C) {
	var (
		expected = &api.Info{
			Macaroons: []macaroon.Slice{{}},
		}
		received *api.Info
	)
	origin := func(ctx context.Context, info *api.Info, dialOpts api.DialOpts) (api.Connection, error) {
		received = info
		return nil, nil
	}
	openFunc := modelcmd.OpenAPIFuncWithMacaroons(origin, s.store, "foo")
	_, err := openFunc(c.Context(), expected, api.DialOpts{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(received, tc.DeepEquals, expected)
}

func (s *OpenAPIFuncSuite) TestOpenAPIFuncWithNoMacaroons(c *tc.C) {
	var (
		expected = &api.Info{
			Password: "meshuggah",
		}
		received *api.Info
	)
	origin := func(ctx context.Context, info *api.Info, dialOpts api.DialOpts) (api.Connection, error) {
		received = info
		return nil, nil
	}
	openFunc := modelcmd.OpenAPIFuncWithMacaroons(origin, s.store, "foo")
	_, err := openFunc(c.Context(), expected, api.DialOpts{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(received, tc.DeepEquals, expected)
}

func (s *OpenAPIFuncSuite) TestOpenAPIFuncUsesStore(c *tc.C) {
	mac, err := jujutesting.NewMacaroon("id")
	c.Assert(err, tc.ErrorIsNil)
	jar, err := cookiejar.New(nil)
	c.Assert(err, tc.ErrorIsNil)

	addCookie(c, jar, mac, api.CookieURLFromHost("foo"))
	s.store.CookieJars["foo"] = jar

	var (
		expected = &api.Info{
			ControllerUUID: "foo",
			Macaroons:      []macaroon.Slice{{mac}},
		}
		received *api.Info
	)
	origin := func(ctx context.Context, info *api.Info, dialOpts api.DialOpts) (api.Connection, error) {
		received = info
		return nil, nil
	}
	openFunc := modelcmd.OpenAPIFuncWithMacaroons(origin, s.store, "foo")
	_, err = openFunc(c.Context(), &api.Info{
		ControllerUUID: "foo",
	}, api.DialOpts{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(received, tc.DeepEquals, expected)
}

func (s *OpenAPIFuncSuite) TestOpenAPIFuncUsesStoreWithSNIHost(c *tc.C) {
	mac, err := jujutesting.NewMacaroon("id")
	c.Assert(err, tc.ErrorIsNil)
	jar, err := cookiejar.New(nil)
	c.Assert(err, tc.ErrorIsNil)

	addCookie(c, jar, mac, api.CookieURLFromHost("foo"))
	s.store.CookieJars["foo"] = jar

	var (
		expected = &api.Info{
			SNIHostName:    "foo",
			ControllerUUID: "bar",
			Macaroons:      []macaroon.Slice{{mac}},
		}
		received *api.Info
	)
	origin := func(ctx context.Context, info *api.Info, dialOpts api.DialOpts) (api.Connection, error) {
		received = info
		return nil, nil
	}
	openFunc := modelcmd.OpenAPIFuncWithMacaroons(origin, s.store, "foo")
	_, err = openFunc(c.Context(), &api.Info{
		SNIHostName:    "foo",
		ControllerUUID: "bar",
	}, api.DialOpts{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(received, tc.DeepEquals, expected)
}

func addCookie(c *tc.C, jar http.CookieJar, mac *macaroon.Macaroon, url *url.URL) {
	cookie, err := httpbakery.NewCookie(nil, macaroon.Slice{mac})
	c.Assert(err, tc.ErrorIsNil)
	cookie.Expires = time.Now().Add(time.Hour) // only persistent cookies are stored
	jar.SetCookies(url, []*http.Cookie{cookie})
}
