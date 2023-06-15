// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/charm/v11"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/charmhub/transport"
	coreseries "github.com/juju/juju/core/series"
	"github.com/juju/juju/state"
)

type UpdateBaseSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&UpdateBaseSuite{})

func (s *UpdateBaseSuite) TestUpdateBase(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	app := NewMockApplication(ctrl)
	app.EXPECT().IsPrincipal().Return(true)
	app.EXPECT().UpdateApplicationBase(state.UbuntuBase("20.04"), false)

	state := NewMockUpdateBaseState(ctrl)
	state.EXPECT().Application("foo").Return(app, nil)

	validator := NewMockUpdateBaseValidator(ctrl)
	coreBase := coreseries.MakeDefaultBase("ubuntu", "20.04")
	validator.EXPECT().ValidateApplication(app, coreBase, false).Return(nil)

	api := NewUpdateBaseAPI(state, validator)
	err := api.UpdateBase("application-foo", coreBase, false)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *UpdateBaseSuite) TestUpdateBaseNoSeries(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	api := NewUpdateBaseAPI(nil, nil)
	err := api.UpdateBase("application-foo", coreseries.Base{}, false)
	c.Assert(err, gc.ErrorMatches, `base missing from args`)
}

func (s *UpdateBaseSuite) TestUpdateBaseNotPrincipal(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	app := NewMockApplication(ctrl)
	app.EXPECT().IsPrincipal().Return(false)

	state := NewMockUpdateBaseState(ctrl)
	state.EXPECT().Application("foo").Return(app, nil)

	validator := NewMockUpdateBaseValidator(ctrl)

	api := NewUpdateBaseAPI(state, validator)
	err := api.UpdateBase("application-foo", coreseries.MakeDefaultBase("ubuntu", "20.04"), false)
	c.Assert(err, gc.ErrorMatches, `"foo" is a subordinate application, update-series not supported`)
}

func (s *UpdateBaseSuite) TestUpdateBaseNotValid(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	app := NewMockApplication(ctrl)
	app.EXPECT().IsPrincipal().Return(true)

	state := NewMockUpdateBaseState(ctrl)
	state.EXPECT().Application("foo").Return(app, nil)

	validator := NewMockUpdateBaseValidator(ctrl)
	validator.EXPECT().ValidateApplication(app, coreseries.MakeDefaultBase("ubuntu", "20.04"), false).Return(errors.New("bad"))

	api := NewUpdateBaseAPI(state, validator)
	err := api.UpdateBase("application-foo", coreseries.MakeDefaultBase("ubuntu", "20.04"), false)
	c.Assert(err, gc.ErrorMatches, `bad`)
}

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
	}).MinTimes(2)
	ch.EXPECT().Manifest().Return(nil).AnyTimes()

	application := NewMockApplication(ctrl)
	application.EXPECT().Charm().Return(ch, false, nil)

	validator := stateSeriesValidator{}
	err := validator.ValidateApplication(application, coreseries.MakeDefaultBase("ubuntu", "20.04"), false)
	c.Assert(err, jc.ErrorIsNil)
}

func (s StateValidatorSuite) TestValidateApplicationWithFallbackSeries(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	url := charm.MustParseURL("ch:focal/foo-1")

	ch := NewMockCharm(ctrl)
	ch.EXPECT().Meta().Return(&charm.Meta{}).MinTimes(2)
	ch.EXPECT().Manifest().Return(nil).AnyTimes()
	ch.EXPECT().URL().Return(url)

	application := NewMockApplication(ctrl)
	application.EXPECT().Charm().Return(ch, false, nil)

	validator := stateSeriesValidator{}
	err := validator.ValidateApplication(application, coreseries.MakeDefaultBase("ubuntu", "20.04"), false)
	c.Assert(err, jc.ErrorIsNil)
}

func (s StateValidatorSuite) TestValidateApplicationWithUnsupportedSeries(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ch := NewMockCharm(ctrl)
	ch.EXPECT().Meta().Return(&charm.Meta{
		Series: []string{"xenial", "bionic"},
	}).MinTimes(2)
	ch.EXPECT().Manifest().Return(nil).AnyTimes()
	ch.EXPECT().String().Return("ch:foo-1")

	application := NewMockApplication(ctrl)
	application.EXPECT().Charm().Return(ch, false, nil)

	validator := stateSeriesValidator{}
	err := validator.ValidateApplication(application, coreseries.MakeDefaultBase("ubuntu", "20.04"), false)
	c.Assert(err, gc.ErrorMatches, `series "focal" not supported by charm "ch:foo-1", supported series are: xenial, bionic`)
}

func (s StateValidatorSuite) TestValidateApplicationWithUnsupportedSeriesWithForce(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ch := NewMockCharm(ctrl)
	ch.EXPECT().Meta().Return(&charm.Meta{
		Series: []string{"xenial", "bionic"},
	}).MinTimes(2)
	ch.EXPECT().Manifest().Return(nil).AnyTimes()

	application := NewMockApplication(ctrl)
	application.EXPECT().Charm().Return(ch, false, nil)

	validator := stateSeriesValidator{}
	err := validator.ValidateApplication(application, coreseries.MakeDefaultBase("ubuntu", "20.04"), true)
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
		{Entity: transport.RefreshEntity{
			Bases: []transport.Base{{Channel: "18.04"}, {Channel: "20.04"}},
		}},
	}, nil)

	revision := 1

	application := NewMockApplication(ctrl)
	application.EXPECT().CharmOrigin().Return(&state.CharmOrigin{
		ID:       "mycharmhubid",
		Revision: &revision,
		Platform: &state.Platform{
			Architecture: "amd64",
			OS:           "ubuntu",
			Channel:      "18.04/stable",
		},
	})

	validator := charmhubSeriesValidator{
		client: client,
	}
	err := validator.ValidateApplication(application, coreseries.MakeDefaultBase("ubuntu", "20.04"), false)
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
	err := validator.ValidateApplication(application, coreseries.MakeDefaultBase("ubuntu", "20.04"), false)
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
		ID:       "mycharmhubid",
		Revision: &revision,
		Platform: &state.Platform{
			Architecture: "amd64",
			OS:           "ubuntu",
			Channel:      "18.04/stable",
		},
	})

	validator := charmhubSeriesValidator{
		client: client,
	}
	err := validator.ValidateApplication(application, coreseries.MakeDefaultBase("ubuntu", "20.04"), false)
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
		ID:       "mycharmhubid",
		Revision: &revision,
		Platform: &state.Platform{
			Architecture: "amd64",
			OS:           "ubuntu",
			Channel:      "18.04/stable",
		},
	})

	validator := charmhubSeriesValidator{
		client: client,
	}
	err := validator.ValidateApplication(application, coreseries.MakeDefaultBase("ubuntu", "20.04"), false)
	c.Assert(err, gc.ErrorMatches, `unable to locate application with base ubuntu@20.04: bad`)
}

func (s CharmhubValidatorSuite) TestValidateApplicationWithRefreshErrorAndForce(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	client := NewMockCharmhubClient(ctrl)
	client.EXPECT().Refresh(gomock.Any(), gomock.Any()).Return([]transport.RefreshResponse{{
		Entity: transport.RefreshEntity{
			Bases: []transport.Base{{Channel: "18.04"}, {Channel: "20.04"}},
		},
		Error: &transport.APIError{
			Message: "bad",
		}},
	}, nil)

	revision := 1

	application := NewMockApplication(ctrl)
	application.EXPECT().CharmOrigin().Return(&state.CharmOrigin{
		ID:       "mycharmhubid",
		Revision: &revision,
		Platform: &state.Platform{
			Architecture: "amd64",
			OS:           "ubuntu",
			Channel:      "18.04/stable",
		},
	})

	validator := charmhubSeriesValidator{
		client: client,
	}
	err := validator.ValidateApplication(application, coreseries.MakeDefaultBase("ubuntu", "20.04"), true)
	c.Assert(err, jc.ErrorIsNil)
}
