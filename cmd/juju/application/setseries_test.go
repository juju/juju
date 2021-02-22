// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/application"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
)

type setSeriesSuite struct {
	testing.IsolationSuite
	mockApplicationAPI *mockSetApplicationSeriesAPI
}

var _ = gc.Suite(&setSeriesSuite{})

func (s *setSeriesSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.mockApplicationAPI = &mockSetApplicationSeriesAPI{Stub: &testing.Stub{}}
}

func (s *setSeriesSuite) runUpdateSeries(c *gc.C, args ...string) (*cmd.Context, error) {
	store := jujuclienttesting.MinimalStore()
	return cmdtesting.RunCommand(c, application.NewSetSeriesCommandForTest(s.mockApplicationAPI, store), args...)
}

func (s *setSeriesSuite) TestSetSeriesApplicationGoodPath(c *gc.C) {
	_, err := s.runUpdateSeries(c, "ghost", "xenial")
	c.Assert(err, jc.ErrorIsNil)
	s.mockApplicationAPI.CheckCall(c, 0, "UpdateApplicationSeries", "ghost", "xenial", false)
}

func (s *setSeriesSuite) TestSetSeriesApplicationGoodPathForce(c *gc.C) {
	_, err := s.runUpdateSeries(c, "--force", "ghost", "xenial")
	c.Assert(err, jc.ErrorIsNil)
	s.mockApplicationAPI.CheckCall(c, 0, "UpdateApplicationSeries", "ghost", "xenial", true)
}

func (s *setSeriesSuite) TestNoArguments(c *gc.C) {
	_, err := s.runUpdateSeries(c)
	c.Assert(err, gc.ErrorMatches, "application name and series required")
}

func (s *setSeriesSuite) TestArgumentsSeriesOnly(c *gc.C) {
	_, err := s.runUpdateSeries(c, "ghost")
	c.Assert(err, gc.ErrorMatches, "no series specified")
}

func (s *setSeriesSuite) TestArgumentsApplicationOnly(c *gc.C) {
	_, err := s.runUpdateSeries(c, "xenial")
	c.Assert(err, gc.ErrorMatches, "no application name")
}

func (s *setSeriesSuite) TestTooManyArguments(c *gc.C) {
	_, err := s.runUpdateSeries(c, "ghost", "xenial", "something else")
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["something else"\]`, gc.Commentf("details: %s", errors.Details(err)))
}

type mockSetApplicationSeriesAPI struct {
	*testing.Stub
}

func (a *mockSetApplicationSeriesAPI) Close() error {
	a.MethodCall(a, "Close")
	return a.NextErr()
}

func (a *mockSetApplicationSeriesAPI) UpdateApplicationSeries(appName, series string, force bool) error {
	a.MethodCall(a, "UpdateApplicationSeries", appName, series, force)
	return a.NextErr()
}
