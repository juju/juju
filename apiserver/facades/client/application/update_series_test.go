// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/charm/v8"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/charmhub/transport"
	"github.com/juju/juju/state"
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

//////////////////////

type StateValidatorSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&StateValidatorSuite{})

func (s StateValidatorSuite) TestValidateApplication(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ch := NewMockCharm(ctrl)
	ch.EXPECT().Meta().Return(&charm.Meta{
		Series: []string{"focal", "bionic"},
	})

	application := NewMockApplication(ctrl)
	application.EXPECT().Charm().Return(ch, false, nil)

	validator := stateSeriesValidator{}
	err := validator.ValidateApplication(application, "focal", false)
	c.Assert(err, jc.ErrorIsNil)
}

func (s StateValidatorSuite) TestValidateApplicationWithFallbackSeries(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	url := charm.MustParseURL("cs:focal/foo-1")

	ch := NewMockCharm(ctrl)
	ch.EXPECT().Meta().Return(&charm.Meta{})
	ch.EXPECT().URL().Return(url)

	application := NewMockApplication(ctrl)
	application.EXPECT().Charm().Return(ch, false, nil)

	validator := stateSeriesValidator{}
	err := validator.ValidateApplication(application, "focal", false)
	c.Assert(err, jc.ErrorIsNil)
}

func (s StateValidatorSuite) TestValidateApplicationWithUnsupportedSeries(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ch := NewMockCharm(ctrl)
	ch.EXPECT().Meta().Return(&charm.Meta{
		Series: []string{"xenial", "bionic"},
	})
	ch.EXPECT().String().Return("cs:foo-1")

	application := NewMockApplication(ctrl)
	application.EXPECT().Charm().Return(ch, false, nil)

	validator := stateSeriesValidator{}
	err := validator.ValidateApplication(application, "focal", false)
	c.Assert(err, gc.ErrorMatches, `series "focal" not supported by charm "cs:foo-1", supported series are: xenial, bionic`)
}

func (s StateValidatorSuite) TestValidateApplicationWithUnsupportedSeriesWithForce(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ch := NewMockCharm(ctrl)
	ch.EXPECT().Meta().Return(&charm.Meta{
		Series: []string{"xenial", "bionic"},
	})

	application := NewMockApplication(ctrl)
	application.EXPECT().Charm().Return(ch, false, nil)

	validator := stateSeriesValidator{}
	err := validator.ValidateApplication(application, "focal", true)
	c.Assert(err, jc.ErrorIsNil)
}

type CharmhubValidatorSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&CharmhubValidatorSuite{})

func (s CharmhubValidatorSuite) TestValidateApplication(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	client := NewMockCharmhubClient(ctrl)
	client.EXPECT().Refresh(gomock.Any(), gomock.Any()).Return([]transport.RefreshResponse{
		{},
	}, nil)

	revision := 1

	application := NewMockApplication(ctrl)
	application.EXPECT().CharmOrigin().Return(&state.CharmOrigin{
		Revision: &revision,
		Platform: &state.Platform{
			Architecture: "amd64",
			OS:           "ubuntu",
			Series:       "bionic",
		},
	})

	validator := charmhubSeriesValidator{
		client: client,
	}
	err := validator.ValidateApplication(application, "focal", false)
	c.Assert(err, jc.ErrorIsNil)
}

func (s CharmhubValidatorSuite) TestValidateApplicationWithNoRevision(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	client := NewMockCharmhubClient(ctrl)

	application := NewMockApplication(ctrl)
	application.EXPECT().CharmOrigin().Return(&state.CharmOrigin{})
	application.EXPECT().Name().Return("foo")

	validator := charmhubSeriesValidator{
		client: client,
	}
	err := validator.ValidateApplication(application, "focal", false)
	c.Assert(err, gc.ErrorMatches, `no revision found for application "foo"`)
}

func (s CharmhubValidatorSuite) TestValidateApplicationWithClientRefreshError(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	client := NewMockCharmhubClient(ctrl)
	client.EXPECT().Refresh(gomock.Any(), gomock.Any()).Return([]transport.RefreshResponse{
		{},
	}, errors.Errorf("bad"))

	revision := 1

	application := NewMockApplication(ctrl)
	application.EXPECT().CharmOrigin().Return(&state.CharmOrigin{
		Revision: &revision,
		Platform: &state.Platform{
			Architecture: "amd64",
			OS:           "ubuntu",
			Series:       "bionic",
		},
	})

	validator := charmhubSeriesValidator{
		client: client,
	}
	err := validator.ValidateApplication(application, "focal", false)
	c.Assert(err, gc.ErrorMatches, `bad`)
}

func (s CharmhubValidatorSuite) TestValidateApplicationWithRefreshError(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	client := NewMockCharmhubClient(ctrl)
	client.EXPECT().Refresh(gomock.Any(), gomock.Any()).Return([]transport.RefreshResponse{
		{Error: &transport.APIError{
			Message: "bad",
		}},
	}, nil)

	revision := 1

	application := NewMockApplication(ctrl)
	application.EXPECT().CharmOrigin().Return(&state.CharmOrigin{
		Revision: &revision,
		Platform: &state.Platform{
			Architecture: "amd64",
			OS:           "ubuntu",
			Series:       "bionic",
		},
	})

	validator := charmhubSeriesValidator{
		client: client,
	}
	err := validator.ValidateApplication(application, "focal", false)
	c.Assert(err, gc.ErrorMatches, `unable to locate application with series focal: bad`)
}

func (s CharmhubValidatorSuite) TestValidateApplicationWithRefreshErrorAndForce(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	client := NewMockCharmhubClient(ctrl)
	client.EXPECT().Refresh(gomock.Any(), gomock.Any()).Return([]transport.RefreshResponse{
		{Error: &transport.APIError{
			Message: "bad",
		}},
	}, nil)

	revision := 1

	application := NewMockApplication(ctrl)
	application.EXPECT().CharmOrigin().Return(&state.CharmOrigin{
		Revision: &revision,
		Platform: &state.Platform{
			Architecture: "amd64",
			OS:           "ubuntu",
			Series:       "bionic",
		},
	})

	validator := charmhubSeriesValidator{
		client: client,
	}
	err := validator.ValidateApplication(application, "focal", true)
	c.Assert(err, jc.ErrorIsNil)
}
