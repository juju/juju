// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/testing/httptesting"
	"github.com/juju/utils"
	"golang.org/x/crypto/nacl/secretbox"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
)

type registrationSuite struct {
	apiserverBaseSuite
	bob             *state.User
	registrationURL string
}

var _ = gc.Suite(&registrationSuite{})

func (s *registrationSuite) SetUpTest(c *gc.C) {
	s.apiserverBaseSuite.SetUpTest(c)
	bob, err := s.State.AddUserWithSecretKey("bob", "", "admin")
	c.Assert(err, jc.ErrorIsNil)
	s.bob = bob
	s.registrationURL = s.server.URL + "/register"
}

func (s *registrationSuite) TestRegister(c *gc.C) {
	// Ensure we cannot log in with the password yet.
	const password = "hunter2"
	c.Assert(s.bob.PasswordValid(password), jc.IsFalse)

	validNonce := []byte(strings.Repeat("X", 24))
	secretKey := s.bob.SecretKey()
	ciphertext := s.sealBox(
		c, validNonce, secretKey, fmt.Sprintf(`{"password": "%s"}`, password),
	)
	resp := httptesting.Do(c, httptesting.DoRequestParams{
		Do:     utils.GetNonValidatingHTTPClient().Do,
		URL:    s.registrationURL,
		Method: "POST",
		JSONBody: &params.SecretKeyLoginRequest{
			User:              "user-bob",
			Nonce:             validNonce,
			PayloadCiphertext: ciphertext,
		},
	})
	c.Assert(resp.StatusCode, gc.Equals, http.StatusOK)
	defer resp.Body.Close()

	// It should be possible to log in as bob with the
	// password "hunter2" now, and there should be no
	// secret key any longer.
	err := s.bob.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.bob.PasswordValid(password), jc.IsTrue)
	c.Assert(s.bob.SecretKey(), gc.IsNil)

	var response params.SecretKeyLoginResponse
	bodyData, err := ioutil.ReadAll(resp.Body)
	c.Assert(err, jc.ErrorIsNil)
	err = json.Unmarshal(bodyData, &response)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(response.Nonce, gc.HasLen, len(validNonce))
	plaintext := s.openBox(c, response.PayloadCiphertext, response.Nonce, secretKey)

	var responsePayload params.SecretKeyLoginResponsePayload
	err = json.Unmarshal(plaintext, &responsePayload)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(responsePayload.CACert, gc.Equals, coretesting.CACert)
	model, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(responsePayload.ControllerUUID, gc.Equals, model.ControllerUUID())
}

func (s *registrationSuite) TestRegisterInvalidMethod(c *gc.C) {
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Do:           utils.GetNonValidatingHTTPClient().Do,
		URL:          s.registrationURL,
		Method:       "GET",
		ExpectStatus: http.StatusMethodNotAllowed,
		ExpectBody: &params.ErrorResult{
			Error: &params.Error{
				Message: `unsupported method: "GET"`,
				Code:    params.CodeMethodNotAllowed,
			},
		},
	})
}

func (s *registrationSuite) TestRegisterInvalidFormat(c *gc.C) {
	s.testInvalidRequest(
		c, "[]", "json: cannot unmarshal array into Go value of type params.SecretKeyLoginRequest", "",
		http.StatusInternalServerError,
	)
}

func (s *registrationSuite) TestRegisterInvalidUserTag(c *gc.C) {
	s.testInvalidRequest(
		c, `{"user": "application-bob"}`, `"application-bob" is not a valid user tag`, "",
		http.StatusInternalServerError,
	)
}

func (s *registrationSuite) TestRegisterInvalidNonce(c *gc.C) {
	s.testInvalidRequest(
		c, `{"user": "user-bob", "nonce": ""}`, `nonce not valid`, "",
		http.StatusInternalServerError,
	)
}

func (s *registrationSuite) TestRegisterInvalidCiphertext(c *gc.C) {
	validNonce := []byte(strings.Repeat("X", 24))
	s.testInvalidRequest(c,
		fmt.Sprintf(
			`{"user": "user-bob", "nonce": "%s"}`,
			base64.StdEncoding.EncodeToString(validNonce),
		), `secret key not valid`, "",
		http.StatusInternalServerError,
	)
}

func (s *registrationSuite) TestRegisterNoSecretKey(c *gc.C) {
	err := s.bob.SetPassword("anything")
	c.Assert(err, jc.ErrorIsNil)
	validNonce := []byte(strings.Repeat("X", 24))
	s.testInvalidRequest(c,
		fmt.Sprintf(
			`{"user": "user-bob", "nonce": "%s"}`,
			base64.StdEncoding.EncodeToString(validNonce),
		), `secret key for user "bob" not found`, params.CodeNotFound,
		http.StatusNotFound,
	)
}

func (s *registrationSuite) TestRegisterInvalidRequestPayload(c *gc.C) {
	validNonce := []byte(strings.Repeat("X", 24))
	ciphertext := s.sealBox(c, validNonce, s.bob.SecretKey(), "[]")
	s.testInvalidRequest(c,
		fmt.Sprintf(
			`{"user": "user-bob", "nonce": "%s", "cipher-text": "%s"}`,
			base64.StdEncoding.EncodeToString(validNonce),
			base64.StdEncoding.EncodeToString(ciphertext),
		),
		`cannot unmarshal payload: json: cannot unmarshal array into Go value of type params.SecretKeyLoginRequestPayload`, "",
		http.StatusInternalServerError,
	)
}

func (s *registrationSuite) testInvalidRequest(c *gc.C, requestBody, errorMessage, errorCode string, statusCode int) {
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Do:           utils.GetNonValidatingHTTPClient().Do,
		URL:          s.registrationURL,
		Method:       "POST",
		Body:         strings.NewReader(requestBody),
		ExpectStatus: statusCode,
		ExpectBody: &params.ErrorResult{
			Error: &params.Error{Message: errorMessage, Code: errorCode},
		},
	})
}

func (s *registrationSuite) sealBox(c *gc.C, nonce, key []byte, message string) []byte {
	var nonceArray [24]byte
	var keyArray [32]byte
	c.Assert(copy(nonceArray[:], nonce), gc.Equals, len(nonceArray))
	c.Assert(copy(keyArray[:], key), gc.Equals, len(keyArray))
	return secretbox.Seal(nil, []byte(message), &nonceArray, &keyArray)
}

func (s *registrationSuite) openBox(c *gc.C, ciphertext, nonce, key []byte) []byte {
	var nonceArray [24]byte
	var keyArray [32]byte
	c.Assert(copy(nonceArray[:], nonce), gc.Equals, len(nonceArray))
	c.Assert(copy(keyArray[:], key), gc.Equals, len(keyArray))
	message, ok := secretbox.Open(nil, ciphertext, &nonceArray, &keyArray)
	c.Assert(ok, jc.IsTrue)
	return message
}
