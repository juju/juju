// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/juju/errors"
	jujuhttp "github.com/juju/http/v2"
	"github.com/juju/names/v6"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
	jujuparams "github.com/juju/juju/rpc/params"
	coretesting "github.com/juju/juju/testing"
)

type clientCredentialsLoginProviderSuite struct {
	jujutesting.JujuConnSuite
}

var _ = gc.Suite(&clientCredentialsLoginProviderSuite{})

func (s *clientCredentialsLoginProviderSuite) TestClientCredentialsLogin(c *gc.C) {
	info := s.APIInfo(c)

	clientID := "test-client-id"
	clientSecret := "test-client-secret"

	s.PatchValue(api.LoginWithClientCredentialsAPICall, func(_ base.APICaller, request interface{}, response interface{}) error {
		data, err := json.Marshal(request)
		if err != nil {
			return errors.Trace(err)
		}

		var lr struct {
			ClientID     string `json:"client-id"`
			ClientSecret string `json:"client-secret"`
		}

		err = json.Unmarshal(data, &lr)
		if err != nil {
			return errors.Trace(err)
		}

		if lr.ClientID != clientID {
			return errors.Unauthorized
		}
		if lr.ClientSecret != clientSecret {
			return errors.Unauthorized
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

	lp := api.NewClientCredentialsLoginProvider(clientID, clientSecret)
	apiState, err := api.Open(&api.Info{
		Addrs:          info.Addrs,
		ControllerUUID: info.ControllerUUID,
		CACert:         info.CACert,
	}, api.DialOpts{
		LoginProvider: lp,
	})
	c.Assert(err, jc.ErrorIsNil)
	defer func() { _ = apiState.Close() }()
	c.Check(err, jc.ErrorIsNil)
}

// A separate suite for tests that don't need to communicate with a Juju controller.
type clientCredentialsLoginProviderBasicSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&clientCredentialsLoginProviderBasicSuite{})

func (s *clientCredentialsLoginProviderBasicSuite) TestClientCredentialsAuthHeader(c *gc.C) {
	clientID := "test-client-id"
	clientSecret := "test-client-secret"
	lp := api.NewClientCredentialsLoginProvider(clientID, clientSecret)
	expectedHeader := jujuhttp.BasicAuthHeader(clientID, clientSecret)
	got, err := lp.AuthHeader()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(got, jc.DeepEquals, expectedHeader)
}

func (s *clientCredentialsLoginProviderBasicSuite) TestNewClientCredentialsLoginProviderFromEnvironment_NotSet(c *gc.C) {
	ctx := context.Background()

	_, err := api.NewClientCredentialsLoginProviderFromEnvironment(func() {}).Login(ctx, nil)
	c.Assert(err, gc.ErrorMatches, "both client id and client secret must be set")
}

func (s *clientCredentialsLoginProviderBasicSuite) TestNewClientCredentialsLoginProviderFromEnvironment(c *gc.C) {
	ctx := context.Background()

	os.Setenv("JUJU_CLIENT_ID", "test-client-id")
	os.Setenv("JUJU_CLIENT_SECRET", "test-client-secret")
	defer func() {
		os.Unsetenv("JUJU_CLIENT_ID")
		os.Unsetenv("JUJU_CLIENT_SECRET")
	}()
	res, err := api.NewClientCredentialsLoginProviderFromEnvironment(func() {}).Login(ctx, callStub{})
	c.Assert(err, gc.IsNil)
	c.Assert(res, gc.NotNil)
}

type callStub struct {
	base.APICaller
}

func (c callStub) APICall(objType string, version int, id string, request string, params interface{}, response interface{}) error {
	if r, ok := response.(*jujuparams.LoginResult); ok {
		r.ServerVersion = "3.6.9"
	} else {
		return fmt.Errorf("unexpected response type %T", response)
	}
	return nil
}
