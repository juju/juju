// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
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
	jujuparams "github.com/juju/juju/rpc/params"
)

type clientCredentialsLoginProviderProviderSuite struct {
	testing.BaseSuite
}

func TestClientCredentialsLoginProviderProviderSuite(t *stdtesting.T) {
	tc.Run(t, &clientCredentialsLoginProviderProviderSuite{})
}
func (s *clientCredentialsLoginProviderProviderSuite) APIInfo() *api.Info {
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

func (s *clientCredentialsLoginProviderProviderSuite) TestClientCredentialsLogin(c *tc.C) {
	info := s.APIInfo()

	clientID := "test-client-id"
	clientSecret := "test-client-secret"

	s.PatchValue(api.LoginWithClientCredentialsAPICall, func(ctx context.Context, _ base.APICaller, request interface{}, response interface{}) error {
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

		loginResult, ok := response.(*jujuparams.LoginResult)
		if !ok {
			return errors.Errorf("expected %T, received %T for response type", loginResult, response)
		}
		loginResult.ControllerTag = names.NewControllerTag(info.ControllerUUID).String()
		loginResult.ServerVersion = "3.4.0"
		loginResult.UserInfo = &jujuparams.AuthUserInfo{
			DisplayName:      "alice@external",
			Identity:         names.NewUserTag("alice@external").String(),
			ControllerAccess: "superuser",
		}
		return nil
	})

	lp := api.NewClientCredentialsLoginProvider(clientID, clientSecret)
	apiState, err := api.Open(c.Context(), &api.Info{
		Addrs:          info.Addrs,
		ControllerUUID: info.ControllerUUID,
		CACert:         info.CACert,
	}, api.DialOpts{
		LoginProvider: lp,
	})
	c.Assert(err, tc.ErrorIsNil)
	defer func() { _ = apiState.Close() }()
	c.Check(err, tc.ErrorIsNil)
}

// A separate suite for tests that don't need to communicate with a Juju controller.
type clientCredentialsLoginProviderBasicSuite struct {
	testing.BaseSuite
}

func TestClientCredentialsLoginProviderBasicSuite(t *stdtesting.T) {
	tc.Run(t, &clientCredentialsLoginProviderBasicSuite{})
}
func (s *clientCredentialsLoginProviderBasicSuite) TestClientCredentialsAuthHeader(c *tc.C) {
	clientID := "test-client-id"
	clientSecret := "test-client-secret"
	lp := api.NewClientCredentialsLoginProvider(clientID, clientSecret)
	expectedHeader := jujuhttp.BasicAuthHeader(clientID, clientSecret)
	got, err := lp.AuthHeader()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got, tc.DeepEquals, expectedHeader)
}

func (s *clientCredentialsLoginProviderBasicSuite) TestNewClientCredentialsLoginProviderFromEnvironment_NotSet(c *tc.C) {
	ctx := context.Background()

	_, err := api.NewClientCredentialsLoginProviderFromEnvironment(func() {}).Login(ctx, nil)
	c.Assert(err, tc.ErrorMatches, "both client id and client secret must be set")
}

func (s *clientCredentialsLoginProviderBasicSuite) TestNewClientCredentialsLoginProviderFromEnvironment(c *tc.C) {
	ctx := context.Background()

	os.Setenv("JUJU_CLIENT_ID", "test-client-id")
	os.Setenv("JUJU_CLIENT_SECRET", "test-client-secret")
	defer func() {
		os.Unsetenv("JUJU_CLIENT_ID")
		os.Unsetenv("JUJU_CLIENT_SECRET")
	}()
	res, err := api.NewClientCredentialsLoginProviderFromEnvironment(func() {}).Login(ctx, callStub{})
	c.Assert(err, tc.IsNil)
	c.Assert(res, tc.NotNil)
}

type callStub struct {
	base.APICaller
}

func (c callStub) APICall(_ context.Context, objType string, version int, id string, request string, params interface{}, response interface{}) error {
	if r, ok := response.(*jujuparams.LoginResult); ok {
		r.ServerVersion = "3.6.9"
	} else {
		return fmt.Errorf("unexpected response type %T", response)
	}
	return nil
}
