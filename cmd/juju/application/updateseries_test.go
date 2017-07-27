// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/testing"
)

type updateSeriesSuite struct {
	testing.IsolationSuite
	mockAPI *mockUpdateSeriesAPI
}

var _ = gc.Suite(&updateSeriesSuite{})

func (s *updateSeriesSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.mockAPI = &mockUpdateSeriesAPI{Stub: &testing.Stub{}}
}

func (s *updateSeriesSuite) runUpdateSeries(c *gc.C, args ...string) (*cmd.Context, error) {
	return cmdtesting.RunCommand(c, NewUpdateSeriesCommandForTest(s.mockAPI), args...)
}

func (s *updateSeriesSuite) TestUpdateSeriesApplicationGoodPath(c *gc.C) {
	_, err := s.runUpdateSeries(c, "ghost", "xenial")
	c.Assert(err, jc.ErrorIsNil)
	s.mockAPI.CheckCall(c, 0, "UpdateApplicationSeries", "ghost", "xenial", false)
}

func (s *updateSeriesSuite) TestNoArguments(c *gc.C) {
	_, err := s.runUpdateSeries(c)
	c.Assert(err, gc.ErrorMatches, "no arguments specified")
}

func (s *updateSeriesSuite) TestNotEnoughArguments(c *gc.C) {
	_, err := s.runUpdateSeries(c, "0")
	c.Assert(err, gc.ErrorMatches, "no series specified")
	_, err = s.runUpdateSeries(c, "xenial")
	c.Assert(err, gc.ErrorMatches, "no application name or no series specified")
}

func (s *updateSeriesSuite) TestTooManyArguments(c *gc.C) {
	_, err := s.runUpdateSeries(c, "ghost", "xenial", "something else")
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["something else"\]`, gc.Commentf("details: %s", errors.Details(err)))
}

func (s *updateSeriesSuite) TestUpdateMachineSeriesUnsupported(c *gc.C) {
	_, err := s.runUpdateSeries(c, "0", "xenial")
	c.Assert(err, jc.Satisfies, errors.IsNotSupported)
}

type mockUpdateSeriesAPI struct {
	*testing.Stub
}

func (a *mockUpdateSeriesAPI) Close() error {
	a.MethodCall(a, "Close")
	return a.NextErr()
}

func (a *mockUpdateSeriesAPI) BestAPIVersion() int {
	return 5
}

func (a *mockUpdateSeriesAPI) UpdateApplicationSeries(appName, series string, force bool) error {
	a.MethodCall(a, "UpdateApplicationSeries", appName, series, force)
	return a.NextErr()
}
