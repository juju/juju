// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	"context"
	"testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/api/jujuclient"
	"github.com/juju/juju/cmd/cmd"
	"github.com/juju/juju/cmd/cmd/cmdtesting"
	"github.com/juju/juju/cmd/juju/application"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
)

type ConsumeSuite struct {
	testhelpers.IsolationSuite
	mockAPI *mockConsumeAPI
	store   *jujuclient.MemStore
}

func TestConsumeSuite(t *testing.T) {
	tc.Run(t, &ConsumeSuite{})
}

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
		c.Logf("%s", bad)
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
		{"GetConsumeDetails", []any{"bob/booster.uke"}},
		{"Consume", []any{crossmodel.ConsumeApplicationArgs{
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

// newResolverConsumeCommand builds a consume command whose source offers API is
// resolved per controller from the given map, so the resolution fan-out can be
// exercised. A controller absent from the map yields a "not found" offer.
func (s *ConsumeSuite) newResolverConsumeCommand(hosts map[string]bool) (cmd.Command, *testhelpers.Stub) {
	targetStub := &testhelpers.Stub{}
	target := &mockConsumeAPI{Stub: targetStub, localName: "mary-weep"}
	sources := &testhelpers.Stub{}
	newSource := func(controllerName string) (application.ApplicationConsumeDetailsAPIForTest, error) {
		return &mockOfferSourceAPI{
			Stub:      sources,
			hostsIt:   hosts[controllerName],
			offerURL:  "bob/booster.uke",
			offerName: "an offer",
		}, nil
	}
	command := application.NewConsumeCommandForTestWithSourceResolver(s.store, target, newSource)
	return command, targetStub
}

// consumedOfferURL extracts the OfferURL from the (single) Consume call
// recorded on the target stub, which is where the resolved source controller
// is stamped.
func consumedOfferURL(c *tc.C, targetStub *testhelpers.Stub) string {
	for _, call := range targetStub.Calls() {
		if call.FuncName != "Consume" {
			continue
		}
		c.Assert(call.Args, tc.HasLen, 1)
		arg, ok := call.Args[0].(crossmodel.ConsumeApplicationArgs)
		c.Assert(ok, tc.IsTrue)
		return arg.Offer.OfferURL
	}
	c.Fatalf("no Consume call recorded")
	return ""
}

func (s *ConsumeSuite) TestConsumeResolvesSourceControllerWhenUnqualified(c *tc.C) {
	// Register a second controller that actually hosts the offer. The current
	// controller (test-master) does not.
	s.store.Controllers["other"] = jujuclient.ControllerDetails{}
	s.store.Accounts["other"] = jujuclient.AccountDetails{User: "bob"}

	command, targetStub := s.newResolverConsumeCommand(map[string]bool{"other": true})

	_, err := cmdtesting.RunCommand(c, command, "booster.uke")
	c.Assert(err, tc.ErrorIsNil)
	// The offer was resolved to the "other" controller and its URL is
	// namespaced accordingly on the consumed offer.
	c.Check(consumedOfferURL(c, targetStub), tc.Equals, "other:bob/booster.uke")
}

func (s *ConsumeSuite) TestConsumeUnqualifiedNotFoundAnywhere(c *tc.C) {
	s.store.Controllers["other"] = jujuclient.ControllerDetails{}
	s.store.Accounts["other"] = jujuclient.AccountDetails{User: "bob"}

	// No controller hosts the offer.
	command, _ := s.newResolverConsumeCommand(map[string]bool{})

	_, err := cmdtesting.RunCommand(c, command, "booster.uke")
	c.Assert(err, tc.ErrorMatches, `offer "bob/booster.uke" on any registered controller not found`)
}

func (s *ConsumeSuite) TestConsumeUnqualifiedPrefersCurrentController(c *tc.C) {
	s.store.Controllers["other"] = jujuclient.ControllerDetails{}
	s.store.Accounts["other"] = jujuclient.AccountDetails{User: "bob"}

	// Both controllers host it; the current controller (test-master) must win.
	command, targetStub := s.newResolverConsumeCommand(map[string]bool{"test-master": true, "other": true})

	_, err := cmdtesting.RunCommand(c, command, "booster.uke")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(consumedOfferURL(c, targetStub), tc.Equals, "test-master:bob/booster.uke")
}

func (s *ConsumeSuite) TestConsumeExplicitSourceSkipsResolution(c *tc.C) {
	s.store.Controllers["other"] = jujuclient.ControllerDetails{}
	s.store.Accounts["other"] = jujuclient.AccountDetails{User: "bob"}

	// Only "other" hosts it; naming it explicitly must target it directly.
	command, targetStub := s.newResolverConsumeCommand(map[string]bool{"other": true})

	_, err := cmdtesting.RunCommand(c, command, "other:booster.uke")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(consumedOfferURL(c, targetStub), tc.Equals, "other:bob/booster.uke")
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

// mockOfferSourceAPI is a source offers client that reports whether it hosts a
// requested offer. Used to exercise the per-controller consume resolution.
type mockOfferSourceAPI struct {
	*testhelpers.Stub

	hostsIt   bool
	offerURL  string
	offerName string
}

func (a *mockOfferSourceAPI) Close() error {
	a.MethodCall(a, "Close")
	return nil
}

func (a *mockOfferSourceAPI) GetConsumeDetails(ctx context.Context, url string) (params.ConsumeOfferDetails, error) {
	a.MethodCall(a, "GetConsumeDetails", url)
	if !a.hostsIt {
		return params.ConsumeOfferDetails{}, errors.NotFoundf("offer %q", url)
	}
	mac, err := jujutesting.NewMacaroon("id")
	if err != nil {
		return params.ConsumeOfferDetails{}, err
	}
	return params.ConsumeOfferDetails{
		Offer:    &params.ApplicationOfferDetailsV5{OfferName: a.offerName, OfferURL: a.offerURL},
		Macaroon: mac,
		ControllerInfo: &params.ExternalControllerInfo{
			ControllerTag: coretesting.ControllerTag.String(),
			Alias:         "controller-alias",
			Addrs:         []string{"192.168.1:1234"},
			CACert:        coretesting.CACert,
		},
	}, nil
}
