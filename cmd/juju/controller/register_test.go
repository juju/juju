// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller_test

import (
	"context"
	"encoding/asn1"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	stdtesting "testing"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"golang.org/x/crypto/nacl/secretbox"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/cmd/juju/controller"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/rpc/params"
)

type RegisterSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	apiConnection            *mockAPIConnection
	store                    *jujuclient.MemStore
	apiOpenError             error
	listModels               func(context.Context, jujuclient.ClientStore, string, string) ([]base.UserModel, error)
	listModelsControllerName string
	listModelsUserName       string
	server                   *httptest.Server
	httpHandler              http.Handler
}

const noModelsText = `
There are no models available. You can add models with
"juju add-model", or you can ask an administrator of a
model to grant access to that model with "juju grant".
`

func TestRegisterSuite(t *stdtesting.T) {
	tc.Run(t, &RegisterSuite{})
}

func (s *RegisterSuite) SetUpTest(c *tc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)

	s.apiOpenError = nil
	s.httpHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	s.server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.httpHandler.ServeHTTP(w, r)
	}))

	serverURL, err := url.Parse(s.server.URL)
	c.Assert(err, tc.ErrorIsNil)
	s.apiConnection = &mockAPIConnection{
		controllerTag: names.NewControllerTag(mockControllerUUID),
		addr:          serverURL,
	}
	s.listModelsControllerName = ""
	s.listModelsUserName = ""
	s.listModels = func(ctx context.Context, _ jujuclient.ClientStore, controllerName, userName string) ([]base.UserModel, error) {
		s.listModelsControllerName = controllerName
		s.listModelsUserName = userName
		return nil, nil
	}

	s.store = jujuclient.NewMemStore()
}

func (s *RegisterSuite) TearDownTest(c *tc.C) {
	s.server.Close()
	s.FakeJujuXDGDataHomeSuite.TearDownTest(c)
}

func (s *RegisterSuite) TestInit(c *tc.C) {
	registerCommand := controller.NewRegisterCommandForTest(nil, nil, nil)

	err := cmdtesting.InitCommand(registerCommand, []string{})
	c.Assert(err, tc.ErrorMatches, "registration data missing")

	err = cmdtesting.InitCommand(registerCommand, []string{"foo", "bar"})
	c.Assert(err, tc.ErrorMatches, `unrecognized args: \["bar"\]`)
}

func (s *RegisterSuite) TestRegister(c *tc.C) {
	s.testRegisterSuccess(c, nil, "", false, false)
	c.Assert(s.listModelsControllerName, tc.Equals, "controller-name")
	c.Assert(s.listModelsUserName, tc.Equals, "bob")
}

func (s *RegisterSuite) TestRegisterWithProxy(c *tc.C) {
	s.testRegisterSuccess(c, nil, "", true, false)
	c.Assert(s.listModelsControllerName, tc.Equals, "controller-name")
	c.Assert(s.listModelsUserName, tc.Equals, "bob")
}

func (s *RegisterSuite) TestRegisterOneModel(c *tc.C) {
	s.listModels = func(ctx context.Context, _ jujuclient.ClientStore, controllerName, userName string) ([]base.UserModel, error) {
		return []base.UserModel{{
			Name:      "theoneandonly",
			Qualifier: "prod",
			UUID:      mockControllerUUID,
			Type:      model.IAAS,
		}}, nil
	}
	prompter := cmdtesting.NewSeqPrompter(c, "»", `
Enter a new password: »hunter2

Confirm password: »hunter2

Enter a name for this controller \[controller-name\]: »
Initial password successfully set for bob.

Welcome, bob. You are now logged into "controller-name".

Current model set to "prod/theoneandonly".
`[1:])
	s.testRegisterSuccess(c, prompter, "", false, false)
	c.Assert(
		s.store.Models["controller-name"].CurrentModel,
		tc.Equals, "prod/theoneandonly",
	)
	prompter.CheckDone()
}

func (s *RegisterSuite) TestRegisterMultipleModels(c *tc.C) {
	s.listModels = func(ctx context.Context, _ jujuclient.ClientStore, controllerName, userName string) ([]base.UserModel, error) {
		return []base.UserModel{{
			Name:      "model1",
			Qualifier: "prod",
			UUID:      mockControllerUUID,
			Type:      model.IAAS,
		}, {
			Name:      "model2",
			Qualifier: "prod",
			UUID:      "eeeeeeee-12e9-11e4-8a70-b2227cce2b55",
			Type:      model.IAAS,
		}}, nil
	}
	prompter := cmdtesting.NewSeqPrompter(c, "»", `
Enter a new password: »hunter2

Confirm password: »hunter2

Enter a name for this controller \[controller-name\]: »
Initial password successfully set for bob.

Welcome, bob. You are now logged into "controller-name".

There are 2 models available. Use "juju switch" to select
one of them:
  - juju switch prod/model1
  - juju switch prod/model2
`[1:])
	defer prompter.CheckDone()
	s.testRegisterSuccess(c, prompter, "", false, false)

	// When there are multiple models, no current model will be set.
	// Instead, the command will output the list of models and inform
	// the user how to set the current model.
	_, err := s.store.CurrentModel("controller-name")
	c.Assert(err, tc.ErrorIs, errors.NotFound)
}

// testRegisterSuccess tests that the register command when the given
// stdio instance is used for input and output. If stdio is nil, a
// default prompter will be used.
// If controllerName is non-empty, that name will be expected
// to be the name of the registered controller.
func (s *RegisterSuite) testRegisterSuccess(c *tc.C, stdio io.ReadWriter, controllerName string, withProxy, replace bool) {
	if controllerName == "" {
		controllerName = "controller-name"
	}

	var proxy *params.Proxy
	registrationInfo := jujuclient.RegistrationInfo{
		User:           "bob",
		SecretKey:      mockSecretKey,
		ControllerName: "controller-name",
	}
	rawConfig := map[string]interface{}{
		"api-host":              "https://127.0.0.1:16443",
		"ca-cert":               "cert====",
		"namespace":             "controller-controller-name",
		"remote-port":           "17070",
		"service":               "controller-service",
		"service-account-token": "token====",
	}
	if withProxy {
		proxy = &params.Proxy{Type: "kubernetes-port-forward", Config: rawConfig}
		registrationInfo.ProxyConfig = `
type: kubernetes-port-forward
config:
    api-host: https://127.0.0.1:443
    namespace: controller-controller-name
    remote-port: "17070"
    service: controller-service
    service-account-token: token====
`[1:]
	}
	srv := s.mockServer(c, proxy)
	s.httpHandler = srv

	registrationData := s.encodeRegistrationData(c, registrationInfo)
	c.Logf("registration data: %q", registrationData)
	if stdio == nil {
		prompter := cmdtesting.NewSeqPrompter(c, "»", `
Enter a new password: »hunter2

Confirm password: »hunter2

Enter a name for this controller \[controller-name\]: »
Initial password successfully set for bob.

Welcome, bob. You are now logged into "controller-name".
`[1:]+noModelsText)
		defer prompter.CheckDone()
		stdio = prompter
	}
	args := []string{registrationData}
	if replace {
		args = append(args, "--replace")
	}
	err := s.run(c, stdio, args...)
	c.Assert(err, tc.ErrorIsNil)

	// There should have been one POST command to "/register".
	c.Assert(srv.requests, tc.HasLen, 1)
	c.Assert(srv.requests[0].Method, tc.Equals, "POST")
	c.Assert(srv.requests[0].URL.Path, tc.Equals, "/register")
	var request params.SecretKeyLoginRequest
	err = json.Unmarshal(srv.requestBodies[0], &request)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(request.User, tc.DeepEquals, "user-bob")
	c.Assert(request.Nonce, tc.HasLen, 24)
	requestPayloadPlaintext, err := json.Marshal(params.SecretKeyLoginRequestPayload{
		Password: "hunter2",
	})
	c.Assert(err, tc.ErrorIsNil)
	expectedCiphertext := s.seal(c, requestPayloadPlaintext, mockSecretKey, request.Nonce)
	c.Assert(request.PayloadCiphertext, tc.DeepEquals, expectedCiphertext)

	// The controller and account details should be recorded with
	// the specified controller name and user
	// name from the registration string.

	controller, err := s.store.ControllerByName(controllerName)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(controller.ControllerUUID, tc.Equals, mockControllerUUID)
	c.Assert(controller.APIEndpoints, tc.DeepEquals, []string{s.apiConnection.addr.String()})
	c.Assert(controller.CACert, tc.DeepEquals, testing.CACert)
	if withProxy {
		c.Assert(controller.Proxy.Proxier.Type(), tc.Equals, "kubernetes-port-forward")
		rcfg, err := controller.Proxy.Proxier.RawConfig()
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(rcfg, tc.DeepEquals, rawConfig)
	}
	account, err := s.store.AccountDetails(controllerName)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(account, tc.DeepEquals, &jujuclient.AccountDetails{
		User:            "bob",
		LastKnownAccess: "login",
	})
}

func (s *RegisterSuite) TestRegisterInvalidRegistrationData(c *tc.C) {
	err := s.run(c, nil, "not base64")
	c.Assert(err, tc.ErrorMatches, "invalid registration token: illegal base64 data at input byte 3")

	err = s.run(c, nil, "YXNuLjEK")
	c.Assert(err, tc.ErrorMatches, "asn1: structure error: .*")
}

func (s *RegisterSuite) TestRegisterEmptyControllerName(c *tc.C) {
	srv := s.mockServer(c, nil)
	s.httpHandler = srv
	registrationData := s.encodeRegistrationData(c, jujuclient.RegistrationInfo{
		User:      "bob",
		SecretKey: mockSecretKey,
	})
	// We check that it loops when an empty controller name
	// is entered and that the loop terminates when the user
	// types ^D.
	prompter := cmdtesting.NewSeqPrompter(c, "»", `
Enter a new password: »hunter2

Confirm password: »hunter2

Enter a name for this controller: »
You must specify a non-empty controller name.
Enter a name for this controller: »
You must specify a non-empty controller name.
Enter a name for this controller: »»
`[1:])
	err := s.run(c, prompter, registrationData)
	c.Assert(err, tc.ErrorMatches, "EOF")
	prompter.AssertDone()
}

func (s *RegisterSuite) TestRegisterControllerNameExists(c *tc.C) {
	err := s.store.AddController("controller-name", jujuclient.ControllerDetails{
		ControllerUUID: "0d75314a-5266-4f4f-8523-415be76f92dc",
		CACert:         testing.CACert,
	})
	c.Assert(err, tc.ErrorIsNil)
	prompter := cmdtesting.NewSeqPrompter(c, "»", `
Enter a new password: »hunter2

Confirm password: »hunter2

Enter a name for this controller: »controller-name
Controller "controller-name" already exists.
Enter a name for this controller: »other-name
Initial password successfully set for bob.

Welcome, bob. You are now logged into "other-name".
`[1:]+noModelsText)
	s.testRegisterSuccess(c, prompter, "other-name", false, false)
	prompter.AssertDone()
}

func (s *RegisterSuite) TestControllerUUIDExists(c *tc.C) {
	// Controller has the UUID from s.testRegister to mimic a user with
	// this controller already registered (regardless of its name).
	err := s.store.AddController("controller-name", jujuclient.ControllerDetails{
		ControllerUUID: mockControllerUUID,
		CACert:         testing.CACert,
	})
	c.Assert(err, tc.ErrorIsNil)

	s.listModels = func(ctx context.Context, _ jujuclient.ClientStore, controllerName, userName string) ([]base.UserModel, error) {
		return []base.UserModel{{
			Name:      "model-name",
			Qualifier: "prod",
			UUID:      mockControllerUUID,
			Type:      model.IAAS,
		}}, nil
	}

	registrationData := s.encodeRegistrationData(c, jujuclient.RegistrationInfo{
		User:           "bob",
		SecretKey:      mockSecretKey,
		ControllerName: "controller-name",
	})

	srv := s.mockServer(c, nil)
	s.httpHandler = srv

	prompter := cmdtesting.NewSeqPrompter(c, "»", `
Enter a new password: »hunter2

Confirm password: »hunter2

Enter a name for this controller: »foo
Initial password successfully set for bob.
`[1:])
	err = s.run(c, prompter, registrationData)
	c.Assert(err, tc.Not(tc.IsNil))
	c.Assert(err.Error(), tc.Equals, `This controller has already been registered on this client as "controller-name".
To login as user "bob" run 'juju login -u bob -c controller-name'.
To update controller details and login as user "bob":
    1. run 'juju unregister controller-name'
    2. request from your controller admin another registration string, i.e
       output from 'juju change-user-password bob --reset'
    3. re-run 'juju register' with the registration string from (2) above.
`)
	prompter.CheckDone()
}

func (s *RegisterSuite) TestReplaceLoggedInController(c *tc.C) {
	// Ensure that, if the user is already logged in to the controller being replaced, we raise
	// an error prompting them to log out first
	controllerName := "controller-name"
	err := s.store.AddController(controllerName, jujuclient.ControllerDetails{
		ControllerUUID: mockControllerUUID,
		CACert:         testing.CACert,
	})
	c.Assert(err, tc.ErrorIsNil)

	accountDetails := jujuclient.AccountDetails{User: "bob"}
	err = s.store.UpdateAccount(controllerName, accountDetails)
	c.Assert(err, tc.ErrorIsNil)

	registrationData := s.encodeRegistrationData(c, jujuclient.RegistrationInfo{
		User:           "mary",
		SecretKey:      mockSecretKey,
		ControllerName: controllerName,
	})

	srv := s.mockServer(c, nil)
	s.httpHandler = srv

	prompter := cmdtesting.NewSeqPrompter(c, "»", `
Enter a new password: »hunter2

Confirm password: »hunter2

Enter a name for this controller \[replace controller-name\]: »controller-name
Initial password successfully set for mary.
`[1:])
	err = s.run(c, prompter, registrationData, "--replace")
	c.Assert(err, tc.Not(tc.IsNil))
	c.Assert(err.Error(), tc.Equals, `User "bob" is currently logged into controller "controller-name".
Cannot replace a controller we're currently logged into.
To register and replace this controller:
    1. run 'juju logout -c controller-name'
    2. request from your controller admin another registration string, i.e
       output from 'juju change-user-password mary --reset'
    3. re-run 'juju register TOKEN --replace' with the registration string from (2) above.
`)
	prompter.CheckDone()
}

func (s *RegisterSuite) TestProposedControllerNameExists(c *tc.C) {
	// Controller does not have the UUID from s.testRegister, thereby
	// mimicing a user with an already registered 'foreign' controller.
	err := s.store.AddController("controller-name", jujuclient.ControllerDetails{
		ControllerUUID: "0d75314a-5266-4f4f-8523-415be76f92dc",
		CACert:         testing.CACert,
	})
	c.Assert(err, tc.ErrorIsNil)

	s.listModels = func(ctx context.Context, _ jujuclient.ClientStore, controllerName, userName string) ([]base.UserModel, error) {
		return []base.UserModel{{
			Name:      "model-name",
			Qualifier: "prod",
			UUID:      mockControllerUUID,
			Type:      model.IAAS,
		}}, nil
	}

	prompter := cmdtesting.NewSeqPrompter(c, "»", `
Enter a new password: »hunter2

Confirm password: »hunter2

Enter a name for this controller: »controller-name
Controller "controller-name" already exists.
Enter a name for this controller: »other-name
Initial password successfully set for bob.

Welcome, bob. You are now logged into "other-name".

Current model set to "prod/model-name".
`[1:])
	defer prompter.CheckDone()
	s.testRegisterSuccess(c, prompter, "other-name", false, false)
}

func (s *RegisterSuite) TestRegisterControllerNameExistsReplace(c *tc.C) {
	err := s.store.AddController("controller-name", jujuclient.ControllerDetails{
		ControllerUUID: "0d75314a-5266-4f4f-8523-415be76f92dc",
		CACert:         testing.CACert,
	})
	c.Assert(err, tc.ErrorIsNil)
	prompter := cmdtesting.NewSeqPrompter(c, "»", `
Enter a new password: »hunter2

Confirm password: »hunter2

Enter a name for this controller \[replace controller-name\]: »
Initial password successfully set for bob.

Welcome, bob. You are now logged into "controller-name".
`[1:]+noModelsText)
	s.testRegisterSuccess(c, prompter, "controller-name", false, true)
	prompter.AssertDone()
}

func (s *RegisterSuite) TestControllerUUIDExistsReplace(c *tc.C) {
	// Controller has the UUID from s.testRegister to mimic a user with
	// this controller already registered (regardless of its name).
	err := s.store.AddController("controller-name", jujuclient.ControllerDetails{
		ControllerUUID: mockControllerUUID,
		CACert:         testing.CACert,
	})
	c.Assert(err, tc.ErrorIsNil)

	s.listModels = func(ctx context.Context, _ jujuclient.ClientStore, controllerName, userName string) ([]base.UserModel, error) {
		return []base.UserModel{{
			Name:      "model-name",
			Qualifier: "prod",
			UUID:      mockControllerUUID,
			Type:      model.IAAS,
		}}, nil
	}

	srv := s.mockServer(c, nil)
	s.httpHandler = srv

	prompter := cmdtesting.NewSeqPrompter(c, "»", `
Enter a new password: »hunter2

Confirm password: »hunter2

Enter a name for this controller \[replace controller-name\]: »controller-name
Initial password successfully set for bob.

Welcome, bob. You are now logged into "controller-name".

Current model set to "prod/model-name".
`[1:])
	s.testRegisterSuccess(c, prompter, "controller-name", false, true)
	prompter.CheckDone()
}

func (s *RegisterSuite) TestControllerUUIDExistsRenameNotAllowed(c *tc.C) {
	// Controller has the UUID from s.testRegister to mimic a user with
	// this controller already registered (regardless of its name).
	err := s.store.AddController("controller-name", jujuclient.ControllerDetails{
		ControllerUUID: mockControllerUUID,
		CACert:         testing.CACert,
	})
	c.Assert(err, tc.ErrorIsNil)

	s.listModels = func(ctx context.Context, _ jujuclient.ClientStore, controllerName, userName string) ([]base.UserModel, error) {
		return []base.UserModel{{
			Name:      "model-name",
			Qualifier: "prod",
			UUID:      mockControllerUUID,
			Type:      model.IAAS,
		}}, nil
	}

	registrationData := s.encodeRegistrationData(c, jujuclient.RegistrationInfo{
		User:           "bob",
		SecretKey:      mockSecretKey,
		ControllerName: "controller-name",
	})

	srv := s.mockServer(c, nil)
	s.httpHandler = srv

	prompter := cmdtesting.NewSeqPrompter(c, "»", `
Enter a new password: »hunter2

Confirm password: »hunter2

Enter a name for this controller \[replace controller-name\]: »foo
Initial password successfully set for bob.
`[1:])
	err = s.run(c, prompter, registrationData, "--replace")
	c.Assert(err, tc.Not(tc.IsNil))
	c.Assert(err.Error(), tc.Equals, `This controller has already been registered on this client as "controller-name".
To login as user "bob" run 'juju login -u bob -c controller-name'.
To update controller details and login as user "bob":
    1. run 'juju unregister controller-name'
    2. request from your controller admin another registration string, i.e
       output from 'juju change-user-password bob --reset'
    3. re-run 'juju register' with the registration string from (2) above.
`)
	prompter.CheckDone()
}

func (s *RegisterSuite) TestRegisterEmptyPassword(c *tc.C) {
	registrationData := s.encodeRegistrationData(c, jujuclient.RegistrationInfo{
		User:      "bob",
		SecretKey: mockSecretKey,
	})
	prompter := cmdtesting.NewSeqPrompter(c, "»", `
Enter a new password: »

`[1:])
	defer prompter.CheckDone()
	err := s.run(c, prompter, registrationData)
	c.Assert(err, tc.ErrorMatches, "you must specify a non-empty password")
}

func (s *RegisterSuite) TestRegisterPasswordMismatch(c *tc.C) {
	registrationData := s.encodeRegistrationData(c, jujuclient.RegistrationInfo{
		User:      "bob",
		SecretKey: mockSecretKey,
	})
	prompter := cmdtesting.NewSeqPrompter(c, "»", `
Enter a new password: »hunter2

Confirm password: »hunter3

`[1:])
	defer prompter.CheckDone()
	err := s.run(c, prompter, registrationData)
	c.Assert(err, tc.ErrorMatches, "passwords do not match")
}

func (s *RegisterSuite) TestAPIOpenError(c *tc.C) {
	registrationData := s.encodeRegistrationData(c, jujuclient.RegistrationInfo{
		User:      "bob",
		SecretKey: mockSecretKey,
	})
	prompter := cmdtesting.NewSeqPrompter(c, "»", `
Enter a new password: »hunter2

Confirm password: »hunter2

Enter a name for this controller: »foo
`[1:])
	defer prompter.CheckDone()
	s.apiOpenError = errors.New("open failed")
	err := s.run(c, prompter, registrationData)
	//c.Assert(c.GetTestLog(), tc.Matches, "(.|\n)*open failed(.|\n)*")
	controllerURL := s.apiConnection.Addr()
	c.Assert(err, tc.ErrorMatches, `Cannot reach controller "foo" at: `+controllerURL.String()+".\n"+
		"Check that the controller ip is reachable from your network.")
}

func (s *RegisterSuite) TestRegisterServerError(c *tc.C) {
	response, err := json.Marshal(params.ErrorResult{
		Error: &params.Error{Message: "xyz", Code: "123"},
	})

	s.httpHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, err = w.Write(response)
		c.Check(err, tc.ErrorIsNil)
	})
	prompter := cmdtesting.NewSeqPrompter(c, "»", `
Enter a new password: »hunter2

Confirm password: »hunter2

Enter a name for this controller: »foo

`[1:])

	registrationData := s.encodeRegistrationData(c, jujuclient.RegistrationInfo{
		User:      "bob",
		SecretKey: mockSecretKey,
	})
	err = s.run(c, prompter, registrationData)
	//c.Assert(c.GetTestLog(), tc.Matches, "(.|\n)* xyz(.|\n)*")
	c.Assert(err, tc.ErrorMatches, `
Provided registration token may have expired.
A controller administrator must reset your user to issue a new token.
See "juju help change-user-password" for more information.`[1:])

	// Check that the controller hasn't been added.
	_, err = s.store.ControllerByName("controller-name")
	c.Assert(err, tc.ErrorIs, errors.NotFound)
}

func (s *RegisterSuite) TestRegisterPublic(c *tc.C) {
	s.apiConnection.authTag = names.NewUserTag("bob@external")
	s.apiConnection.controllerAccess = "login"
	prompter := cmdtesting.NewSeqPrompter(c, "»", `
Enter a name for this controller: »public-controller-name

Welcome, bob@external. You are now logged into "public-controller-name".
`[1:]+noModelsText)
	defer prompter.CheckDone()
	err := s.run(c, prompter, "0.1.2.3")
	c.Assert(err, tc.ErrorIsNil)

	// The controller and account details should be recorded with
	// the specified controller name and user
	// name from the auth tag.

	controller, err := s.store.ControllerByName("public-controller-name")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(controller, tc.DeepEquals, &jujuclient.ControllerDetails{
		ControllerUUID: mockControllerUUID,
		APIEndpoints:   []string{"0.1.2.3:443"},
	})
	account, err := s.store.AccountDetails("public-controller-name")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(account, tc.DeepEquals, &jujuclient.AccountDetails{
		User:            "bob@external",
		LastKnownAccess: "login",
	})
}

func (s *RegisterSuite) TestRegisterAlreadyKnownPublicControllerEndpoint(c *tc.C) {
	prompter := cmdtesting.NewSeqPrompter(c, "»", "")
	defer prompter.CheckDone()

	err := s.store.AddController("foo", jujuclient.ControllerDetails{
		APIEndpoints:   []string{"foo.com:17070"},
		ControllerUUID: "0d75314a-5266-4f4f-8523-415be76f92dc",
		CACert:         testing.CACert,
	})
	c.Assert(err, tc.ErrorIsNil)

	err = s.run(c, prompter, "foo.com:17070")
	c.Assert(err, tc.Not(tc.IsNil))
	c.Assert(err.Error(), tc.Equals, `A controller with the same hostname has already been registered on this client as "foo".
To login run 'juju login -c foo'.`)
}

func (s *RegisterSuite) TestRegisterAlreadyKnownControllerEndpointWithReplace(c *tc.C) {
	controllerName := "controller-name"
	err := s.store.AddController(controllerName, jujuclient.ControllerDetails{
		APIEndpoints:   []string{"42.42.42.42:17070"},
		ControllerUUID: mockControllerUUID,
		CACert:         testing.CACert,
	})
	c.Assert(err, tc.ErrorIsNil)

	registrationData := s.encodeRegistrationDataWithAddrs(c, jujuclient.RegistrationInfo{
		User:           "bob",
		SecretKey:      mockSecretKey,
		ControllerName: controllerName,
		Addrs:          []string{"42.42.42.42:17070"},
	})

	srv := s.mockServer(c, nil)
	s.httpHandler = srv

	prompter := cmdtesting.NewSeqPrompter(c, "»", `
Enter a new password: »hunter2

Confirm password: »hunter2

Enter a name for this controller \[replace controller-name\]: »controller-name
Initial password successfully set for bob.

Welcome, bob. You are now logged into "controller-name".
`[1:]+noModelsText)
	err = s.run(c, prompter, registrationData, "--replace")
	c.Assert(err, tc.ErrorIsNil)
	prompter.CheckDone()
}

func (s *RegisterSuite) TestRegisterAlreadyKnownControllerEndpointAndUser(c *tc.C) {
	prompter := cmdtesting.NewSeqPrompter(c, "»", "")
	defer prompter.CheckDone()

	err := s.store.AddController("foo", jujuclient.ControllerDetails{
		APIEndpoints:   []string{"42.42.42.42:17070"},
		ControllerUUID: "0d75314a-5266-4f4f-8523-415be76f92dc",
		CACert:         testing.CACert,
	})
	c.Assert(err, tc.ErrorIsNil)

	s.store.Accounts["foo"] = jujuclient.AccountDetails{
		User: "bob",
	}

	err = s.run(c, prompter, "42.42.42.42:17070")
	c.Assert(err, tc.Not(tc.IsNil))
	c.Assert(err.Error(), tc.Equals, `A controller with the same address has already been registered on this client as "foo".
You are already logged in as user "bob".
To update controller details:
    1. run 'juju logout'
    2. re-run 'juju register --replace' with your registration string.
`)
}

func (s *RegisterSuite) TestRegisterAlreadyKnownControllerEndpointAndUserByBase64(c *tc.C) {
	prompter := cmdtesting.NewSeqPrompter(c, "»", "")
	defer prompter.CheckDone()

	registrationInfo := jujuclient.RegistrationInfo{
		User:      "mary",
		SecretKey: mockSecretKey,
		Addrs:     []string{"42.42.42.42:17070"},
	}

	registrationData := s.encodeRegistrationDataWithAddrs(c, registrationInfo)
	c.Logf("registration data: %q", registrationData)

	err := s.store.AddController("foo", jujuclient.ControllerDetails{
		APIEndpoints:   []string{"42.42.42.42:17070"},
		ControllerUUID: "0d75314a-5266-4f4f-8523-415be76f92dc",
		CACert:         testing.CACert,
	})
	c.Assert(err, tc.ErrorIsNil)

	s.store.Accounts["foo"] = jujuclient.AccountDetails{
		User: "bob",
	}

	args := []string{registrationData}
	err = s.run(c, prompter, args...)
	c.Assert(err, tc.Not(tc.IsNil))
	c.Assert(err.Error(), tc.Equals, `A controller with the same address has already been registered on this client as "foo".
You are already logged in as user "bob".
To update controller details and login as user "mary":
    1. run 'juju logout'
    2. re-run 'juju register --replace' with your registration string.
`)
}

func (s *RegisterSuite) TestRegisterPublicAPIOpenError(c *tc.C) {
	s.apiOpenError = errors.New("open failed")
	prompter := cmdtesting.NewSeqPrompter(c, "»", `
Enter a name for this controller: »public-controller-name
`[1:])
	defer prompter.CheckDone()
	err := s.run(c, prompter, "0.1.2.3")
	c.Assert(err, tc.ErrorMatches, `open failed`)
}

func (s *RegisterSuite) TestRegisterPublicWithPort(c *tc.C) {
	s.apiConnection.authTag = names.NewUserTag("bob@external")
	s.apiConnection.controllerAccess = "login"
	prompter := cmdtesting.NewSeqPrompter(c, "»", `
Enter a name for this controller: »public-controller-name

Welcome, bob@external. You are now logged into "public-controller-name".
`[1:]+noModelsText)
	defer prompter.CheckDone()
	err := s.run(c, prompter, "0.1.2.3:5678")
	c.Assert(err, tc.ErrorIsNil)

	// The controller and account details should be recorded with
	// the specified controller name and user
	// name from the auth tag.

	controller, err := s.store.ControllerByName("public-controller-name")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(controller, tc.DeepEquals, &jujuclient.ControllerDetails{
		ControllerUUID: mockControllerUUID,
		APIEndpoints:   []string{"0.1.2.3:5678"},
	})
}

type mockServer struct {
	requests      []*http.Request
	requestBodies [][]byte
	response      []byte
}

const mockControllerUUID = "df136476-12e9-11e4-8a70-b2227cce2b54"

var mockSecretKey = []byte(strings.Repeat("X", 32))

// mockServer returns a mock HTTP server that will always respond with a
// response encoded with mockSecretKey and a constant nonce, containing
// testing.CACert and mockControllerUUID.
//
// Each time a call is made, the requests and requestBodies fields in
// the returned mockServer instance are appended with the request details.
func (s *RegisterSuite) mockServer(c *tc.C, proxy *params.Proxy) *mockServer {
	respNonce := []byte(strings.Repeat("X", 24))

	responsePayloadPlaintext, err := json.Marshal(params.SecretKeyLoginResponsePayload{
		CACert:         testing.CACert,
		ControllerUUID: mockControllerUUID,
		ProxyConfig:    proxy,
	})
	c.Assert(err, tc.ErrorIsNil)
	response, err := json.Marshal(params.SecretKeyLoginResponse{
		Nonce:             respNonce,
		PayloadCiphertext: s.seal(c, responsePayloadPlaintext, mockSecretKey, respNonce),
	})
	c.Assert(err, tc.ErrorIsNil)
	return &mockServer{
		response: response,
	}
}

// encodeRegistrationData encodes the given registration info into a base64 string, replacing the RegistrationInfo.Addrs
// field with the default address.
func (s *RegisterSuite) encodeRegistrationData(c *tc.C, info jujuclient.RegistrationInfo) string {
	info.Addrs = []string{s.apiConnection.addr.String()}
	return s.encodeRegistrationDataWithAddrs(c, info)
}

// encodeRegistrationData encodes the given registration info into a base64 string.
func (s *RegisterSuite) encodeRegistrationDataWithAddrs(c *tc.C, info jujuclient.RegistrationInfo) string {
	data, err := asn1.Marshal(info)
	c.Assert(err, tc.ErrorIsNil)
	// Append some junk to the end of the encoded data to
	// ensure that, if we have to pad the data in add-user,
	// register can still decode it.
	data = append(data, 0, 0, 0)
	return base64.URLEncoding.EncodeToString(data)
}

func (srv *mockServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get(httpbakery.BakeryProtocolHeader) != "3" {
		http.Error(w, "unexpected bakery version", http.StatusInternalServerError)
		return
	}
	srv.requests = append(srv.requests, r)
	requestBody, err := io.ReadAll(r.Body)
	if err != nil {
		panic(err)
	}
	srv.requestBodies = append(srv.requestBodies, requestBody)
	_, err = w.Write(srv.response)
	if err != nil {
		panic(err)
	}
}

func (s *RegisterSuite) apiOpen(ctx context.Context, info *api.Info, opts api.DialOpts) (api.Connection, error) {
	if s.apiOpenError != nil {
		return nil, s.apiOpenError
	}
	return s.apiConnection, nil
}

func (s *RegisterSuite) run(c *tc.C, stdio io.ReadWriter, args ...string) error {
	if stdio == nil {
		p := noPrompts(c)
		stdio = p
		defer p.CheckDone()
	}

	command := controller.NewRegisterCommandForTest(s.apiOpen, s.listModels, s.store)
	err := cmdtesting.InitCommand(command, args)
	c.Assert(err, tc.ErrorIsNil)
	return command.Run(&cmd.Context{
		Dir:    c.MkDir(),
		Stdin:  stdio,
		Stdout: stdio,
		Stderr: stdio,
	})
}

func noPrompts(c *tc.C) *cmdtesting.SeqPrompter {
	return cmdtesting.NewSeqPrompter(c, "»", "")
}

func (s *RegisterSuite) seal(c *tc.C, message, key, nonce []byte) []byte {
	var keyArray [32]byte
	var nonceArray [24]byte
	c.Assert(copy(keyArray[:], key), tc.Equals, len(keyArray))
	c.Assert(copy(nonceArray[:], nonce), tc.Equals, len(nonceArray))
	return secretbox.Seal(nil, message, &nonceArray, &keyArray)
}
