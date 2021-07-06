// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/charm/v8"
	"github.com/juju/charm/v8/resource"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	basemocks "github.com/juju/juju/api/base/mocks"
	apicommoncharms "github.com/juju/juju/api/common/charms"
	"github.com/juju/juju/apiserver/params"
	coretesting "github.com/juju/juju/testing"
)

type charmsMockSuite struct {
	coretesting.BaseSuite
	charmsCommonClient *apicommoncharms.CharmsClient
}

var _ = gc.Suite(&charmsMockSuite{})

func (s *charmsMockSuite) TestCharmInfo(c *gc.C) {
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
				},
			},
			Storage: map[string]params.CharmStorage{
				"database": {
					Type: "filesystem",
				},
			},
			Assumes: []string{"kubernetes"},
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

	mockFacadeCaller.EXPECT().FacadeCall("CharmInfo", args, info).SetArg(2, params).Return(nil)

	client := apicommoncharms.NewCharmInfoClient(mockFacadeCaller)
	got, err := client.CharmInfo(url)
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
				},
			},
			Storage: map[string]charm.Storage{
				"database": {
					Type: "filesystem",
				},
			},
			Assumes: []string{"kubernetes"},
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
