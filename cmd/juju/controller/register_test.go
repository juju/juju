// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller_test

import (
	"bytes"
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
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"golang.org/x/crypto/nacl/secretbox"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/controller"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	"github.com/juju/juju/testing"
)

type RegisterSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	apiConnection            *mockAPIConnection
	store                    *jujuclienttesting.MemStore
	apiOpenError             error
	listModels               func(jujuclient.ClientStore, string, string) ([]base.UserModel, error)
	listModelsControllerName string
	listModelsUserName       string
	server                   *httptest.Server
	httpHandler              http.Handler
}

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
		controllerTag: testing.ControllerTag,
		addr:          serverURL.Host,
	}
	s.listModelsControllerName = ""
	s.listModelsUserName = ""
	s.listModels = func(_ jujuclient.ClientStore, controllerName, userName string) ([]base.UserModel, error) {
		s.listModelsControllerName = controllerName
		s.listModelsUserName = userName
		return nil, nil
	}

	s.store = jujuclienttesting.NewMemStore()
}

func (s *RegisterSuite) TearDownTest(c *gc.C) {
	s.server.Close()
	s.FakeJujuXDGDataHomeSuite.TearDownTest(c)
}

func (s *RegisterSuite) apiOpen(info *api.Info, opts api.DialOpts) (api.Connection, error) {
	if s.apiOpenError != nil {
		return nil, s.apiOpenError
	}
	s.apiConnection.info = info
	s.apiConnection.opts = opts
	return s.apiConnection, nil
}

func (s *RegisterSuite) run(c *gc.C, stdin io.Reader, args ...string) (*cmd.Context, error) {
	command := controller.NewRegisterCommandForTest(s.apiOpen, s.listModels, s.store)
	err := testing.InitCommand(command, args)
	c.Assert(err, jc.ErrorIsNil)
	ctx := testing.Context(c)
	ctx.Stdin = stdin
	return ctx, command.Run(ctx)
}

func (s *RegisterSuite) encodeRegistrationData(c *gc.C, user string, secretKey []byte) string {
	data, err := asn1.Marshal(jujuclient.RegistrationInfo{
		User:      user,
		Addrs:     []string{s.apiConnection.addr},
		SecretKey: secretKey,
	})
	c.Assert(err, jc.ErrorIsNil)
	// Append some junk to the end of the encoded data to
	// ensure that, if we have to pad the data in add-user,
	// register can still decode it.
	data = append(data, 0, 0, 0)
	return base64.URLEncoding.EncodeToString(data)
}

func (s *RegisterSuite) encodeRegistrationDataWithControllerName(c *gc.C, user string, secretKey []byte, controller string) string {
	data, err := asn1.Marshal(jujuclient.RegistrationInfo{
		User:           user,
		Addrs:          []string{s.apiConnection.addr},
		SecretKey:      secretKey,
		ControllerName: controller,
	})
	c.Assert(err, jc.ErrorIsNil)
	// Append some junk to the end of the encoded data to
	// ensure that, if we have to pad the data in add-user,
	// register can still decode it.
	data = append(data, 0, 0, 0)
	return base64.URLEncoding.EncodeToString(data)
}

func (s *RegisterSuite) seal(c *gc.C, message, key, nonce []byte) []byte {
	var keyArray [32]byte
	var nonceArray [24]byte
	c.Assert(copy(keyArray[:], key), gc.Equals, len(keyArray))
	c.Assert(copy(nonceArray[:], nonce), gc.Equals, len(nonceArray))
	return secretbox.Seal(nil, message, &nonceArray, &keyArray)
}

func (s *RegisterSuite) TestInit(c *gc.C) {
	registerCommand := controller.NewRegisterCommandForTest(nil, nil, nil)

	err := testing.InitCommand(registerCommand, []string{})
	c.Assert(err, gc.ErrorMatches, "registration data missing")

	err = testing.InitCommand(registerCommand, []string{"foo"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(registerCommand.EncodedData, gc.Equals, "foo")

	err = testing.InitCommand(registerCommand, []string{"foo", "bar"})
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["bar"\]`)
}

func (s *RegisterSuite) TestRegister(c *gc.C) {
	ctx := s.testRegister(c, "")
	c.Assert(s.listModelsControllerName, gc.Equals, "controller-name")
	c.Assert(s.listModelsUserName, gc.Equals, "bob@local")
	stderr := testing.Stderr(ctx)
	c.Assert(stderr, gc.Equals, `
Enter a name for this controller [controller-name]: 
Enter a new password: 
Confirm password: 

Welcome, bob. You are now logged into "controller-name".

There are no models available. You can add models with
"juju add-model", or you can ask an administrator or owner
of a model to grant access to that model with "juju grant".

`[1:])
}

func (s *RegisterSuite) TestRegisterOneModel(c *gc.C) {
	s.listModels = func(_ jujuclient.ClientStore, controllerName, userName string) ([]base.UserModel, error) {
		return []base.UserModel{{
			Name:  "theoneandonly",
			Owner: "carol@local",
			UUID:  "df136476-12e9-11e4-8a70-b2227cce2b54",
		}}, nil
	}
	s.testRegister(c, "")
	c.Assert(
		s.store.Models["controller-name"].CurrentModel,
		gc.Equals, "carol@local/theoneandonly",
	)
}

func (s *RegisterSuite) TestRegisterMultipleModels(c *gc.C) {
	s.listModels = func(_ jujuclient.ClientStore, controllerName, userName string) ([]base.UserModel, error) {
		return []base.UserModel{{
			Name:  "model1",
			Owner: "bob@local",
			UUID:  "df136476-12e9-11e4-8a70-b2227cce2b54",
		}, {
			Name:  "model2",
			Owner: "bob@local",
			UUID:  "df136476-12e9-11e4-8a70-b2227cce2b55",
		}}, nil
	}
	ctx := s.testRegister(c, "")

	// When there are multiple models, no current model will be set.
	// Instead, the command will output the list of models and inform
	// the user how to set the current model.
	_, err := s.store.CurrentModel("controller-name")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	stderr := testing.Stderr(ctx)
	c.Assert(stderr, gc.Equals, `
Enter a name for this controller [controller-name]: 
Enter a new password: 
Confirm password: 

Welcome, bob. You are now logged into "controller-name".

There are 2 models available. Use "juju switch" to select
one of them:
  - juju switch model1
  - juju switch model2

`[1:])
}

func (s *RegisterSuite) testRegister(c *gc.C, expectedError string) *cmd.Context {
	secretKey := []byte(strings.Repeat("X", 32))
	respNonce := []byte(strings.Repeat("X", 24))

	var requests []*http.Request
	var requestBodies [][]byte
	const controllerUUID = "df136476-12e9-11e4-8a70-b2227cce2b54"
	responsePayloadPlaintext, err := json.Marshal(params.SecretKeyLoginResponsePayload{
		CACert:         testing.CACert,
		ControllerUUID: controllerUUID,
	})
	c.Assert(err, jc.ErrorIsNil)
	response, err := json.Marshal(params.SecretKeyLoginResponse{
		Nonce:             respNonce,
		PayloadCiphertext: s.seal(c, responsePayloadPlaintext, secretKey, respNonce),
	})
	c.Assert(err, jc.ErrorIsNil)
	s.httpHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		requestBody, err := ioutil.ReadAll(requests[0].Body)
		c.Check(err, jc.ErrorIsNil)
		requestBodies = append(requestBodies, requestBody)
		_, err = w.Write(response)
		c.Check(err, jc.ErrorIsNil)
	})

	registrationData := s.encodeRegistrationDataWithControllerName(c, "bob", secretKey, "controller-name")
	stdin := strings.NewReader("\nhunter2\nhunter2\n")
	ctx, err := s.run(c, stdin, registrationData)
	if expectedError != "" {
		c.Assert(err, gc.ErrorMatches, expectedError)
		return ctx
	}
	c.Assert(err, jc.ErrorIsNil)

	// There should have been one POST command to "/register".
	c.Assert(requests, gc.HasLen, 1)
	c.Assert(requests[0].Method, gc.Equals, "POST")
	c.Assert(requests[0].URL.Path, gc.Equals, "/register")
	var request params.SecretKeyLoginRequest
	err = json.Unmarshal(requestBodies[0], &request)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(request.User, jc.DeepEquals, "user-bob")
	c.Assert(request.Nonce, gc.HasLen, 24)
	requestPayloadPlaintext, err := json.Marshal(params.SecretKeyLoginRequestPayload{
		"hunter2",
	})
	c.Assert(err, jc.ErrorIsNil)
	expectedCiphertext := s.seal(c, requestPayloadPlaintext, secretKey, request.Nonce)
	c.Assert(request.PayloadCiphertext, jc.DeepEquals, expectedCiphertext)

	// The controller and account details should be recorded with
	// the specified controller name ("controller-name") and user
	// name from the registration string.

	controller, err := s.store.ControllerByName("controller-name")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(controller, jc.DeepEquals, &jujuclient.ControllerDetails{
		ControllerUUID: controllerUUID,
		APIEndpoints:   []string{s.apiConnection.addr},
		CACert:         testing.CACert,
	})
	account, err := s.store.AccountDetails("controller-name")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(account, jc.DeepEquals, &jujuclient.AccountDetails{
		User:            "bob@local",
		LastKnownAccess: "login",
	})
	return ctx
}

func (s *RegisterSuite) TestRegisterInvalidRegistrationData(c *gc.C) {
	_, err := s.run(c, bytes.NewReader(nil), "not base64")
	c.Assert(err, gc.ErrorMatches, "illegal base64 data at input byte 3")

	_, err = s.run(c, bytes.NewReader(nil), "YXNuLjEK")
	c.Assert(err, gc.ErrorMatches, "asn1: structure error: .*")
}

func (s *RegisterSuite) TestRegisterEmptyControllerName(c *gc.C) {
	secretKey := []byte(strings.Repeat("X", 32))
	registrationData := s.encodeRegistrationData(c, "bob", secretKey)
	stdin := strings.NewReader("\n")
	_, err := s.run(c, stdin, registrationData)
	c.Assert(err, gc.ErrorMatches, "you must specify a non-empty controller name")
}

func (s *RegisterSuite) TestRegisterControllerNameExists(c *gc.C) {
	err := s.store.AddController("controller-name", jujuclient.ControllerDetails{
		ControllerUUID: "df136476-12e9-11e4-8a70-b2227cce2b54",
		CACert:         testing.CACert,
	})

	secretKey := []byte(strings.Repeat("X", 32))
	registrationData := s.encodeRegistrationData(c, "bob", secretKey)
	stdin := strings.NewReader("controller-name\nhunter2\nhunter2\n")
	_, err = s.run(c, stdin, registrationData)
	c.Assert(err, gc.ErrorMatches, `controller "controller-name" already exists`)
}

func (s *RegisterSuite) TestControllerUUIDExists(c *gc.C) {
	// Controller has the UUID from s.testRegister to mimic a user with
	// this controller already registered (regardless of its name).
	err := s.store.AddController("controller-name", jujuclient.ControllerDetails{
		ControllerUUID: "df136476-12e9-11e4-8a70-b2227cce2b54",
		CACert:         testing.CACert,
	})

	s.listModels = func(_ jujuclient.ClientStore, controllerName, userName string) ([]base.UserModel, error) {
		return []base.UserModel{{
			Name:  "model-name",
			Owner: "bob@local",
			UUID:  "df136476-12e9-11e4-8a70-b2227cce2b54",
		}}, nil
	}

	s.testRegister(c, "you must specify a non-empty controller name")

	secretKey := []byte(strings.Repeat("X", 32))
	registrationData := s.encodeRegistrationDataWithControllerName(c, "bob", secretKey, "controller-name")

	stdin := strings.NewReader("another-controller-name\nhunter2\nhunter2\n")
	_, err = s.run(c, stdin, registrationData)
	c.Assert(err, gc.ErrorMatches, "controller with UUID.*already exists")
}

func (s *RegisterSuite) TestProposedControllerNameExists(c *gc.C) {
	// Controller does not have the UUID from s.testRegister, thereby
	// mimicing a user with an already registered 'foreign' controller.
	err := s.store.AddController("controller-name", jujuclient.ControllerDetails{
		ControllerUUID: "0d75314a-5266-4f4f-8523-415be76f92dc",
		CACert:         testing.CACert,
	})

	s.listModels = func(_ jujuclient.ClientStore, controllerName, userName string) ([]base.UserModel, error) {
		return []base.UserModel{{
			Name:  "model-name",
			Owner: "bob@local",
			UUID:  "df136476-12e9-11e4-8a70-b2227cce2b54",
		}}, nil
	}

	ctx := s.testRegister(c, "you must specify a non-empty controller name")

	secretKey := []byte(strings.Repeat("X", 32))
	registrationData := s.encodeRegistrationDataWithControllerName(c, "bob", secretKey, "controller-name")

	stdin := strings.NewReader("another-controller-name\nhunter2\nhunter2\n")
	_, err = s.run(c, stdin, registrationData)
	c.Assert(err, jc.ErrorIsNil)
	stderr := testing.Stderr(ctx)
	c.Assert(stderr, gc.Equals, `
WARNING: You already have a controller registered with the name "controller-name". Please choose a different name for the new controller.

Enter a name for this controller: 
`[1:])

}

func (s *RegisterSuite) TestRegisterEmptyPassword(c *gc.C) {
	secretKey := []byte(strings.Repeat("X", 32))
	registrationData := s.encodeRegistrationData(c, "bob", secretKey)
	stdin := strings.NewReader("controller-name\n\n")
	_, err := s.run(c, stdin, registrationData)
	c.Assert(err, gc.ErrorMatches, "you must specify a non-empty password")
}

func (s *RegisterSuite) TestRegisterPasswordMismatch(c *gc.C) {
	secretKey := []byte(strings.Repeat("X", 32))
	registrationData := s.encodeRegistrationData(c, "bob", secretKey)
	stdin := strings.NewReader("controller-name\nhunter2\nhunter3\n")
	_, err := s.run(c, stdin, registrationData)
	c.Assert(err, gc.ErrorMatches, "passwords do not match")
}

func (s *RegisterSuite) TestAPIOpenError(c *gc.C) {
	secretKey := []byte(strings.Repeat("X", 32))
	registrationData := s.encodeRegistrationData(c, "bob", secretKey)
	stdin := strings.NewReader("controller-name\nhunter2\nhunter2\n")
	s.apiOpenError = errors.New("open failed")
	_, err := s.run(c, stdin, registrationData)
	c.Assert(err, gc.ErrorMatches, `open failed`)
}

func (s *RegisterSuite) TestRegisterServerError(c *gc.C) {
	secretKey := []byte(strings.Repeat("X", 32))
	response, err := json.Marshal(params.ErrorResult{
		Error: &params.Error{Message: "xyz", Code: "123"},
	})

	s.httpHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, err = w.Write(response)
		c.Check(err, jc.ErrorIsNil)
	})

	registrationData := s.encodeRegistrationData(c, "bob", secretKey)
	stdin := strings.NewReader("controller-name\nhunter2\nhunter2\n")
	_, err = s.run(c, stdin, registrationData)
	c.Assert(err, gc.ErrorMatches, "xyz")

	_, err = s.store.ControllerByName("controller-name")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}
