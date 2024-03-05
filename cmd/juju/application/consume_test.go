// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	"github.com/juju/cmd/v4"
	"github.com/juju/cmd/v4/cmdtesting"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/application"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/model"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/rpc/params"
	coretesting "github.com/juju/juju/testing"
)

type ConsumeSuite struct {
	testing.IsolationSuite
	mockAPI *mockConsumeAPI
	store   *jujuclient.MemStore
}

var _ = gc.Suite(&ConsumeSuite{})

func (s *ConsumeSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.mockAPI = &mockConsumeAPI{Stub: &testing.Stub{}}

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

func (s *ConsumeSuite) runConsume(c *gc.C, args ...string) (*cmd.Context, error) {
	return cmdtesting.RunCommand(c, application.NewConsumeCommandForTest(s.store, s.mockAPI, s.mockAPI), args...)
}

func (s *ConsumeSuite) TestNoArguments(c *gc.C) {
	_, err := s.runConsume(c)
	c.Assert(err, gc.ErrorMatches, "no remote offer specified")
}

func (s *ConsumeSuite) TestTooManyArguments(c *gc.C) {
	_, err := s.runConsume(c, "model.application", "alias", "something else")
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["something else"\]`, gc.Commentf("details: %s", errors.Details(err)))
}

func (s *ConsumeSuite) TestInvalidRemoteApplication(c *gc.C) {
	badApplications := []string{
		"application",
		"user/model.application:endpoint",
		"user/model",
		"unknown:/wherever",
	}
	for _, bad := range badApplications {
		c.Logf(bad)
		_, err := s.runConsume(c, bad)
		c.Check(err != nil, jc.IsTrue)
	}
}

func (s *ConsumeSuite) TestErrorFromAPI(c *gc.C) {
	s.mockAPI.SetErrors(errors.New("infirmary"))
	_, err := s.runConsume(c, "model.application")
	c.Assert(err, gc.ErrorMatches, "infirmary")
}

func (s *ConsumeSuite) TestConsumeBlocked(c *gc.C) {
	s.mockAPI.SetErrors(nil, &params.Error{Code: params.CodeOperationBlocked, Message: "nope"})
	_, err := s.runConsume(c, "model.application")
	s.mockAPI.CheckCallNames(c, "GetConsumeDetails", "Consume", "Close", "Close")
	c.Assert(err.Error(), jc.Contains, `could not consume bob/model.application: nope`)
	c.Assert(err.Error(), jc.Contains, `All operations that change model have been disabled for the current model.`)
}

func (s *ConsumeSuite) assertSuccessModelDotApplication(c *gc.C, alias string) {
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
	c.Assert(err, jc.ErrorIsNil)
	mac, err := jujutesting.NewMacaroon("id")
	c.Assert(err, jc.ErrorIsNil)
	s.mockAPI.CheckCalls(c, []testing.StubCall{
		{"GetConsumeDetails", []interface{}{"bob/booster.uke"}},
		{"Consume", []interface{}{crossmodel.ConsumeApplicationArgs{
			Offer:            params.ApplicationOfferDetails{OfferName: "an offer", OfferURL: "ctrl:bob/booster.uke"},
			ApplicationAlias: alias,
			Macaroon:         mac,
			ControllerInfo: &crossmodel.ControllerInfo{
				ControllerTag: coretesting.ControllerTag,
				Alias:         "controller-alias",
				Addrs:         []string{"192.168.1:1234"},
				CACert:        coretesting.CACert,
			},
		},
		}},
		{"Close", nil},
		{"Close", nil},
	})
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "Added ctrl:bob/booster.uke as mary-weep\n")
}

func (s *ConsumeSuite) TestSuccessModelDotApplication(c *gc.C) {
	s.assertSuccessModelDotApplication(c, "")
}

func (s *ConsumeSuite) TestSuccessModelDotApplicationWithAlias(c *gc.C) {
	s.assertSuccessModelDotApplication(c, "alias")
}

type mockConsumeAPI struct {
	*testing.Stub

	localName string
}

func (a *mockConsumeAPI) Close() error {
	a.MethodCall(a, "Close")
	return a.NextErr()
}

func (a *mockConsumeAPI) Consume(arg crossmodel.ConsumeApplicationArgs) (string, error) {
	a.MethodCall(a, "Consume", arg)
	return a.localName, a.NextErr()
}

func (a *mockConsumeAPI) GetConsumeDetails(url string) (params.ConsumeOfferDetails, error) {
	a.MethodCall(a, "GetConsumeDetails", url)
	mac, err := jujutesting.NewMacaroon("id")
	if err != nil {
		return params.ConsumeOfferDetails{}, err
	}
	return params.ConsumeOfferDetails{
		Offer:    &params.ApplicationOfferDetails{OfferName: "an offer", OfferURL: "bob/booster.uke"},
		Macaroon: mac,
		ControllerInfo: &params.ExternalControllerInfo{
			ControllerTag: coretesting.ControllerTag.String(),
			Alias:         "controller-alias",
			Addrs:         []string{"192.168.1:1234"},
			CACert:        coretesting.CACert,
		},
	}, a.NextErr()
}
