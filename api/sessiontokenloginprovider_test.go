// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"

	"github.com/juju/errors"
	jujuhttp "github.com/juju/http/v2"
	"github.com/juju/names/v6"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
	coretesting "github.com/juju/juju/testing"
)

type sessionTokenLoginProviderSuite struct {
	jujutesting.JujuConnSuite
}

var _ = gc.Suite(&sessionTokenLoginProviderSuite{})

func (s *sessionTokenLoginProviderSuite) TestSessionTokenLogin(c *gc.C) {
	info := s.APIInfo(c)

	sessionToken := "test-session-token"
	userCode := "1234567"
	verificationURI := "http://localhost:8080/test-verification"

	var obtainedSessionToken string

	s.PatchValue(api.LoginDeviceAPICall, func(_ base.APICaller, request interface{}, response interface{}) error {
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

	s.PatchValue(api.GetDeviceSessionTokenAPICall, func(_ base.APICaller, request interface{}, response interface{}) error {
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

	s.PatchValue(api.LoginWithSessionTokenAPICall, func(_ base.APICaller, request interface{}, response interface{}) error {
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
	apiState, err := api.Open(&api.Info{
		Addrs:          info.Addrs,
		ControllerUUID: info.ControllerUUID,
		CACert:         info.CACert,
	}, api.DialOpts{
		LoginProvider: lp,
	})
	c.Assert(err, jc.ErrorIsNil)
	defer func() { _ = apiState.Close() }()

	c.Check(output.String(), gc.Equals, "Please visit http://localhost:8080/test-verification and enter code 1234567 to log in.\n")
	c.Check(obtainedSessionToken, gc.Equals, sessionToken)
	c.Check(err, jc.ErrorIsNil)
}

func (s *sessionTokenLoginProviderSuite) TestInvalidSessionTokenLogin(c *gc.C) {
	info := s.APIInfo(c)

	expectedErr := &params.Error{
		Message: "unauthorized",
		Code:    params.CodeUnauthorized,
	}
	s.PatchValue(api.LoginWithSessionTokenAPICall, func(_ base.APICaller, request interface{}, response interface{}) error {
		return expectedErr
	})

	var output bytes.Buffer
	_, err := api.Open(&api.Info{
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
	c.Assert(err, jc.ErrorIs, expectedErr)
}

// A separate suite for tests that don't need to communicate with a controller.
type sessionTokenLoginProviderBasicSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&sessionTokenLoginProviderBasicSuite{})

func (s *sessionTokenLoginProviderBasicSuite) TestSessionTokenAuthHeader(c *gc.C) {
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
			c.Assert(err, gc.ErrorMatches, tC.err)
		} else {
			c.Assert(err, jc.ErrorIsNil)
			c.Check(tC.expected, gc.DeepEquals, header)
		}
	}
}
