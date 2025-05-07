// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"
	"golang.org/x/crypto/nacl/secretbox"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/permission"
	providertracker "github.com/juju/juju/core/providertracker"
	"github.com/juju/juju/core/user"
	usertesting "github.com/juju/juju/core/user/testing"
	usererrors "github.com/juju/juju/domain/access/errors"
	accessservice "github.com/juju/juju/domain/access/service"
	environs "github.com/juju/juju/environs"
	"github.com/juju/juju/internal/auth"
	jujuhttp "github.com/juju/juju/internal/http"
	"github.com/juju/juju/internal/testhelpers/httptesting"
	coretesting "github.com/juju/juju/internal/testing"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
)

type registrationSuite struct {
	jujutesting.ApiServerSuite
	accessService   *accessservice.Service
	userUUID        user.UUID
	activationKey   []byte
	registrationURL string
}

var _ = tc.Suite(&registrationSuite{})

func (s *registrationSuite) SetUpTest(c *tc.C) {
	s.ApiServerSuite.SetUpTest(c)

	s.accessService = s.ControllerDomainServices(c).Access()
	var err error
	s.userUUID, _, err = s.accessService.AddUser(context.Background(), accessservice.AddUserArg{
		Name:        usertesting.GenNewName(c, "bob"),
		CreatorUUID: s.AdminUserUUID,
		Permission: permission.AccessSpec{
			Access: permission.LoginAccess,
			Target: permission.ID{
				ObjectType: permission.Controller,
				Key:        s.ControllerUUID,
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	s.activationKey, err = s.accessService.ResetPassword(context.Background(), usertesting.GenNewName(c, "bob"))
	c.Assert(err, tc.ErrorIsNil)

	s.registrationURL = s.URL("/register", url.Values{}).String()
}

func (s *registrationSuite) assertRegisterNoProxy(c *tc.C, hasProxy bool) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	rawConfig := map[string]interface{}{
		"api-host":              "https://127.0.0.1:16443",
		"ca-cert":               "cert====",
		"namespace":             "controller-k1",
		"remote-port":           "17070",
		"service":               "controller-service",
		"service-account-token": "token====",
	}
	environ := NewMockConnectorInfo(ctrl)
	proxier := NewMockProxier(ctrl)

	providerFactory := NewMockProviderFactory(ctrl)

	// Setting this like this is less than ideal, as it should be done much
	// earlier in the test setup, but it's the only way to get the provider
	// factory to return a provider that implements the providertracker.Provider.
	s.ProviderFactory = providerFactory

	if hasProxy {
		// This is a bit of a hack. We can't hack out the domain services,
		// but we can hack out the provider factory, which is the only thing
		// that the domain services uses to get the provider.
		// By brute force, we can make the provider factory return a provider
		// that implements the providertracker.Provider interface.
		providerFactory.EXPECT().ProviderForModel(gomock.Any(), gomock.Any()).Return(struct {
			// This has to match the proxy service Provider interface, with
			// the addition of the providertracker.Provider interface.
			providertracker.Provider
			environs.ConnectorInfo
			caas.ProxyManager
		}{ConnectorInfo: environ}, nil)

		environ.EXPECT().ConnectionProxyInfo(gomock.Any()).Return(proxier, nil)
		proxier.EXPECT().RawConfig().Return(rawConfig, nil)
		proxier.EXPECT().Type().Return("kubernetes-port-forward")
	} else {
		// If there is no provider for the model and no error, then it won't
		// match the right provider type, so will return a not supported error.
		providerFactory.EXPECT().ProviderForModel(gomock.Any(), gomock.Any()).Return(nil, nil)
	}

	password := "hunter2"
	// It should be not possible to log in as bob with the password "hunter2"
	// now.
	_, err := s.accessService.GetUserByAuth(context.Background(), usertesting.GenNewName(c, "bob"), auth.NewPassword(password))
	c.Assert(err, tc.ErrorIs, usererrors.UserUnauthorized)

	validNonce := []byte(strings.Repeat("X", 24))
	ciphertext := s.sealBox(
		c, validNonce, s.activationKey, fmt.Sprintf(`{"password": "%s"}`, password),
	)
	client := jujuhttp.NewClient(jujuhttp.WithSkipHostnameVerification(true))
	resp := httptesting.Do(c, httptesting.DoRequestParams{
		Do:     client.Do,
		URL:    s.registrationURL,
		Method: "POST",
		JSONBody: &params.SecretKeyLoginRequest{
			User:              "user-bob",
			Nonce:             validNonce,
			PayloadCiphertext: ciphertext,
		},
	})
	c.Assert(resp.StatusCode, tc.Equals, http.StatusOK)
	defer resp.Body.Close()

	// It should be possible to log in as bob with the
	// password "hunter2" now, and there should be no
	// secret key any longer.
	user, err := s.accessService.GetUserByAuth(context.Background(), usertesting.GenNewName(c, "bob"), auth.NewPassword(password))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(user.UUID, tc.Equals, s.userUUID)

	var response params.SecretKeyLoginResponse
	bodyData, err := io.ReadAll(resp.Body)
	c.Assert(err, tc.ErrorIsNil)
	err = json.Unmarshal(bodyData, &response)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(response.Nonce, tc.HasLen, len(validNonce))

	// Open the box to ensure that the response is as expected.
	plaintext := s.openBox(c, response.PayloadCiphertext, response.Nonce, s.activationKey)

	var responsePayload params.SecretKeyLoginResponsePayload
	err = json.Unmarshal(plaintext, &responsePayload)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(responsePayload.CACert, tc.Equals, coretesting.CACert)
	c.Assert(responsePayload.ControllerUUID, tc.Equals, s.ControllerUUID)
	if hasProxy {
		c.Assert(responsePayload.ProxyConfig, tc.DeepEquals, &params.Proxy{
			Type: "kubernetes-port-forward", Config: rawConfig,
		})
	} else {
		c.Assert(responsePayload.ProxyConfig, tc.IsNil)
	}
}

func (s *registrationSuite) TestRegisterNoProxy(c *tc.C) {
	s.assertRegisterNoProxy(c, false)
}

func (s *registrationSuite) TestRegisterWithProxy(c *tc.C) {
	s.assertRegisterNoProxy(c, true)
}

func (s *registrationSuite) TestRegisterInvalidMethod(c *tc.C) {
	client := jujuhttp.NewClient(jujuhttp.WithSkipHostnameVerification(true))
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Do:           client.Do,
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

func (s *registrationSuite) TestRegisterInvalidFormat(c *tc.C) {
	s.testInvalidRequest(
		c, "[]", "json: cannot unmarshal array into Go value of type params.SecretKeyLoginRequest", "",
		http.StatusInternalServerError,
	)
}

func (s *registrationSuite) TestRegisterInvalidUserTag(c *tc.C) {
	s.testInvalidRequest(
		c, `{"user": "application-bob"}`, `"application-bob" is not a valid user tag`, "",
		http.StatusInternalServerError,
	)
}

func (s *registrationSuite) TestRegisterInvalidNonce(c *tc.C) {
	s.testInvalidRequest(
		c, `{"user": "user-bob", "nonce": ""}`, `nonce not valid`, params.CodeNotValid,
		http.StatusInternalServerError,
	)
}

func (s *registrationSuite) TestRegisterInvalidCiphertext(c *tc.C) {
	validNonce := []byte(strings.Repeat("X", 24))
	s.testInvalidRequest(c,
		fmt.Sprintf(
			`{"user": "user-bob", "nonce": "%s"}`,
			base64.StdEncoding.EncodeToString(validNonce),
		), `activation key not valid`, params.CodeNotValid,
		http.StatusInternalServerError,
	)
}

func (s *registrationSuite) TestRegisterNoSecretKey(c *tc.C) {
	err := s.accessService.SetPassword(context.Background(), usertesting.GenNewName(c, "bob"), auth.NewPassword("anything"))
	c.Assert(err, tc.ErrorIsNil)

	validNonce := []byte(strings.Repeat("X", 24))
	s.testInvalidRequest(c,
		fmt.Sprintf(
			`{"user": "user-bob", "nonce": "%s"}`,
			base64.StdEncoding.EncodeToString(validNonce),
		), `activation key not found`, params.CodeNotFound,
		http.StatusNotFound,
	)
}

func (s *registrationSuite) testInvalidRequest(c *tc.C, requestBody, errorMessage, errorCode string, statusCode int) {
	client := jujuhttp.NewClient(jujuhttp.WithSkipHostnameVerification(true))
	httptesting.AssertJSONCall(c, httptesting.JSONCallParams{
		Do:           client.Do,
		URL:          s.registrationURL,
		Method:       "POST",
		Body:         strings.NewReader(requestBody),
		ExpectStatus: statusCode,
		ExpectBody: &params.ErrorResult{
			Error: &params.Error{Message: errorMessage, Code: errorCode},
		},
	})
}

func (s *registrationSuite) sealBox(c *tc.C, nonce, key []byte, message string) []byte {
	var nonceArray [24]byte
	var keyArray [32]byte
	c.Assert(copy(nonceArray[:], nonce), tc.Equals, len(nonceArray))
	c.Assert(copy(keyArray[:], key), tc.Equals, len(keyArray))
	return secretbox.Seal(nil, []byte(message), &nonceArray, &keyArray)
}

func (s *registrationSuite) openBox(c *tc.C, ciphertext, nonce, key []byte) []byte {
	var nonceArray [24]byte
	var keyArray [32]byte
	c.Assert(copy(nonceArray[:], nonce), tc.Equals, len(nonceArray), tc.Commentf("nonce: %v", nonce))
	c.Assert(copy(keyArray[:], key), tc.Equals, len(keyArray), tc.Commentf("key: %v", key))
	message, ok := secretbox.Open(nil, ciphertext, &nonceArray, &keyArray)
	c.Assert(ok, tc.IsTrue)
	return message
}
