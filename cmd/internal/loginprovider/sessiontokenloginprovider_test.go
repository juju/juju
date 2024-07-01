// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package loginprovider_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/cmd/internal/loginprovider"
	"github.com/juju/juju/rpc/params"
	jtesting "github.com/juju/juju/testing"
	jujuversion "github.com/juju/juju/version"
)

type sessionTokenLoginProviderProviderSuite struct {
	jtesting.BaseSuite
}

var _ = gc.Suite(&sessionTokenLoginProviderProviderSuite{})

func (s *sessionTokenLoginProviderProviderSuite) APIInfo() *api.Info {
	srv := apiservertesting.NewAPIServer(func(modelUUID string) (interface{}, error) {
		var err error
		if modelUUID != "" && modelUUID != jtesting.ModelTag.Id() {
			err = fmt.Errorf("%w: %q", apiservererrors.UnknownModelError, modelUUID)
		}
		return &testRootAPI{}, err
	})
	s.AddCleanup(func(_ *gc.C) { srv.Close() })
	info := &api.Info{
		Addrs:          srv.Addrs,
		CACert:         jtesting.CACert,
		ControllerUUID: jtesting.ControllerTag.Id(),
		ModelTag:       jtesting.ModelTag,
	}
	return info
}

type testRootAPI struct {
	serverAddrs [][]params.HostPort
}

func (r testRootAPI) Admin(id string) (testAdminAPI, error) {
	return testAdminAPI{r: r}, nil
}

type testAdminAPI struct {
	r testRootAPI
}

func (a testAdminAPI) Login(req params.LoginRequest) params.LoginResult {
	return params.LoginResult{
		ControllerTag: jtesting.ControllerTag.String(),
		ModelTag:      jtesting.ModelTag.String(),
		Servers:       a.r.serverAddrs,
		ServerVersion: jujuversion.Current.String(),
		PublicDNSName: "somewhere.example.com",
	}
}

func (s *sessionTokenLoginProviderProviderSuite) Test(c *gc.C) {
	info := s.APIInfo()

	sessionToken := "test-session-token"
	userCode := "1234567"
	verificationURI := "http://localhost:8080/test-verification"

	var obtainedSessionToken string

	s.PatchValue(loginprovider.LoginDeviceAPICall, func(ctx context.Context, _ base.APICaller, request interface{}, response interface{}) error {
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

	s.PatchValue(loginprovider.GetDeviceSessionTokenAPICall, func(ctx context.Context, _ base.APICaller, request interface{}, response interface{}) error {
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

	s.PatchValue(loginprovider.LoginWithSessionTokenAPICall, func(ctx context.Context, _ base.APICaller, request interface{}, response interface{}) error {
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

	var output bytes.Buffer
	apiState, err := api.Open(&api.Info{
		Addrs:          info.Addrs,
		ControllerUUID: info.ControllerUUID,
		CACert:         info.CACert,
	}, api.DialOpts{
		LoginProvider: loginprovider.NewSessionTokenLoginProvider(
			"expired-token",
			&output,
			func(sessionToken string) error {
				obtainedSessionToken = sessionToken
				return nil
			},
		),
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(output.String(), gc.Equals, "Please visit http://localhost:8080/test-verification and enter code 1234567 to log in.\n")
	c.Assert(obtainedSessionToken, gc.Equals, sessionToken)
	defer func() { _ = apiState.Close() }()
}
