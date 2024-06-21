// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api_test

import (
	"encoding/json"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
)

type clientCredentialsLoginProviderProviderSuite struct {
	jujutesting.JujuConnSuite
}

var _ = gc.Suite(&clientCredentialsLoginProviderProviderSuite{})

func (s *clientCredentialsLoginProviderProviderSuite) Test(c *gc.C) {
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

	apiState, err := api.Open(&api.Info{
		Addrs:          info.Addrs,
		ControllerUUID: info.ControllerUUID,
		CACert:         info.CACert,
	}, api.DialOpts{
		LoginProvider: api.NewClientCredentialsLoginProvider(clientID, clientSecret),
	})
	c.Assert(err, jc.ErrorIsNil)

	defer func() { _ = apiState.Close() }()
}
