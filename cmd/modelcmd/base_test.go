// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelcmd_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	cookiejar "github.com/juju/persistent-cookiejar"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon-bakery.v2/httpbakery"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	apitesting "github.com/juju/juju/api/testing"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/cmd/modelcmd/mocks"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/pki"
	"github.com/juju/juju/rpc/params"
	coretesting "github.com/juju/juju/testing"
)

type BaseCommandSuite struct {
	testing.IsolationSuite
	store *jujuclient.MemStore
}

var _ = gc.Suite(&BaseCommandSuite{})

func (s *BaseCommandSuite) SetUpTest(c *gc.C) {
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

func (s *BaseCommandSuite) assertUnknownModel(c *gc.C, baseCmd *modelcmd.ModelCommandBase, current, expectedCurrent string) {
	s.store.Models["foo"].CurrentModel = current
	apiOpen := func(*api.Info, api.DialOpts) (api.Connection, error) {
		return nil, errors.Trace(&params.Error{Code: params.CodeModelNotFound, Message: "model deaddeaf not found"})
	}
	baseCmd.SetClientStore(s.store)
	baseCmd.SetAPIOpen(apiOpen)
	modelcmd.InitContexts(&cmd.Context{Stderr: io.Discard}, baseCmd)
	modelcmd.SetRunStarted(baseCmd)
	baseCmd.SetModelIdentifier("foo:admin/badmodel", false)
	conn, err := baseCmd.NewAPIRoot()
	c.Assert(conn, gc.IsNil)
	msg := strings.Replace(err.Error(), "\n", "", -1)
	c.Assert(msg, gc.Equals, `model "admin/badmodel" has been removed from the controller, run 'juju models' and switch to one of them.`)
	c.Assert(s.store.Models["foo"].Models, gc.HasLen, 1)
	c.Assert(s.store.Models["foo"].Models["admin/goodmodel"], gc.DeepEquals,
		jujuclient.ModelDetails{ModelUUID: "deadbeef2", ModelType: model.IAAS})
	c.Assert(s.store.Models["foo"].CurrentModel, gc.Equals, expectedCurrent)
}

func (s *BaseCommandSuite) TestUnknownModel(c *gc.C) {
	s.assertUnknownModel(c, new(modelcmd.ModelCommandBase), "admin/badmodel", "admin/badmodel")
}

func (s *BaseCommandSuite) TestUnknownUncachedModel(c *gc.C) {
	baseCmd := new(modelcmd.ModelCommandBase)
	baseCmd.CanClearCurrentModel = false
	baseCmd.RemoveModelFromClientStore(s.store, "foo", "admin/nonexistent")
	// expecting silence in the logs since this model has never been cached.
	c.Assert(c.GetTestLog(), gc.DeepEquals, "")
}

func (s *BaseCommandSuite) TestUnknownModelCanRemoveCachedCurrent(c *gc.C) {
	baseCmd := new(modelcmd.ModelCommandBase)
	baseCmd.CanClearCurrentModel = true
	s.assertUnknownModel(c, baseCmd, "admin/badmodel", "")
}

func (s *BaseCommandSuite) TestUnknownModelNotCurrent(c *gc.C) {
	s.assertUnknownModel(c, new(modelcmd.ModelCommandBase), "admin/goodmodel", "admin/goodmodel")
}

func (s *BaseCommandSuite) TestUnknownModelNotCurrentCanRemoveCachedCurrent(c *gc.C) {
	baseCmd := new(modelcmd.ModelCommandBase)
	baseCmd.CanClearCurrentModel = true
	s.assertUnknownModel(c, baseCmd, "admin/goodmodel", "admin/goodmodel")
}

func (s *BaseCommandSuite) TestMigratedModelErrorHandling(c *gc.C) {
	var callCount int
	apiOpen := func(*api.Info, api.DialOpts) (api.Connection, error) {
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

	c.Assert(baseCmd.SetModelIdentifier("foo:admin/badmodel", false), jc.ErrorIsNil)

	fingerprint, _, err := pki.Fingerprint([]byte(coretesting.CACert))
	c.Assert(err, jc.ErrorIsNil)

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

		_, err := baseCmd.NewAPIRoot()
		c.Assert(err, gc.Not(gc.IsNil))
		c.Assert(err.Error(), gc.Equals, spec.expErr)
	}
}

func (s *BaseCommandSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	return ctrl
}

func (s *BaseCommandSuite) TestNewAPIRootExternalUser(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()
	conn := mocks.NewMockConnection(ctrl)
	apiOpen := func(info *api.Info, opts api.DialOpts) (api.Connection, error) {
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

	c.Assert(baseCmd.SetModelIdentifier("foo:admin/badmodel", false), jc.ErrorIsNil)

	_, err := baseCmd.NewAPIRoot()
	c.Assert(err, jc.ErrorIsNil)
}

// TestNewAPIConnectionParams checks that the connection
// parameters used to establish a connection are valid,
// currently only the login provider is verified.
func (s *BaseCommandSuite) TestNewAPIConnectionParams(c *gc.C) {
	baseCmd := new(modelcmd.ModelCommandBase)
	modelcmd.InitContexts(&cmd.Context{Stderr: io.Discard}, baseCmd)
	modelcmd.SetRunStarted(baseCmd)
	s.store.Accounts["foo"] = jujuclient.AccountDetails{
		User: "bar", Password: "hunter2",
	}
	account := s.store.Accounts["foo"]
	params, err := baseCmd.NewAPIConnectionParams(s.store, s.store.CurrentControllerName, "", &account)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(params.DialOpts.LoginProvider, gc.IsNil)
}

// TestNewAPIRoot_OIDCLogin_ClientCredentials verifies that when we have a controller supporting
// OAuth/OIDC login (i.e. JAAS) that the client credential login provider is tried, and if successful
// logs the user in.
func (s *BaseCommandSuite) TestNewAPIRoot_OIDCLogin_ClientCredentials(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	ctx := context.Background()

	conn := mocks.NewMockConnection(ctrl)

	s.store.Controllers["foo"] = jujuclient.ControllerDetails{
		APIEndpoints: []string{"testing.invalid:1234"},
		OIDCLogin:    true,
	}

	baseCmd := new(modelcmd.ModelCommandBase)
	baseCmd.SetClientStore(s.store)
	baseCmd.SetAPIOpen(func(info *api.Info, opts api.DialOpts) (api.Connection, error) {
		// We don't care about the result, just the APICalls made to the login facades.
		_, err := opts.LoginProvider.Login(ctx, conn)
		c.Check(err, gc.IsNil)
		return conn, nil
	})
	modelcmd.InitContexts(&cmd.Context{Stderr: io.Discard}, baseCmd)
	modelcmd.SetRunStarted(baseCmd)

	c.Assert(baseCmd.SetModelIdentifier("foo:admin/badmodel", false), jc.ErrorIsNil)

	request := struct {
		ClientID     string `json:"client-id"`
		ClientSecret string `json:"client-secret"`
	}{
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
	}
	os.Setenv("JUJU_CLIENT_ID", request.ClientID)
	os.Setenv("JUJU_CLIENT_SECRET", request.ClientSecret)
	defer func() {
		os.Unsetenv("JUJU_CLIENT_ID")
		os.Unsetenv("JUJU_CLIENT_SECRET")
	}()

	// Expect env login to succeed.
	conn.EXPECT().
		APICall(
			"Admin",
			gomock.Any(),
			gomock.Any(),
			"LoginWithClientCredentials",
			request,
			gomock.Any(),
		).
		DoAndReturn(func(_ string, _ int, _ string, _ string, _ interface{}, response interface{}) error {
			if r, ok := response.(*params.LoginResult); ok {
				// Set server version so the login calls NewLoginResultParams can succeed.
				r.ServerVersion = "3.6.9"
			} else {
				return fmt.Errorf("unexpected response type %T", response)
			}
			return nil
		})

	conn.EXPECT().AuthTag()
	conn.EXPECT().APIHostPorts()
	conn.EXPECT().ServerVersion()
	conn.EXPECT().Addr()
	conn.EXPECT().IPAddr()
	conn.EXPECT().PublicDNSName()
	conn.EXPECT().IsProxied()
	conn.EXPECT().ControllerAccess()

	_, err := baseCmd.NewAPIRoot()
	c.Assert(err, jc.ErrorIsNil)
}

// TestNewAPIRoot_OIDCLogin_SessionToken verifies that when we have a controller supporting
// OAuth/OIDC login (i.e. JAAS) that the login provider can return a new
// session token which is then saved in the client's account store.
func (s *BaseCommandSuite) TestNewAPIRoot_OIDCLogin_SessionToken(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	ctx := context.Background()

	conn := mocks.NewMockConnection(ctrl)

	sessionLoginFactory := mocks.NewMockSessionLoginFactory(ctrl)
	sessionLoginProvider := mocks.NewMockLoginProvider(ctrl)

	apiOpen := func(info *api.Info, opts api.DialOpts) (api.Connection, error) {
		_, err := opts.LoginProvider.Login(ctx, conn)
		c.Check(err, jc.ErrorIsNil)
		return conn, nil
	}
	externalName := "alice@external"

	conn.EXPECT().AuthTag().Return(names.NewUserTag(externalName)).MinTimes(1)
	conn.EXPECT().APIHostPorts()
	conn.EXPECT().ServerVersion()
	conn.EXPECT().Addr()
	conn.EXPECT().IPAddr()
	conn.EXPECT().PublicDNSName()
	conn.EXPECT().IsProxied()
	conn.EXPECT().ControllerAccess().MinTimes(1)

	var tokenCallbackFunc func(string)
	sessionLoginFactory.EXPECT().NewLoginProvider("test-token", gomock.Any(), gomock.Any()).Do(
		func(_ string, _ io.Writer, f func(string)) {
			tokenCallbackFunc = f
		}).Return(sessionLoginProvider)

	sessionLoginProvider.EXPECT().Login(gomock.Any(), gomock.Any()).Do(
		func(_ context.Context, _ base.APICaller) {
			tokenCallbackFunc("new-token")
		}).Return(nil, nil)

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

	c.Assert(baseCmd.SetModelIdentifier("foo:admin/badmodel", false), jc.ErrorIsNil)

	_, err := baseCmd.NewAPIRoot()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(s.store.Accounts["foo"].SessionToken, gc.Equals, "new-token")
}

// TestNewAPIRoot_OIDCLogin_TriesInOrder verifies that the client credential flow is attempted first and subsequently the
// session token flow. The error returned to the user is the session token flow.
func (s *BaseCommandSuite) TestNewAPIRoot_OIDCLogin_TriesInOrder(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	ctx := context.Background()

	conn := mocks.NewMockConnection(ctrl)

	s.store.Controllers["foo"] = jujuclient.ControllerDetails{
		APIEndpoints: []string{"testing.invalid:1234"},
		OIDCLogin:    true,
	}

	baseCmd := new(modelcmd.ModelCommandBase)
	baseCmd.SetClientStore(s.store)
	baseCmd.SetAPIOpen(func(info *api.Info, opts api.DialOpts) (api.Connection, error) {
		_, err := opts.LoginProvider.Login(ctx, conn)
		return conn, err
	})
	modelcmd.InitContexts(&cmd.Context{Stderr: io.Discard}, baseCmd)
	modelcmd.SetRunStarted(baseCmd)

	c.Assert(baseCmd.SetModelIdentifier("foo:admin/badmodel", false), jc.ErrorIsNil)

	request := struct {
		ClientID     string `json:"client-id"`
		ClientSecret string `json:"client-secret"`
	}{
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
	}
	os.Setenv("JUJU_CLIENT_ID", request.ClientID)
	os.Setenv("JUJU_CLIENT_SECRET", request.ClientSecret)
	defer func() {
		os.Unsetenv("JUJU_CLIENT_ID")
		os.Unsetenv("JUJU_CLIENT_SECRET")
	}()

	// Expect env login to failed.
	conn.EXPECT().
		APICall(
			"Admin",
			gomock.Any(),
			gomock.Any(),
			"LoginWithClientCredentials",
			request,
			gomock.Any(),
		).
		DoAndReturn(func(_ string, _ int, _ string, _ string, _ interface{}, response interface{}) error {
			return errors.New("unauthorised") // Simulate a failed client credential login.
		})

		// Expect session login to be tried next and also fail
	conn.EXPECT().
		APICall(
			"Admin",
			gomock.Any(),
			gomock.Any(),
			"LoginWithSessionToken",
			gomock.Any(),
			gomock.Any(),
		).
		DoAndReturn(func(_ string, _ int, _ string, _ string, _ interface{}, response interface{}) error {
			return errors.New("session token unauthorised")
		})

	_, err := baseCmd.NewAPIRoot()
	// Expect an unauthorised error from the session token attempt.
	c.Assert(err, gc.ErrorMatches, "session token unauthorised")
}

type NewGetBootstrapConfigParamsFuncSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&NewGetBootstrapConfigParamsFuncSuite{})

func (NewGetBootstrapConfigParamsFuncSuite) TestDetectCredentials(c *gc.C) {
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
	_, params, err := f("foo")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(params.Cloud.Credential.Label, gc.Equals, "finalized")
}

func (NewGetBootstrapConfigParamsFuncSuite) TestCloudCACert(c *gc.C) {
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
	_, params, err := f("foo")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(params.Cloud.CACertificates, jc.SameContents, []string{fakeCert})
	c.Assert(params.Cloud.SkipTLSVerify, jc.IsTrue)
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
	testing.IsolationSuite
	store *jujuclient.MemStore
}

var _ = gc.Suite(&OpenAPIFuncSuite{})

func (s *OpenAPIFuncSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.store = jujuclient.NewMemStore()
}

func (s *OpenAPIFuncSuite) TestOpenAPIFunc(c *gc.C) {
	var (
		expected = &api.Info{
			Password:  "meshuggah",
			Macaroons: []macaroon.Slice{{}},
		}
		received *api.Info
	)
	origin := func(info *api.Info, dialOpts api.DialOpts) (api.Connection, error) {
		received = info
		return nil, nil
	}
	openFunc := modelcmd.OpenAPIFuncWithMacaroons(origin, s.store, "foo")
	_, err := openFunc(expected, api.DialOpts{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(received, jc.DeepEquals, expected)
}

func (s *OpenAPIFuncSuite) TestOpenAPIFuncWithNoPassword(c *gc.C) {
	var (
		expected = &api.Info{
			Macaroons: []macaroon.Slice{{}},
		}
		received *api.Info
	)
	origin := func(info *api.Info, dialOpts api.DialOpts) (api.Connection, error) {
		received = info
		return nil, nil
	}
	openFunc := modelcmd.OpenAPIFuncWithMacaroons(origin, s.store, "foo")
	_, err := openFunc(expected, api.DialOpts{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(received, jc.DeepEquals, expected)
}

func (s *OpenAPIFuncSuite) TestOpenAPIFuncWithNoMacaroons(c *gc.C) {
	var (
		expected = &api.Info{
			Password: "meshuggah",
		}
		received *api.Info
	)
	origin := func(info *api.Info, dialOpts api.DialOpts) (api.Connection, error) {
		received = info
		return nil, nil
	}
	openFunc := modelcmd.OpenAPIFuncWithMacaroons(origin, s.store, "foo")
	_, err := openFunc(expected, api.DialOpts{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(received, jc.DeepEquals, expected)
}

func (s *OpenAPIFuncSuite) TestOpenAPIFuncUsesStore(c *gc.C) {
	mac, err := apitesting.NewMacaroon("id")
	c.Assert(err, jc.ErrorIsNil)
	jar, err := cookiejar.New(nil)
	c.Assert(err, jc.ErrorIsNil)

	addCookie(c, jar, mac, api.CookieURLFromHost("foo"))
	s.store.CookieJars["foo"] = jar

	var (
		expected = &api.Info{
			ControllerUUID: "foo",
			Macaroons:      []macaroon.Slice{{mac}},
		}
		received *api.Info
	)
	origin := func(info *api.Info, dialOpts api.DialOpts) (api.Connection, error) {
		received = info
		return nil, nil
	}
	openFunc := modelcmd.OpenAPIFuncWithMacaroons(origin, s.store, "foo")
	_, err = openFunc(&api.Info{
		ControllerUUID: "foo",
	}, api.DialOpts{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(received, jc.DeepEquals, expected)
}

func (s *OpenAPIFuncSuite) TestOpenAPIFuncUsesStoreWithSNIHost(c *gc.C) {
	mac, err := apitesting.NewMacaroon("id")
	c.Assert(err, jc.ErrorIsNil)
	jar, err := cookiejar.New(nil)
	c.Assert(err, jc.ErrorIsNil)

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
	origin := func(info *api.Info, dialOpts api.DialOpts) (api.Connection, error) {
		received = info
		return nil, nil
	}
	openFunc := modelcmd.OpenAPIFuncWithMacaroons(origin, s.store, "foo")
	_, err = openFunc(&api.Info{
		SNIHostName:    "foo",
		ControllerUUID: "bar",
	}, api.DialOpts{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(received, jc.DeepEquals, expected)
}

func addCookie(c *gc.C, jar http.CookieJar, mac *macaroon.Macaroon, url *url.URL) {
	cookie, err := httpbakery.NewCookie(nil, macaroon.Slice{mac})
	c.Assert(err, jc.ErrorIsNil)
	cookie.Expires = time.Now().Add(time.Hour) // only persistent cookies are stored
	jar.SetCookies(url, []*http.Cookie{cookie})
}
