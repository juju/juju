// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms_test

import (
	"context"

	"github.com/juju/charm/v11"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/common/charms"
	"github.com/juju/juju/apiserver/common/charms/mocks"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type appCharmInfoSuite struct{}

var _ = gc.Suite(&appCharmInfoSuite{})

func (s *appCharmInfoSuite) TestBasic(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	st := mocks.NewMockState(ctrl)
	model := mocks.NewMockModel(ctrl)
	st.EXPECT().Model().Return(model, nil)
	app := mocks.NewMockApplication(ctrl)
	st.EXPECT().Application("foo").Return(app, nil)

	ch := mocks.NewMockCharm(ctrl)
	app.EXPECT().Charm().Return(ch, false, nil)

	// The convertCharm logic is tested in the CharmInfo tests, so just test
	// the minimal set of fields here.
	ch.EXPECT().String().Return("ch:foo-1")
	ch.EXPECT().Revision().Return(1)
	ch.EXPECT().Config().Return(&charm.Config{})
	ch.EXPECT().Meta().Return(&charm.Meta{Name: "foo"})
	ch.EXPECT().Actions().Return(&charm.Actions{})
	ch.EXPECT().Metrics().Return(&charm.Metrics{})
	ch.EXPECT().Manifest().Return(&charm.Manifest{})
	ch.EXPECT().LXDProfile().Return(&state.LXDProfile{})

	authorizer := facademocks.NewMockAuthorizer(ctrl)
	authorizer.EXPECT().AuthController().Return(true)

	// Make the ApplicationCharmInfo call
	api, err := charms.NewApplicationCharmInfoAPI(st, authorizer)
	c.Assert(err, gc.IsNil)
	charmInfo, err := api.ApplicationCharmInfo(context.Background(), params.Entity{Tag: names.NewApplicationTag("foo").String()})
	c.Assert(err, gc.IsNil)

	c.Check(charmInfo.URL, gc.Equals, "ch:foo-1")
	c.Check(charmInfo.Meta.Name, gc.Equals, "foo")
}

func (s *appCharmInfoSuite) TestPermissionDenied(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	st := mocks.NewMockState(ctrl)
	model := mocks.NewMockModel(ctrl)
	st.EXPECT().Model().Return(model, nil)

	modelTag := names.NewModelTag("1")
	model.EXPECT().ModelTag().Return(modelTag)

	authorizer := facademocks.NewMockAuthorizer(ctrl)
	authorizer.EXPECT().AuthController().Return(false)
	authorizer.EXPECT().HasPermission(permission.ReadAccess, modelTag).
		Return(errors.WithType(apiservererrors.ErrPerm, authentication.ErrorEntityMissingPermission))

	// Make the ApplicationCharmInfo call
	api, err := charms.NewApplicationCharmInfoAPI(st, authorizer)
	c.Assert(err, gc.IsNil)
	_, err = api.ApplicationCharmInfo(context.Background(), params.Entity{Tag: names.NewApplicationTag("foo").String()})
	c.Assert(err, gc.ErrorMatches, "permission denied")
}
