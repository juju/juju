// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/cmdtesting"
	"github.com/juju/juju/cmd/juju/application"
	"github.com/juju/juju/jujuclient"
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
		CurrentModel: "fred/test",
		Models: map[string]jujuclient.ModelDetails{
			"bob/test": {"test-uuid"},
			"bob/prod": {"prod-uuid"},
		},
	}
	s.store.Accounts[controllerName] = jujuclient.AccountDetails{
		User: "bob",
	}
}

func (s *ConsumeSuite) runConsume(c *gc.C, args ...string) (*cmd.Context, error) {
	return cmdtesting.RunCommand(c, application.NewConsumeCommandForTest(s.store, s.mockAPI), args...)
}

func (s *ConsumeSuite) TestNoArguments(c *gc.C) {
	_, err := s.runConsume(c)
	c.Assert(err, gc.ErrorMatches, "no remote application specified")
}

func (s *ConsumeSuite) TestTooManyArguments(c *gc.C) {
	_, err := s.runConsume(c, "model.application", "alias", "something else")
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["something else"\]`)
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

func (s *ConsumeSuite) TestSuccessModelDotApplication(c *gc.C) {
	s.mockAPI.localName = "mary-weep"
	ctx, err := s.runConsume(c, "booster.uke")
	c.Assert(err, jc.ErrorIsNil)
	s.mockAPI.CheckCalls(c, []testing.StubCall{
		{"Consume", []interface{}{"bob/booster.uke", ""}},
		{"Close", nil},
	})
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "Added bob/booster.uke as mary-weep\n")
}

func (s *ConsumeSuite) TestSuccessModelDotApplicationWithAlias(c *gc.C) {
	s.mockAPI.localName = "mary-weep"
	ctx, err := s.runConsume(c, "booster.uke", "alias")
	c.Assert(err, jc.ErrorIsNil)
	s.mockAPI.CheckCalls(c, []testing.StubCall{
		{"Consume", []interface{}{"bob/booster.uke", "alias"}},
		{"Close", nil},
	})
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "Added bob/booster.uke as mary-weep\n")
}

type mockConsumeAPI struct {
	*testing.Stub

	localName string
}

func (a *mockConsumeAPI) Close() error {
	a.MethodCall(a, "Close")
	return a.NextErr()
}

func (a *mockConsumeAPI) Consume(remoteApplication, alias string) (string, error) {
	a.MethodCall(a, "Consume", remoteApplication, alias)
	return a.localName, a.NextErr()
}
