// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api_test

import (
	"encoding/json"
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
)

type sessionTokenLoginProviderProviderSuite struct {
	jujutesting.JujuConnSuite
}

var _ = gc.Suite(&sessionTokenLoginProviderProviderSuite{})

func (s *sessionTokenLoginProviderProviderSuite) Test(c *gc.C) {
	info := s.APIInfo(c)

	sessionToken := "test-session-token"
	userCode := "1234567"
	verificationURI := "http://localhost:8080/test-verification"

	var loginDetails string
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
				Message: "unauthorized",
				Code:    params.CodeUnauthorized,
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

	apiState, err := api.Open(&api.Info{
		Addrs:          info.Addrs,
		ControllerUUID: info.ControllerUUID,
		CACert:         info.CACert,
	}, api.DialOpts{
		LoginProvider: api.NewSessionTokenLoginProvider(
			"expired-token",
			func(s string, a ...any) error {
				loginDetails = fmt.Sprintf(s, a...)
				return nil
			},
			func(sessionToken string) error {
				obtainedSessionToken = sessionToken
				return nil
			},
		),
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(loginDetails, gc.Equals, "Please visit http://localhost:8080/test-verification and enter code 1234567 to log in.")
	c.Assert(obtainedSessionToken, gc.Equals, sessionToken)
	defer func() { _ = apiState.Close() }()
}
