// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	jujuhttp "github.com/juju/juju/internal/http"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type sessionTokenLoginProviderProviderSuite struct {
	testing.BaseSuite
}

func TestSessionTokenLoginProviderProviderSuite(t *stdtesting.T) {
	tc.Run(t, &sessionTokenLoginProviderProviderSuite{})
}
func (s *sessionTokenLoginProviderProviderSuite) APIInfo() *api.Info {
	srv := apiservertesting.NewAPIServer(func(modelUUID string) (interface{}, error) {
		var err error
		if modelUUID != "" && modelUUID != testing.ModelTag.Id() {
			err = fmt.Errorf("%w: %q", apiservererrors.UnknownModelError, modelUUID)
		}
		return &testRootAPI{}, err
	})
	s.AddCleanup(func(_ *tc.C) { srv.Close() })
	info := &api.Info{
		Addrs:          srv.Addrs,
		CACert:         testing.CACert,
		ControllerUUID: testing.ControllerTag.Id(),
		ModelTag:       testing.ModelTag,
	}
	return info
}

func (s *sessionTokenLoginProviderProviderSuite) TestSessionTokenLogin(c *tc.C) {
	info := s.APIInfo()

	sessionToken := "test-session-token"
	userCode := "1234567"
	verificationURI := "http://localhost:8080/test-verification"

	var obtainedSessionToken string

	s.PatchValue(api.LoginDeviceAPICall, func(ctx context.Context, _ base.APICaller, request interface{}, response interface{}) error {
		lr := struct {
			UserCode        string `json:"user-code"`
			VerificationURI string `json:"verification-uri"`
		}{
			UserCode:        userCode,
			VerificationURI: verificationURI,
		}

		data, err := json.Marshal(lr)
		if err != nil {
			return errors.Trace(err)
		}

		return json.Unmarshal(data, response)
	})

	s.PatchValue(api.GetDeviceSessionTokenAPICall, func(ctx context.Context, _ base.APICaller, request interface{}, response interface{}) error {
		lr := struct {
			SessionToken string `json:"session-token"`
		}{
			SessionToken: sessionToken,
		}

		data, err := json.Marshal(lr)
		if err != nil {
			return errors.Trace(err)
		}

		return json.Unmarshal(data, response)
	})

	s.PatchValue(api.LoginWithSessionTokenAPICall, func(ctx context.Context, _ base.APICaller, request interface{}, response interface{}) error {
		data, err := json.Marshal(request)
		if err != nil {
			return errors.Trace(err)
		}

		var lr struct {
			SessionToken string `json:"session-token"`
		}

		err = json.Unmarshal(data, &lr)
		if err != nil {
			return errors.Trace(err)
		}

		if lr.SessionToken != sessionToken {
			return &params.Error{
				Message: "invalid token",
				Code:    params.CodeSessionTokenInvalid,
			}
		}

		loginResult, ok := response.(*params.LoginResult)
		if !ok {
			return errors.Errorf("expected %T, received %T for response type", loginResult, response)
		}
		loginResult.ControllerTag = names.NewControllerTag(info.ControllerUUID).String()
		loginResult.ServerVersion = "3.4.0"
		loginResult.UserInfo = &params.AuthUserInfo{
			DisplayName:      "alice@external",
			Identity:         names.NewUserTag("alice@external").String(),
			ControllerAccess: "superuser",
		}
		return nil
	})

	var output bytes.Buffer
	lp := api.NewSessionTokenLoginProvider(
		"expired-token",
		&output,
		func(sessionToken string) {
			obtainedSessionToken = sessionToken
		})
	apiState, err := api.Open(c.Context(), &api.Info{
		Addrs:          info.Addrs,
		ControllerUUID: info.ControllerUUID,
		CACert:         info.CACert,
	}, api.DialOpts{
		LoginProvider: lp,
	})
	c.Assert(err, tc.ErrorIsNil)
	defer func() { _ = apiState.Close() }()

	c.Check(output.String(), tc.Equals, "Please visit http://localhost:8080/test-verification and enter code 1234567 to log in.\n")
	c.Check(obtainedSessionToken, tc.Equals, sessionToken)
	c.Check(err, tc.ErrorIsNil)
}

func (s *sessionTokenLoginProviderProviderSuite) TestInvalidSessionTokenLogin(c *tc.C) {
	info := s.APIInfo()

	expectedErr := &params.Error{
		Message: "unauthorized",
		Code:    params.CodeUnauthorized,
	}
	s.PatchValue(api.LoginWithSessionTokenAPICall, func(_ context.Context, _ base.APICaller, request interface{}, response interface{}) error {
		return expectedErr
	})

	var output bytes.Buffer
	_, err := api.Open(c.Context(), &api.Info{
		Addrs:          info.Addrs,
		ControllerUUID: info.ControllerUUID,
		CACert:         info.CACert,
	}, api.DialOpts{
		LoginProvider: api.NewSessionTokenLoginProvider(
			"random-token",
			&output,
			func(sessionToken string) {},
		),
	})
	c.Assert(err, tc.ErrorIs, expectedErr)
}

// A separate suite for tests that don't need to communicate with a controller.
type sessionTokenLoginProviderBasicSuite struct {
	testing.BaseSuite
}

func TestSessionTokenLoginProviderBasicSuite(t *stdtesting.T) {
	tc.Run(t, &sessionTokenLoginProviderBasicSuite{})
}
func (s *sessionTokenLoginProviderBasicSuite) TestSessionTokenAuthHeader(c *tc.C) {
	var output bytes.Buffer
	testCases := []struct {
		desc     string
		lp       api.LoginProvider
		expected http.Header
		err      string
	}{
		{
			desc:     "Non-empty session token is valid",
			expected: jujuhttp.BasicAuthHeader("", "test-token"),
			lp:       api.NewSessionTokenLoginProvider("test-token", &output, nil),
		},
		{
			desc: "Empty session token returns error",
			lp:   api.NewSessionTokenLoginProvider("", &output, nil),
			err:  "login provider needs to be logged in",
		},
	}
	for i, tC := range testCases {
		c.Logf("test %d: %s", i, tC.desc)
		header, err := tC.lp.AuthHeader()
		if tC.err != "" {
			c.Assert(err, tc.ErrorMatches, tC.err)
		} else {
			c.Assert(err, tc.ErrorIsNil)
			c.Check(tC.expected, tc.DeepEquals, header)
		}
	}
}
