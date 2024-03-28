// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms_test

import (
	"github.com/juju/charm/v12"
	"github.com/juju/charm/v12/resource"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
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
	ch.EXPECT().URL().Return("ch:foo-1")
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
	charmInfo, err := api.ApplicationCharmInfo(params.Entity{Tag: names.NewApplicationTag("foo").String()})
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
	_, err = api.ApplicationCharmInfo(params.Entity{Tag: names.NewApplicationTag("foo").String()})
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *appCharmInfoSuite) TestSidecarCharm(c *gc.C) {
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
	ch.EXPECT().URL().Return("ch:foo-1")
	ch.EXPECT().Revision().Return(1)
	ch.EXPECT().Config().Return(&charm.Config{})
	ch.EXPECT().Meta().Return(&charm.Meta{
		Name:      "foo",
		CharmUser: charm.RunAsRoot,
		Containers: map[string]charm.Container{
			"my-container": {
				Resource: "my-image",
				Mounts: []charm.Mount{{
					Storage:  "my-storage",
					Location: "/my/storage/location",
				}},
				Uid: 5000,
				Gid: 5001,
			},
		},
		Storage: map[string]charm.Storage{
			"my-storage": {
				Name: "my-storage",
				Type: charm.StorageFilesystem,
			},
		},
		Resources: map[string]resource.Meta{
			"my-image": {
				Name:        "my-image",
				Type:        resource.TypeContainerImage,
				Description: "my container image",
			},
		},
	})
	ch.EXPECT().Actions().Return(&charm.Actions{})
	ch.EXPECT().Metrics().Return(&charm.Metrics{})
	ch.EXPECT().Manifest().Return(&charm.Manifest{})
	ch.EXPECT().LXDProfile().Return(&state.LXDProfile{})

	authorizer := facademocks.NewMockAuthorizer(ctrl)
	authorizer.EXPECT().AuthController().Return(true)

	// Make the ApplicationCharmInfo call
	api, err := charms.NewApplicationCharmInfoAPI(st, authorizer)
	c.Assert(err, gc.IsNil)
	charmInfo, err := api.ApplicationCharmInfo(params.Entity{Tag: names.NewApplicationTag("foo").String()})
	c.Assert(err, gc.IsNil)

	c.Check(charmInfo, gc.DeepEquals, params.Charm{
		URL:      "ch:foo-1",
		Revision: 1,
		Meta: &params.CharmMeta{
			Name: "foo",
			Storage: map[string]params.CharmStorage{
				"my-storage": {
					Name: "my-storage",
					Type: "filesystem",
				},
			},
			Containers: map[string]params.CharmContainer{
				"my-container": {
					Resource: "my-image",
					Mounts: []params.CharmMount{{
						Storage:  "my-storage",
						Location: "/my/storage/location",
					}},
					Uid: 5000,
					Gid: 5001,
				},
			},
			Resources: map[string]params.CharmResourceMeta{
				"my-image": {
					Name:        "my-image",
					Type:        "oci-image",
					Description: "my container image",
				},
			},
			MinJujuVersion: "0.0.0",
			CharmUser:      "root",
		},
		Config:   map[string]params.CharmOption{},
		Actions:  &params.CharmActions{},
		Metrics:  &params.CharmMetrics{},
		Manifest: &params.CharmManifest{},
	})
}
