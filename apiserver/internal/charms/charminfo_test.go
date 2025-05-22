// Copyright 2012-2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms_test

import (
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/authentication"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	"github.com/juju/juju/apiserver/internal/charms"
	"github.com/juju/juju/apiserver/internal/charms/mocks"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/domain/application/charm"
	internalcharm "github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/testhelpers"
	internaltesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type charmInfoSuite struct {
	testhelpers.IsolationSuite

	charmService *mocks.MockCharmService
	authorizer   *facademocks.MockAuthorizer
}

func TestCharmInfoSuite(t *stdtesting.T) {
	tc.Run(t, &charmInfoSuite{})
}

func (s *charmInfoSuite) TestCharmInfo(c *tc.C) {
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
	charmOrigin := charm.CharmLocator{Source: charm.CharmHubSource, Revision: 1}
	s.charmService.EXPECT().GetCharm(gomock.Any(), charm.CharmLocator{
		Name:     "foo",
		Revision: 1,
		Source:   charm.CharmHubSource,
	}).Return(charmBase, charmOrigin, true, nil)

	// Make the CharmInfo call
	api, err := charms.NewCharmInfoAPI(internaltesting.ModelTag, s.charmService, s.authorizer)
	c.Assert(err, tc.IsNil)
	charmInfo, err := api.CharmInfo(c.Context(), params.CharmURL{URL: "foo-1"})
	c.Assert(err, tc.IsNil)

	c.Check(charmInfo.URL, tc.Equals, "ch:amd64/foo-1")
	c.Check(charmInfo.Meta, tc.DeepEquals, &params.CharmMeta{Name: "foo", MinJujuVersion: "0.0.0"})
	c.Check(charmInfo.Manifest, tc.DeepEquals, &params.CharmManifest{Bases: []params.CharmBase{{Name: "ubuntu", Channel: "22.04/stable"}}})
	c.Check(charmInfo.Config, tc.DeepEquals, map[string]params.CharmOption{"foo": {Type: "string"}})
	c.Check(charmInfo.Actions, tc.DeepEquals, &params.CharmActions{ActionSpecs: map[string]params.CharmActionSpec{"bar": {Description: "baz"}}})
	c.Check(charmInfo.LXDProfile, tc.DeepEquals, &params.CharmLXDProfile{Config: map[string]string{"foo": "bar"}, Devices: map[string]map[string]string{}})
}

func (s *charmInfoSuite) TestCharmInfoMinimal(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.authorizer.EXPECT().AuthController().Return(true)

	metadata := &internalcharm.Meta{Name: "foo"}

	charmBase := internalcharm.NewCharmBase(metadata, nil, nil, nil, nil)
	charmOrigin := charm.CharmLocator{Source: charm.CharmHubSource, Revision: 1}
	s.charmService.EXPECT().GetCharm(gomock.Any(), charm.CharmLocator{
		Name:     "foo",
		Revision: 1,
		Source:   charm.CharmHubSource,
	}).Return(charmBase, charmOrigin, true, nil)

	// Make the CharmInfo call
	api, err := charms.NewCharmInfoAPI(internaltesting.ModelTag, s.charmService, s.authorizer)
	c.Assert(err, tc.IsNil)
	charmInfo, err := api.CharmInfo(c.Context(), params.CharmURL{URL: "foo-1"})
	c.Assert(err, tc.IsNil)

	c.Check(charmInfo.URL, tc.Equals, "ch:amd64/foo-1")
	c.Check(charmInfo.Meta, tc.DeepEquals, &params.CharmMeta{Name: "foo", MinJujuVersion: "0.0.0"})
	c.Check(charmInfo.Manifest, tc.IsNil)
	c.Check(charmInfo.Config, tc.IsNil)
	c.Check(charmInfo.Actions, tc.IsNil)
	c.Check(charmInfo.LXDProfile, tc.IsNil)
}

func (s *charmInfoSuite) TestPermissionDenied(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelTag := internaltesting.ModelTag

	s.authorizer.EXPECT().AuthController().Return(false)
	s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.ReadAccess, modelTag).
		Return(errors.WithType(apiservererrors.ErrPerm, authentication.ErrorEntityMissingPermission))

	// Make the CharmInfo call
	api, err := charms.NewCharmInfoAPI(modelTag, s.charmService, s.authorizer)
	c.Assert(err, tc.IsNil)
	_, err = api.CharmInfo(c.Context(), params.CharmURL{URL: "foo"})
	c.Assert(err, tc.ErrorMatches, "permission denied")
}

func (s *charmInfoSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.charmService = mocks.NewMockCharmService(ctrl)
	s.authorizer = facademocks.NewMockAuthorizer(ctrl)

	return ctrl
}
