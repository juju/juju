// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type UpdateSeriesSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&UpdateSeriesSuite{})

func (s *UpdateSeriesSuite) TestUpdateSeries(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	app := NewMockApplication(ctrl)
	app.EXPECT().IsPrincipal().Return(true)
	app.EXPECT().UpdateApplicationSeries("focal", false)

	state := NewMockUpdateSeriesState(ctrl)
	state.EXPECT().Application("foo").Return(app, nil)

	validator := NewMockUpdateSeriesValidator(ctrl)
	validator.EXPECT().ValidateApplication(app, "focal", false).Return(nil)

	api := NewUpdateSeriesAPI(state, validator)
	err := api.UpdateSeries("application-foo", "focal", false)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *UpdateSeriesSuite) TestUpdateSeriesNoSeries(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	api := NewUpdateSeriesAPI(nil, nil)
	err := api.UpdateSeries("application-foo", "", false)
	c.Assert(err, gc.ErrorMatches, `series missing from args`)
}

func (s *UpdateSeriesSuite) TestUpdateSeriesNotPrincipal(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	app := NewMockApplication(ctrl)
	app.EXPECT().IsPrincipal().Return(false)

	state := NewMockUpdateSeriesState(ctrl)
	state.EXPECT().Application("foo").Return(app, nil)

	validator := NewMockUpdateSeriesValidator(ctrl)

	api := NewUpdateSeriesAPI(state, validator)
	err := api.UpdateSeries("application-foo", "focal", false)
	c.Assert(err, gc.ErrorMatches, `"foo" is a subordinate application, update-series not supported`)
}

func (s *UpdateSeriesSuite) TestUpdateSeriesNotValid(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	app := NewMockApplication(ctrl)
	app.EXPECT().IsPrincipal().Return(true)

	state := NewMockUpdateSeriesState(ctrl)
	state.EXPECT().Application("foo").Return(app, nil)

	validator := NewMockUpdateSeriesValidator(ctrl)
	validator.EXPECT().ValidateApplication(app, "focal", false).Return(errors.New("bad"))

	api := NewUpdateSeriesAPI(state, validator)
	err := api.UpdateSeries("application-foo", "focal", false)
	c.Assert(err, gc.ErrorMatches, `bad`)
}
