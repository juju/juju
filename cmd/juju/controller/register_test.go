// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller_test

import (
	"encoding/asn1"
	"encoding/base64"
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"golang.org/x/crypto/nacl/secretbox"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon-bakery.v2/httpbakery"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/controller"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/testing"
)

type RegisterSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	apiConnection            *mockAPIConnection
	store                    *jujuclient.MemStore
	apiOpenError             error
	listModels               func(jujuclient.ClientStore, string, string) ([]base.UserModel, error)
	listModelsControllerName string
	listModelsUserName       string
	server                   *httptest.Server
	httpHandler              http.Handler
}

const noModelsText = `
There are no models available. You can add models with
"juju add-model", or you can ask an administrator or owner
of a model to grant access to that model with "juju grant".
`

var _ = gc.Suite(&RegisterSuite{})

func (s *RegisterSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)

	s.apiOpenError = nil
	s.httpHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	s.server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.httpHandler.ServeHTTP(w, r)
	}))

	serverURL, err := url.Parse(s.server.URL)
	c.Assert(err, jc.ErrorIsNil)
	s.apiConnection = &mockAPIConnection{
		controllerTag: names.NewControllerTag(mockControllerUUID),
		addr:          serverURL.Host,
	}
	s.listModelsControllerName = ""
	s.listModelsUserName = ""
	s.listModels = func(_ jujuclient.ClientStore, controllerName, userName string) ([]base.UserModel, error) {
		s.listModelsControllerName = controllerName
		s.listModelsUserName = userName
		return nil, nil
	}

	s.store = jujuclient.NewMemStore()
}

func (s *RegisterSuite) TearDownTest(c *gc.C) {
	s.server.Close()
	s.FakeJujuXDGDataHomeSuite.TearDownTest(c)
}

func (s *RegisterSuite) TestInit(c *gc.C) {
	registerCommand := controller.NewRegisterCommandForTest(nil, nil, nil)

	err := cmdtesting.InitCommand(registerCommand, []string{})
	c.Assert(err, gc.ErrorMatches, "registration data missing")

	err = cmdtesting.InitCommand(registerCommand, []string{"foo", "bar"})
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["bar"\]`)
}

func (s *RegisterSuite) TestRegister(c *gc.C) {
	s.testRegisterSuccess(c, nil, "")
	c.Assert(s.listModelsControllerName, gc.Equals, "controller-name")
	c.Assert(s.listModelsUserName, gc.Equals, "bob")
}

func (s *RegisterSuite) TestRegisterOneModel(c *gc.C) {
	s.listModels = func(_ jujuclient.ClientStore, controllerName, userName string) ([]base.UserModel, error) {
		return []base.UserModel{{
			Name:  "theoneandonly",
			Owner: "carol",
			UUID:  mockControllerUUID,
			Type:  model.IAAS,
		}}, nil
	}
	prompter := cmdtesting.NewSeqPrompter(c, "»", `
Enter a new password: »hunter2

Confirm password: »hunter2

Enter a name for this controller \[controller-name\]: »
Initial password successfully set for bob.

Welcome, bob. You are now logged into "controller-name".

Current model set to "carol/theoneandonly".
`[1:])
	s.testRegisterSuccess(c, prompter, "")
	c.Assert(
		s.store.Models["controller-name"].CurrentModel,
		gc.Equals, "carol/theoneandonly",
	)
	prompter.CheckDone()
}

func (s *RegisterSuite) TestRegisterMultipleModels(c *gc.C) {
	s.listModels = func(_ jujuclient.ClientStore, controllerName, userName string) ([]base.UserModel, error) {
		return []base.UserModel{{
			Name:  "model1",
			Owner: "bob",
			UUID:  mockControllerUUID,
			Type:  model.IAAS,
		}, {
			Name:  "model2",
			Owner: "bob",
			UUID:  "eeeeeeee-12e9-11e4-8a70-b2227cce2b55",
			Type:  model.IAAS,
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
  - juju switch model1
  - juju switch model2
`[1:])
	defer prompter.CheckDone()
	s.testRegisterSuccess(c, prompter, "")

	// When there are multiple models, no current model will be set.
	// Instead, the command will output the list of models and inform
	// the user how to set the current model.
	_, err := s.store.CurrentModel("controller-name")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

// testRegisterSuccess tests that the register command when the given
// stdio instance is used for input and output. If stdio is nil, a
// default prompter will be used.
// If controllerName is non-empty, that name will be expected
// to be the name of the registered controller.
func (s *RegisterSuite) testRegisterSuccess(c *gc.C, stdio io.ReadWriter, controllerName string) {
	srv := s.mockServer(c)
	s.httpHandler = srv

	if controllerName == "" {
		controllerName = "controller-name"
	}

	registrationData := s.encodeRegistrationData(c, jujuclient.RegistrationInfo{
		User:           "bob",
		SecretKey:      mockSecretKey,
		ControllerName: "controller-name",
	})
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
	err := s.run(c, stdio, registrationData)
	c.Assert(err, jc.ErrorIsNil)

	// There should have been one POST command to "/register".
	c.Assert(srv.requests, gc.HasLen, 1)
	c.Assert(srv.requests[0].Method, gc.Equals, "POST")
	c.Assert(srv.requests[0].URL.Path, gc.Equals, "/register")
	var request params.SecretKeyLoginRequest
	err = json.Unmarshal(srv.requestBodies[0], &request)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(request.User, jc.DeepEquals, "user-bob")
	c.Assert(request.Nonce, gc.HasLen, 24)
	requestPayloadPlaintext, err := json.Marshal(params.SecretKeyLoginRequestPayload{
		"hunter2",
	})
	c.Assert(err, jc.ErrorIsNil)
	expectedCiphertext := s.seal(c, requestPayloadPlaintext, mockSecretKey, request.Nonce)
	c.Assert(request.PayloadCiphertext, jc.DeepEquals, expectedCiphertext)

	// The controller and account details should be recorded with
	// the specified controller name and user
	// name from the registration string.

	controller, err := s.store.ControllerByName(controllerName)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(controller, jc.DeepEquals, &jujuclient.ControllerDetails{
		ControllerUUID: mockControllerUUID,
		APIEndpoints:   []string{s.apiConnection.addr},
		CACert:         testing.CACert,
	})
	account, err := s.store.AccountDetails(controllerName)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(account, jc.DeepEquals, &jujuclient.AccountDetails{
		User:            "bob",
		LastKnownAccess: "login",
	})
}

func (s *RegisterSuite) TestRegisterInvalidRegistrationData(c *gc.C) {
	err := s.run(c, nil, "not base64")
	c.Assert(err, gc.ErrorMatches, "illegal base64 data at input byte 3")

	err = s.run(c, nil, "YXNuLjEK")
	c.Assert(err, gc.ErrorMatches, "asn1: structure error: .*")
}

func (s *RegisterSuite) TestRegisterEmptyControllerName(c *gc.C) {
	srv := s.mockServer(c)
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
	c.Assert(err, gc.ErrorMatches, "EOF")
	prompter.AssertDone()
}

func (s *RegisterSuite) TestRegisterControllerNameExists(c *gc.C) {
	err := s.store.AddController("controller-name", jujuclient.ControllerDetails{
		ControllerUUID: "0d75314a-5266-4f4f-8523-415be76f92dc",
		CACert:         testing.CACert,
	})
	c.Assert(err, jc.ErrorIsNil)
	prompter := cmdtesting.NewSeqPrompter(c, "»", `
Enter a new password: »hunter2

Confirm password: »hunter2

Enter a name for this controller: »controller-name
Controller "controller-name" already exists.
Enter a name for this controller: »other-name
Initial password successfully set for bob.

Welcome, bob. You are now logged into "other-name".
`[1:]+noModelsText)
	s.testRegisterSuccess(c, prompter, "other-name")
	prompter.AssertDone()
}

func (s *RegisterSuite) TestControllerUUIDExists(c *gc.C) {
	// Controller has the UUID from s.testRegister to mimic a user with
	// this controller already registered (regardless of its name).
	err := s.store.AddController("controller-name", jujuclient.ControllerDetails{
		ControllerUUID: mockControllerUUID,
		CACert:         testing.CACert,
	})

	s.listModels = func(_ jujuclient.ClientStore, controllerName, userName string) ([]base.UserModel, error) {
		return []base.UserModel{{
			Name:  "model-name",
			Owner: "bob",
			UUID:  mockControllerUUID,
			Type:  model.IAAS,
		}}, nil
	}

	registrationData := s.encodeRegistrationData(c, jujuclient.RegistrationInfo{
		User:           "bob",
		SecretKey:      mockSecretKey,
		ControllerName: "controller-name",
	})

	srv := s.mockServer(c)
	s.httpHandler = srv

	prompter := cmdtesting.NewSeqPrompter(c, "»", `
Enter a new password: »hunter2

Confirm password: »hunter2

Enter a name for this controller: »foo
Initial password successfully set for bob.
`[1:])
	err = s.run(c, prompter, registrationData)
	c.Assert(err, gc.Not(gc.IsNil))
	c.Assert(err.Error(), gc.Equals, `This controller has already been registered on this client as "controller-name".
To login user "bob" run 'juju login -u bob -c controller-name'.
To update controller details and login as user "bob":
    1. run 'juju unregister controller-name'
    2. request from your controller admin another registration string, i.e
       output from 'juju change-user-password bob --reset'
    3. re-run 'juju register' with the registration string from (2) above.
`)
	prompter.CheckDone()
}

func (s *RegisterSuite) TestProposedControllerNameExists(c *gc.C) {
	// Controller does not have the UUID from s.testRegister, thereby
	// mimicing a user with an already registered 'foreign' controller.
	err := s.store.AddController("controller-name", jujuclient.ControllerDetails{
		ControllerUUID: "0d75314a-5266-4f4f-8523-415be76f92dc",
		CACert:         testing.CACert,
	})
	c.Assert(err, jc.ErrorIsNil)

	s.listModels = func(_ jujuclient.ClientStore, controllerName, userName string) ([]base.UserModel, error) {
		return []base.UserModel{{
			Name:  "model-name",
			Owner: "bob",
			UUID:  mockControllerUUID,
			Type:  model.IAAS,
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

Current model set to "bob/model-name".
`[1:])
	defer prompter.CheckDone()
	s.testRegisterSuccess(c, prompter, "other-name")
}

func (s *RegisterSuite) TestRegisterEmptyPassword(c *gc.C) {
	registrationData := s.encodeRegistrationData(c, jujuclient.RegistrationInfo{
		User:      "bob",
		SecretKey: mockSecretKey,
	})
	prompter := cmdtesting.NewSeqPrompter(c, "»", `
Enter a new password: »

`[1:])
	defer prompter.CheckDone()
	err := s.run(c, prompter, registrationData)
	c.Assert(err, gc.ErrorMatches, "you must specify a non-empty password")
}

func (s *RegisterSuite) TestRegisterPasswordMismatch(c *gc.C) {
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
	c.Assert(err, gc.ErrorMatches, "passwords do not match")
}

func (s *RegisterSuite) TestAPIOpenError(c *gc.C) {
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
	c.Assert(c.GetTestLog(), gc.Matches, "(.|\n)*open failed(.|\n)*")
	c.Assert(err, gc.ErrorMatches, `
Provided registration token may have been expired.
A controller administrator must reset your user to issue a new token.
See "juju help change-user-password" for more information.`[1:])
}

func (s *RegisterSuite) TestRegisterServerError(c *gc.C) {
	response, err := json.Marshal(params.ErrorResult{
		Error: &params.Error{Message: "xyz", Code: "123"},
	})

	s.httpHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, err = w.Write(response)
		c.Check(err, jc.ErrorIsNil)
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
	c.Assert(c.GetTestLog(), gc.Matches, "(.|\n)* xyz(.|\n)*")
	c.Assert(err, gc.ErrorMatches, `
Provided registration token may have been expired.
A controller administrator must reset your user to issue a new token.
See "juju help change-user-password" for more information.`[1:])

	// Check that the controller hasn't been added.
	_, err = s.store.ControllerByName("controller-name")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *RegisterSuite) TestRegisterPublic(c *gc.C) {
	s.apiConnection.authTag = names.NewUserTag("bob@external")
	s.apiConnection.controllerAccess = "login"
	prompter := cmdtesting.NewSeqPrompter(c, "»", `
Enter a name for this controller: »public-controller-name

Welcome, bob@external. You are now logged into "public-controller-name".
`[1:]+noModelsText)
	defer prompter.CheckDone()
	err := s.run(c, prompter, "0.1.2.3")
	c.Assert(err, jc.ErrorIsNil)

	// The controller and account details should be recorded with
	// the specified controller name and user
	// name from the auth tag.

	controller, err := s.store.ControllerByName("public-controller-name")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(controller, jc.DeepEquals, &jujuclient.ControllerDetails{
		ControllerUUID: mockControllerUUID,
		APIEndpoints:   []string{"0.1.2.3:443"},
	})
	account, err := s.store.AccountDetails("public-controller-name")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(account, jc.DeepEquals, &jujuclient.AccountDetails{
		User:            "bob@external",
		LastKnownAccess: "login",
	})
}

func (s *RegisterSuite) TestRegisterAlreadyKnownControllerEndpoint(c *gc.C) {
	prompter := cmdtesting.NewSeqPrompter(c, "»", "")
	defer prompter.CheckDone()

	err := s.store.AddController("foo", jujuclient.ControllerDetails{
		APIEndpoints:   []string{"42.42.42.42:17070"},
		ControllerUUID: "0d75314a-5266-4f4f-8523-415be76f92dc",
		CACert:         testing.CACert,
	})
	c.Assert(err, jc.ErrorIsNil)

	err = s.run(c, prompter, "42.42.42.42:17070")
	c.Assert(err, gc.Not(gc.IsNil))
	c.Assert(err.Error(), gc.Equals, `This controller has already been registered on this client as "foo".
To login run 'juju login -c foo'.`)
}

func (s *RegisterSuite) TestRegisterAlreadyKnownControllerEndpointAndUser(c *gc.C) {
	prompter := cmdtesting.NewSeqPrompter(c, "»", "")
	defer prompter.CheckDone()

	err := s.store.AddController("foo", jujuclient.ControllerDetails{
		APIEndpoints:   []string{"42.42.42.42:17070"},
		ControllerUUID: "0d75314a-5266-4f4f-8523-415be76f92dc",
		CACert:         testing.CACert,
	})
	c.Assert(err, jc.ErrorIsNil)

	s.store.Accounts["foo"] = jujuclient.AccountDetails{
		User: "bob",
	}

	err = s.run(c, prompter, "42.42.42.42:17070")
	c.Assert(err, gc.Not(gc.IsNil))
	c.Assert(err.Error(), gc.Equals, `This controller has already been registered on this client as "foo".
To login user "bob" run 'juju login -u bob -c foo'.
To update controller details and login as user "bob":
    1. run 'juju unregister foo'
    2. request from your controller admin another registration string, i.e
       output from 'juju change-user-password bob --reset'
    3. re-run 'juju register' with the registration string from (2) above.
`)
}

func (s *RegisterSuite) TestRegisterPublicAPIOpenError(c *gc.C) {
	s.apiOpenError = errors.New("open failed")
	prompter := cmdtesting.NewSeqPrompter(c, "»", `
Enter a name for this controller: »public-controller-name
`[1:])
	defer prompter.CheckDone()
	err := s.run(c, prompter, "0.1.2.3")
	c.Assert(err, gc.ErrorMatches, `open failed`)
}

func (s *RegisterSuite) TestRegisterPublicWithPort(c *gc.C) {
	s.apiConnection.authTag = names.NewUserTag("bob@external")
	s.apiConnection.controllerAccess = "login"
	prompter := cmdtesting.NewSeqPrompter(c, "»", `
Enter a name for this controller: »public-controller-name

Welcome, bob@external. You are now logged into "public-controller-name".
`[1:]+noModelsText)
	defer prompter.CheckDone()
	err := s.run(c, prompter, "0.1.2.3:5678")
	c.Assert(err, jc.ErrorIsNil)

	// The controller and account details should be recorded with
	// the specified controller name and user
	// name from the auth tag.

	controller, err := s.store.ControllerByName("public-controller-name")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(controller, jc.DeepEquals, &jujuclient.ControllerDetails{
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
func (s *RegisterSuite) mockServer(c *gc.C) *mockServer {
	respNonce := []byte(strings.Repeat("X", 24))

	responsePayloadPlaintext, err := json.Marshal(params.SecretKeyLoginResponsePayload{
		CACert:         testing.CACert,
		ControllerUUID: mockControllerUUID,
	})
	c.Assert(err, jc.ErrorIsNil)
	response, err := json.Marshal(params.SecretKeyLoginResponse{
		Nonce:             respNonce,
		PayloadCiphertext: s.seal(c, responsePayloadPlaintext, mockSecretKey, respNonce),
	})
	c.Assert(err, jc.ErrorIsNil)
	return &mockServer{
		response: response,
	}
}

func (s *RegisterSuite) encodeRegistrationData(c *gc.C, info jujuclient.RegistrationInfo) string {
	info.Addrs = []string{s.apiConnection.addr}
	data, err := asn1.Marshal(info)
	c.Assert(err, jc.ErrorIsNil)
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
	requestBody, err := ioutil.ReadAll(r.Body)
	if err != nil {
		panic(err)
	}
	srv.requestBodies = append(srv.requestBodies, requestBody)
	_, err = w.Write(srv.response)
	if err != nil {
		panic(err)
	}
}

func (s *RegisterSuite) apiOpen(info *api.Info, opts api.DialOpts) (api.Connection, error) {
	if s.apiOpenError != nil {
		return nil, s.apiOpenError
	}
	return s.apiConnection, nil
}

func (s *RegisterSuite) run(c *gc.C, stdio io.ReadWriter, args ...string) error {
	if stdio == nil {
		p := noPrompts(c)
		stdio = p
		defer p.CheckDone()
	}

	command := controller.NewRegisterCommandForTest(s.apiOpen, s.listModels, s.store)
	err := cmdtesting.InitCommand(command, args)
	c.Assert(err, jc.ErrorIsNil)
	return command.Run(&cmd.Context{
		Dir:    c.MkDir(),
		Stdin:  stdio,
		Stdout: stdio,
		Stderr: stdio,
	})
}

func noPrompts(c *gc.C) *cmdtesting.SeqPrompter {
	return cmdtesting.NewSeqPrompter(c, "»", "")
}

func (s *RegisterSuite) seal(c *gc.C, message, key, nonce []byte) []byte {
	var keyArray [32]byte
	var nonceArray [24]byte
	c.Assert(copy(keyArray[:], key), gc.Equals, len(keyArray))
	c.Assert(copy(nonceArray[:], nonce), gc.Equals, len(nonceArray))
	return secretbox.Seal(nil, message, &nonceArray, &keyArray)
}
