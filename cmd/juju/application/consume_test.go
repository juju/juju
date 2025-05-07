// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/cmd/juju/application"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/rpc/params"
)

type ConsumeSuite struct {
	testhelpers.IsolationSuite
	mockAPI *mockConsumeAPI
	store   *jujuclient.MemStore
}

var _ = tc.Suite(&ConsumeSuite{})

func (s *ConsumeSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.mockAPI = &mockConsumeAPI{Stub: &testhelpers.Stub{}}

	// Set up the current controller, and write just enough info
	// so we don't try to refresh
	controllerName := "test-master"
	s.store = jujuclient.NewMemStore()
	s.store.CurrentControllerName = controllerName
	s.store.Controllers[controllerName] = jujuclient.ControllerDetails{}
	s.store.Models[controllerName] = &jujuclient.ControllerModels{
		CurrentModel: "bob/test",
		Models: map[string]jujuclient.ModelDetails{
			"bob/test": {ModelUUID: "test-uuid", ModelType: model.IAAS},
			"bob/prod": {ModelUUID: "prod-uuid", ModelType: model.IAAS},
		},
	}
	s.store.Accounts[controllerName] = jujuclient.AccountDetails{
		User: "bob",
	}
}

func (s *ConsumeSuite) runConsume(c *tc.C, args ...string) (*cmd.Context, error) {
	return cmdtesting.RunCommand(c, application.NewConsumeCommandForTest(s.store, s.mockAPI, s.mockAPI), args...)
}

func (s *ConsumeSuite) TestNoArguments(c *tc.C) {
	_, err := s.runConsume(c)
	c.Assert(err, tc.ErrorMatches, "no remote offer specified")
}

func (s *ConsumeSuite) TestTooManyArguments(c *tc.C) {
	_, err := s.runConsume(c, "model.application", "alias", "something else")
	c.Assert(err, tc.ErrorMatches, `unrecognized args: \["something else"\]`, tc.Commentf("details: %s", errors.Details(err)))
}

func (s *ConsumeSuite) TestInvalidRemoteApplication(c *tc.C) {
	badApplications := []string{
		"application",
		"user/model.application:endpoint",
		"user/model",
		"unknown:/wherever",
	}
	for _, bad := range badApplications {
		c.Logf(bad)
		_, err := s.runConsume(c, bad)
		c.Check(err != nil, tc.IsTrue)
	}
}

func (s *ConsumeSuite) TestErrorFromAPI(c *tc.C) {
	s.mockAPI.SetErrors(errors.New("infirmary"))
	_, err := s.runConsume(c, "model.application")
	c.Assert(err, tc.ErrorMatches, "infirmary")
}

func (s *ConsumeSuite) TestConsumeBlocked(c *tc.C) {
	s.mockAPI.SetErrors(nil, &params.Error{Code: params.CodeOperationBlocked, Message: "nope"})
	_, err := s.runConsume(c, "model.application")
	s.mockAPI.CheckCallNames(c, "GetConsumeDetails", "Consume", "Close", "Close")
	c.Assert(err.Error(), tc.Contains, `could not consume bob/model.application: nope`)
	c.Assert(err.Error(), tc.Contains, `All operations that change model have been disabled for the current model.`)
}

func (s *ConsumeSuite) assertSuccessModelDotApplication(c *tc.C, alias string) {
	s.mockAPI.localName = "mary-weep"
	var (
		ctx *cmd.Context
		err error
	)
	if alias != "" {
		ctx, err = s.runConsume(c, "ctrl:booster.uke", alias)
	} else {
		ctx, err = s.runConsume(c, "ctrl:booster.uke")
	}
	c.Assert(err, tc.ErrorIsNil)
	mac, err := jujutesting.NewMacaroon("id")
	c.Assert(err, tc.ErrorIsNil)
	s.mockAPI.CheckCalls(c, []testhelpers.StubCall{
		{"GetConsumeDetails", []interface{}{"bob/booster.uke"}},
		{"Consume", []interface{}{crossmodel.ConsumeApplicationArgs{
			Offer:            params.ApplicationOfferDetailsV5{OfferName: "an offer", OfferURL: "ctrl:bob/booster.uke"},
			ApplicationAlias: alias,
			Macaroon:         mac,
			ControllerInfo: &crossmodel.ControllerInfo{
				ControllerUUID: coretesting.ControllerTag.Id(),
				Alias:          "controller-alias",
				Addrs:          []string{"192.168.1:1234"},
				CACert:         coretesting.CACert,
			},
		},
		}},
		{"Close", nil},
		{"Close", nil},
	})
	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, "Added ctrl:bob/booster.uke as mary-weep\n")
}

func (s *ConsumeSuite) TestSuccessModelDotApplication(c *tc.C) {
	s.assertSuccessModelDotApplication(c, "")
}

func (s *ConsumeSuite) TestSuccessModelDotApplicationWithAlias(c *tc.C) {
	s.assertSuccessModelDotApplication(c, "alias")
}

type mockConsumeAPI struct {
	*testhelpers.Stub

	localName string
}

func (a *mockConsumeAPI) Close() error {
	a.MethodCall(a, "Close")
	return a.NextErr()
}

func (a *mockConsumeAPI) Consume(ctx context.Context, arg crossmodel.ConsumeApplicationArgs) (string, error) {
	a.MethodCall(a, "Consume", arg)
	return a.localName, a.NextErr()
}

func (a *mockConsumeAPI) GetConsumeDetails(ctx context.Context, url string) (params.ConsumeOfferDetails, error) {
	a.MethodCall(a, "GetConsumeDetails", url)
	mac, err := jujutesting.NewMacaroon("id")
	if err != nil {
		return params.ConsumeOfferDetails{}, err
	}
	return params.ConsumeOfferDetails{
		Offer:    &params.ApplicationOfferDetailsV5{OfferName: "an offer", OfferURL: "bob/booster.uke"},
		Macaroon: mac,
		ControllerInfo: &params.ExternalControllerInfo{
			ControllerTag: coretesting.ControllerTag.String(),
			Alias:         "controller-alias",
			Addrs:         []string{"192.168.1:1234"},
			CACert:        coretesting.CACert,
		},
	}, a.NextErr()
}
