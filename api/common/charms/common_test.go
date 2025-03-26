// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms_test

import (
	"context"

	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	basemocks "github.com/juju/juju/api/base/mocks"
	apicommoncharms "github.com/juju/juju/api/common/charms"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/charm/resource"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/version"
	"github.com/juju/juju/rpc/params"
)

type suite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&suite{})

func (s *suite) TestCharmInfo(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)

	url := "local:quantal/dummy-1"
	args := params.CharmURL{URL: url}
	info := new(params.Charm)

	params := params.Charm{
		Revision: 1,
		URL:      url,
		Config: map[string]params.CharmOption{
			"config": {
				Type:        "type",
				Description: "config-type option",
			},
		},
		LXDProfile: &params.CharmLXDProfile{
			Description: "LXDProfile",
			Devices: map[string]map[string]string{
				"tun": {
					"path": "/dev/net/tun",
					"type": "unix-char",
				},
			},
		},
		Meta: &params.CharmMeta{
			Name:           "dummy",
			Description:    "cockroachdb",
			MinJujuVersion: "2.9.0",
			Resources: map[string]params.CharmResourceMeta{
				"cockroachdb-image": {
					Type:        "oci-image",
					Description: "OCI image used for cockroachdb",
				},
			},
			Containers: map[string]params.CharmContainer{
				"cockroachdb": {
					Resource: "cockroachdb-image",
					Mounts: []params.CharmMount{
						{
							Storage:  "database",
							Location: "/cockroach/cockroach-data",
						},
					},
					Uid: intPtr(5000),
					Gid: intPtr(5001),
				},
			},
			Storage: map[string]params.CharmStorage{
				"database": {
					Type: "filesystem",
				},
			},
			CharmUser: "root",
		},
		Manifest: &params.CharmManifest{
			Bases: []params.CharmBase{
				{
					Name:    "ubuntu",
					Channel: "20.04/stable",
				},
			},
		},
	}

	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "CharmInfo", args, info).SetArg(3, params).Return(nil)

	client := apicommoncharms.NewCharmInfoClient(mockFacadeCaller)
	got, err := client.CharmInfo(context.Background(), url)
	c.Assert(err, gc.IsNil)

	want := &apicommoncharms.CharmInfo{
		Revision: 1,
		URL:      url,
		Config: &charm.Config{
			Options: map[string]charm.Option{
				"config": {
					Type:        "type",
					Description: "config-type option",
				},
			},
		},
		LXDProfile: &charm.LXDProfile{
			Description: "LXDProfile",
			Config:      map[string]string{},
			Devices: map[string]map[string]string{
				"tun": {
					"path": "/dev/net/tun",
					"type": "unix-char",
				},
			},
		},
		Meta: &charm.Meta{
			Name:           "dummy",
			Description:    "cockroachdb",
			MinJujuVersion: version.MustParse("2.9.0"),
			Resources: map[string]resource.Meta{
				"cockroachdb-image": {
					Type:        resource.TypeContainerImage,
					Description: "OCI image used for cockroachdb",
				},
			},
			Containers: map[string]charm.Container{
				"cockroachdb": {
					Resource: "cockroachdb-image",
					Mounts: []charm.Mount{
						{
							Storage:  "database",
							Location: "/cockroach/cockroach-data",
						},
					},
					Uid: intPtr(5000),
					Gid: intPtr(5001),
				},
			},
			Storage: map[string]charm.Storage{
				"database": {
					Type: "filesystem",
				},
			},
			CharmUser: charm.RunAsRoot,
		},
		Manifest: &charm.Manifest{
			Bases: []charm.Base{
				{
					Name: "ubuntu",
					Channel: charm.Channel{
						Risk:  "stable",
						Track: "20.04",
					},
					Architectures: []string{},
				},
			},
		},
	}
	c.Assert(got, gc.DeepEquals, want)
}

func (s *suite) TestApplicationCharmInfo(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)

	args := params.Entity{Tag: "application-foobar"}
	info := new(params.Charm)

	params := params.Charm{
		Revision: 1,
		URL:      "ch:foobar",
		Meta: &params.CharmMeta{
			Name:           "foobar",
			MinJujuVersion: "2.9.0",
		},
		// The rest of the field conversions are tested by TestCharmInfo
	}

	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "ApplicationCharmInfo", args, info).SetArg(3, params).Return(nil)

	client := apicommoncharms.NewApplicationCharmInfoClient(mockFacadeCaller)
	got, err := client.ApplicationCharmInfo(context.Background(), "foobar")
	c.Assert(err, gc.IsNil)

	want := &apicommoncharms.CharmInfo{
		Revision: 1,
		URL:      "ch:foobar",
		Meta: &charm.Meta{
			Name:           "foobar",
			MinJujuVersion: version.MustParse("2.9.0"),
		},
	}
	c.Assert(got, gc.DeepEquals, want)
}

func intPtr(i int) *int {
	return &i
}
