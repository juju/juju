// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms_test

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/testing"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/common/charms"
	"github.com/juju/juju/apiserver/common/charms/mocks"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/domain/application/charm"
	internalcharm "github.com/juju/juju/internal/charm"
	internaltesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type appCharmInfoSuite struct {
	testing.IsolationSuite

	appService *mocks.MockApplicationService
	authorizer *facademocks.MockAuthorizer
}

var _ = gc.Suite(&appCharmInfoSuite{})

func (s *appCharmInfoSuite) TestApplicationCharmInfo(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.authorizer.EXPECT().AuthController().Return(true)

	metadata := &internalcharm.Meta{Name: "foo"}
	manifest := &internalcharm.Manifest{
		Bases: []internalcharm.Base{{Name: "ubuntu", Channel: internalcharm.Channel{Track: "22.04", Risk: "stable"}}},
	}
	config := &internalcharm.Config{
		Options: map[string]internalcharm.Option{"foo": {Type: "string"}},
	}
	actions := &internalcharm.Actions{
		ActionSpecs: map[string]internalcharm.ActionSpec{"bar": {Description: "baz"}},
	}
	lxdProfile := &internalcharm.LXDProfile{
		Config: map[string]string{"foo": "bar"},
	}

	charmBase := internalcharm.NewCharmBase(metadata, manifest, config, actions, lxdProfile)
	charmOrigin := charm.CharmOrigin{Source: charm.CharmHubSource, Revision: 1}

	s.appService.EXPECT().GetCharmByApplicationName(gomock.Any(), "fuu").Return(charmBase, charmOrigin, nil)

	// Make the ApplicationCharmInfo call
	api, err := charms.NewApplicationCharmInfoAPI(internaltesting.ModelTag, s.appService, s.authorizer)
	c.Assert(err, gc.IsNil)
	charmInfo, err := api.ApplicationCharmInfo(context.Background(), params.Entity{Tag: names.NewApplicationTag("fuu").String()})
	c.Assert(err, gc.IsNil)

	// The application name is used in the charm URL, the charm name is
	// only used as the fallback. This test ensures that the application
	// name is returned.

	c.Check(charmInfo.URL, gc.Equals, "ch:fuu-1")
	c.Check(charmInfo.Meta, gc.DeepEquals, &params.CharmMeta{Name: "foo", MinJujuVersion: "0.0.0"})
	c.Check(charmInfo.Manifest, gc.DeepEquals, &params.CharmManifest{Bases: []params.CharmBase{{Name: "ubuntu", Channel: "22.04/stable"}}})
	c.Check(charmInfo.Config, gc.DeepEquals, map[string]params.CharmOption{"foo": {Type: "string"}})
	c.Check(charmInfo.Actions, gc.DeepEquals, &params.CharmActions{ActionSpecs: map[string]params.CharmActionSpec{"bar": {Description: "baz"}}})
	c.Check(charmInfo.LXDProfile, gc.DeepEquals, &params.CharmLXDProfile{Config: map[string]string{"foo": "bar"}, Devices: map[string]map[string]string{}})
}

func (s *appCharmInfoSuite) TestApplicationCharmInfoMinimal(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.authorizer.EXPECT().AuthController().Return(true)

	metadata := &internalcharm.Meta{Name: "foo"}

	charmBase := internalcharm.NewCharmBase(metadata, nil, nil, nil, nil)
	charmOrigin := charm.CharmOrigin{Source: charm.CharmHubSource, Revision: 1}

	s.appService.EXPECT().GetCharmByApplicationName(gomock.Any(), "fuu").Return(charmBase, charmOrigin, nil)

	// Make the ApplicationCharmInfo call
	api, err := charms.NewApplicationCharmInfoAPI(internaltesting.ModelTag, s.appService, s.authorizer)
	c.Assert(err, gc.IsNil)
	charmInfo, err := api.ApplicationCharmInfo(context.Background(), params.Entity{Tag: names.NewApplicationTag("fuu").String()})
	c.Assert(err, gc.IsNil)

	c.Check(charmInfo.URL, gc.Equals, "ch:fuu-1")
	c.Check(charmInfo.Meta, gc.DeepEquals, &params.CharmMeta{Name: "foo", MinJujuVersion: "0.0.0"})
	c.Check(charmInfo.Manifest, gc.IsNil)
	c.Check(charmInfo.Config, gc.IsNil)
	c.Check(charmInfo.Actions, gc.IsNil)
	c.Check(charmInfo.LXDProfile, gc.IsNil)
}

func (s *appCharmInfoSuite) TestPermissionDenied(c *gc.C) {
	defer s.setupMocks(c).Finish()

	modelTag := internaltesting.ModelTag

	s.authorizer.EXPECT().AuthController().Return(false)
	s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.ReadAccess, modelTag).
		Return(errors.WithType(apiservererrors.ErrPerm, authentication.ErrorEntityMissingPermission))

	// Make the CharmInfo call
	api, err := charms.NewApplicationCharmInfoAPI(modelTag, s.appService, s.authorizer)
	c.Assert(err, gc.IsNil)
	_, err = api.ApplicationCharmInfo(context.Background(), params.Entity{Tag: names.NewApplicationTag("foo").String()})
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *appCharmInfoSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.appService = mocks.NewMockApplicationService(ctrl)
	s.authorizer = facademocks.NewMockAuthorizer(ctrl)

	return ctrl
}
