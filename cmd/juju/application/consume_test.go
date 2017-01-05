// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/application"
	coretesting "github.com/juju/juju/testing"
)

type ConsumeSuite struct {
	testing.IsolationSuite
	mockAPI *mockConsumeAPI
}

var _ = gc.Suite(&ConsumeSuite{})

func (s *ConsumeSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.mockAPI = &mockConsumeAPI{Stub: &testing.Stub{}}
}

func (s *ConsumeSuite) runConsume(c *gc.C, args ...string) (*cmd.Context, error) {
	return coretesting.RunCommand(c, application.NewConsumeCommandForTest(s.mockAPI), args...)
}

func (s *ConsumeSuite) TestNoArguments(c *gc.C) {
	_, err := s.runConsume(c)
	c.Assert(err, gc.ErrorMatches, "no remote application specified")
}

func (s *ConsumeSuite) TestTooManyArguments(c *gc.C) {
	_, err := s.runConsume(c, "model.application", "something else")
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["something else"\]`)
}

func (s *ConsumeSuite) TestInvalidRemoteApplication(c *gc.C) {

	c.Fatalf("writeme")
}

func (s *ConsumeSuite) TestErrorFromAPI(c *gc.C) {
	s.mockAPI.SetErrors(errors.New("infirmary"))
	_, err := s.runConsume(c, "model.application")
	c.Assert(err, gc.ErrorMatches, "infirmary")
}

func (s *ConsumeSuite) TestSuccessUrl(c *gc.C) {
	s.mockAPI.localName = "careless-love"
	ctx, err := s.runConsume(c, "local:/u/james/hill")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(coretesting.Stderr(ctx), gc.Equals, "Added local:/u/james/hill as careless-love\n")
}

func (s *ConsumeSuite) TestSuccessModelDotApplication(c *gc.C) {
	s.mockAPI.localName = "mary-weep"
	ctx, err := s.runConsume(c, "booster.uke")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(coretesting.Stderr(ctx), gc.Equals, "Added booster.uke as mary-weep\n")
}

type mockConsumeAPI struct {
	*testing.Stub

	localName string
}

func (a *mockConsumeAPI) Close() error {
	a.MethodCall(a, "Close")
	return a.NextErr()
}

func (a *mockConsumeAPI) Consume(remoteApplication string) (string, error) {
	a.MethodCall(a, "Consume", remoteApplication)
	return a.localName, a.NextErr()
}
