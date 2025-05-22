// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms_test

import (
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/authentication"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	"github.com/juju/juju/apiserver/internal/charms"
	"github.com/juju/juju/apiserver/internal/charms/mocks"
	applicationtesting "github.com/juju/juju/core/application/testing"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/domain/application/architecture"
	"github.com/juju/juju/domain/application/charm"
	internalcharm "github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/testhelpers"
	internaltesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type appCharmInfoSuite struct {
	testhelpers.IsolationSuite

	appService *mocks.MockApplicationService
	authorizer *facademocks.MockAuthorizer
}

func TestAppCharmInfoSuite(t *stdtesting.T) {
	tc.Run(t, &appCharmInfoSuite{})
}

func (s *appCharmInfoSuite) TestApplicationCharmInfo(c *tc.C) {
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
	locator := charm.CharmLocator{Source: charm.CharmHubSource, Revision: 1, Architecture: architecture.AMD64}

	id := applicationtesting.GenApplicationUUID(c)

	s.appService.EXPECT().GetApplicationIDByName(gomock.Any(), "fuu").Return(id, nil)
	s.appService.EXPECT().GetCharmByApplicationID(gomock.Any(), id).Return(charmBase, locator, nil)

	// Make the ApplicationCharmInfo call
	api, err := charms.NewApplicationCharmInfoAPI(internaltesting.ModelTag, s.appService, s.authorizer)
	c.Assert(err, tc.IsNil)
	charmInfo, err := api.ApplicationCharmInfo(c.Context(), params.Entity{Tag: names.NewApplicationTag("fuu").String()})
	c.Assert(err, tc.IsNil)

	// The application name is used in the charm URL, the charm name is
	// only used as the fallback. This test ensures that the application
	// name is returned.

	c.Check(charmInfo.URL, tc.Equals, "ch:amd64/fuu-1")
	c.Check(charmInfo.Meta, tc.DeepEquals, &params.CharmMeta{Name: "foo", MinJujuVersion: "0.0.0"})
	c.Check(charmInfo.Manifest, tc.DeepEquals, &params.CharmManifest{Bases: []params.CharmBase{{Name: "ubuntu", Channel: "22.04/stable"}}})
	c.Check(charmInfo.Config, tc.DeepEquals, map[string]params.CharmOption{"foo": {Type: "string"}})
	c.Check(charmInfo.Actions, tc.DeepEquals, &params.CharmActions{ActionSpecs: map[string]params.CharmActionSpec{"bar": {Description: "baz"}}})
	c.Check(charmInfo.LXDProfile, tc.DeepEquals, &params.CharmLXDProfile{Config: map[string]string{"foo": "bar"}, Devices: map[string]map[string]string{}})
}

func (s *appCharmInfoSuite) TestApplicationCharmInfoMinimal(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.authorizer.EXPECT().AuthController().Return(true)

	metadata := &internalcharm.Meta{Name: "foo"}

	charmBase := internalcharm.NewCharmBase(metadata, nil, nil, nil, nil)
	locator := charm.CharmLocator{Source: charm.CharmHubSource, Revision: 1, Architecture: architecture.AMD64}

	id := applicationtesting.GenApplicationUUID(c)

	s.appService.EXPECT().GetApplicationIDByName(gomock.Any(), "fuu").Return(id, nil)
	s.appService.EXPECT().GetCharmByApplicationID(gomock.Any(), id).Return(charmBase, locator, nil)

	// Make the ApplicationCharmInfo call
	api, err := charms.NewApplicationCharmInfoAPI(internaltesting.ModelTag, s.appService, s.authorizer)
	c.Assert(err, tc.IsNil)
	charmInfo, err := api.ApplicationCharmInfo(c.Context(), params.Entity{Tag: names.NewApplicationTag("fuu").String()})
	c.Assert(err, tc.IsNil)

	c.Check(charmInfo.URL, tc.Equals, "ch:amd64/fuu-1")
	c.Check(charmInfo.Meta, tc.DeepEquals, &params.CharmMeta{Name: "foo", MinJujuVersion: "0.0.0"})
	c.Check(charmInfo.Manifest, tc.IsNil)
	c.Check(charmInfo.Config, tc.IsNil)
	c.Check(charmInfo.Actions, tc.IsNil)
	c.Check(charmInfo.LXDProfile, tc.IsNil)
}

func (s *appCharmInfoSuite) TestPermissionDenied(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelTag := internaltesting.ModelTag

	s.authorizer.EXPECT().AuthController().Return(false)
	s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.ReadAccess, modelTag).
		Return(errors.WithType(apiservererrors.ErrPerm, authentication.ErrorEntityMissingPermission))

	// Make the CharmInfo call
	api, err := charms.NewApplicationCharmInfoAPI(modelTag, s.appService, s.authorizer)
	c.Assert(err, tc.IsNil)
	_, err = api.ApplicationCharmInfo(c.Context(), params.Entity{Tag: names.NewApplicationTag("foo").String()})
	c.Assert(err, tc.ErrorMatches, "permission denied")
}

func (s *appCharmInfoSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.appService = mocks.NewMockApplicationService(ctrl)
	s.authorizer = facademocks.NewMockAuthorizer(ctrl)

	return ctrl
}
