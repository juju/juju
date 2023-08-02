// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/application"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
)

type setApplicationBaseSuite struct {
	testing.IsolationSuite
	mockApplicationAPI *mockSetApplicationBaseAPI
}

var _ = gc.Suite(&setApplicationBaseSuite{})

func (s *setApplicationBaseSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.mockApplicationAPI = &mockSetApplicationBaseAPI{Stub: &testing.Stub{}}
}

func (s *setApplicationBaseSuite) runSetApplicationBase(c *gc.C, args ...string) (*cmd.Context, error) {
	store := jujuclienttesting.MinimalStore()
	return cmdtesting.RunCommand(c, application.NewSetApplicationBaseCommandForTest(s.mockApplicationAPI, store), args...)
}

func (s *setApplicationBaseSuite) TestSetSeriesApplicationGoodPath(c *gc.C) {
	_, err := s.runSetApplicationBase(c, "ghost", "ubuntu@20.04")
	c.Assert(err, jc.ErrorIsNil)
	s.mockApplicationAPI.CheckCall(c, 0, "UpdateApplicationBase", "ghost", corebase.MustParseBaseFromString("ubuntu@20.04"), false)
}

func (s *setApplicationBaseSuite) TestNoArguments(c *gc.C) {
	_, err := s.runSetApplicationBase(c)
	c.Assert(err, gc.ErrorMatches, "application name and base required")
}

func (s *setApplicationBaseSuite) TestArgumentsSeriesOnly(c *gc.C) {
	_, err := s.runSetApplicationBase(c, "ghost")
	c.Assert(err, gc.ErrorMatches, "no base specified")
}

func (s *setApplicationBaseSuite) TestArgumentsApplicationOnly(c *gc.C) {
	_, err := s.runSetApplicationBase(c, "ubuntu@20.04")
	c.Assert(err, gc.ErrorMatches, "no application name")
}

func (s *setApplicationBaseSuite) TestTooManyArguments(c *gc.C) {
	_, err := s.runSetApplicationBase(c, "ghost", "ubuntu@20.04", "something else")
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["something else"\]`, gc.Commentf("details: %s", errors.Details(err)))
}

type mockSetApplicationBaseAPI struct {
	*testing.Stub
}

func (a *mockSetApplicationBaseAPI) Close() error {
	a.MethodCall(a, "Close")
	return a.NextErr()
}

func (a *mockSetApplicationBaseAPI) UpdateApplicationBase(appName string, series corebase.Base, force bool) error {
	a.MethodCall(a, "UpdateApplicationBase", appName, series, force)
	return a.NextErr()
}
