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
	mockApplicationAPI *mockUpdateApplicationSeriesAPI
	mockMachineAPI     *mockUpdateMachineSeriesAPI
}

var _ = gc.Suite(&updateSeriesSuite{})

func (s *updateSeriesSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.mockApplicationAPI = &mockUpdateApplicationSeriesAPI{Stub: &testing.Stub{}}
	s.mockMachineAPI = &mockUpdateMachineSeriesAPI{Stub: &testing.Stub{}}
}

func (s *updateSeriesSuite) runUpdateSeries(c *gc.C, args ...string) (*cmd.Context, error) {
	return cmdtesting.RunCommand(c, NewUpdateSeriesCommandForTest(s.mockApplicationAPI, s.mockMachineAPI), args...)
}

func (s *updateSeriesSuite) TestUpdateSeriesApplicationGoodPath(c *gc.C) {
	_, err := s.runUpdateSeries(c, "ghost", "xenial")
	c.Assert(err, jc.ErrorIsNil)
	s.mockApplicationAPI.CheckCall(c, 0, "UpdateApplicationSeries", "ghost", "xenial", false)
}

func (s *updateSeriesSuite) TestUpdateSeriesMachineGoodPath(c *gc.C) {
	_, err := s.runUpdateSeries(c, "0", "xenial")
	c.Assert(err, jc.ErrorIsNil)
	s.mockMachineAPI.CheckCall(c, 0, "UpdateMachineSeries", "0", "xenial", false)
}

func (s *updateSeriesSuite) TestUpdateSeriesMachineGoodPathForce(c *gc.C) {
	_, err := s.runUpdateSeries(c, "0", "xenial", "--force")
	c.Assert(err, jc.ErrorIsNil)
	s.mockMachineAPI.CheckCall(c, 0, "UpdateMachineSeries", "0", "xenial", true)
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

type mockUpdateApplicationSeriesAPI struct {
	*testing.Stub
}

func (a *mockUpdateApplicationSeriesAPI) Close() error {
	a.MethodCall(a, "Close")
	return a.NextErr()
}

func (a *mockUpdateApplicationSeriesAPI) BestAPIVersion() int {
	return 5
}

func (a *mockUpdateApplicationSeriesAPI) UpdateApplicationSeries(appName, series string, force bool) error {
	a.MethodCall(a, "UpdateApplicationSeries", appName, series, force)
	return a.NextErr()
}

type mockUpdateMachineSeriesAPI struct {
	*testing.Stub
}

func (a *mockUpdateMachineSeriesAPI) Close() error {
	a.MethodCall(a, "Close")
	return a.NextErr()
}

func (a *mockUpdateMachineSeriesAPI) BestAPIVersion() int {
	return 4
}

func (a *mockUpdateMachineSeriesAPI) UpdateMachineSeries(machName, series string, force bool) error {
	a.MethodCall(a, "UpdateMachineSeries", machName, series, force)
	return a.NextErr()
}
