// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/state"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/poolmanager"
	"github.com/juju/juju/testing"
)

type volumesSuite struct{}

var _ = gc.Suite(&volumesSuite{})

type fakeVolume struct {
	state.Volume
	tag         names.Tag
	provisioned bool
}

func (v *fakeVolume) Tag() names.Tag {
	return v.tag
}

func (v *fakeVolume) Params() (state.VolumeParams, bool) {
	return state.VolumeParams{
		Pool: "loop",
		Size: 1024,
	}, !v.provisioned
}

func (*volumesSuite) TestVolumeParamsAlreadyProvisioned(c *gc.C) {
	tag := names.NewVolumeTag("100")
	_, err := common.VolumeParams(
		&fakeVolume{tag: tag, provisioned: true},
		nil, // StorageInstance
		testing.EnvironConfig(c),
		nil, // PoolManager
	)
	c.Assert(err, jc.Satisfies, common.IsVolumeAlreadyProvisioned)
}

type fakePoolManager struct {
	poolmanager.PoolManager
}

func (pm *fakePoolManager) Get(name string) (*storage.Config, error) {
	return nil, errors.NotFoundf("pool")
}

func (*volumesSuite) TestVolumeParams(c *gc.C) {
	tag := names.NewVolumeTag("100")
	p, err := common.VolumeParams(
		&fakeVolume{tag: tag},
		nil, // StorageInstance
		testing.CustomEnvironConfig(c, testing.Attrs{
			"resource-tags": "a=b c=",
		}),
		&fakePoolManager{},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(p, jc.DeepEquals, params.VolumeParams{
		VolumeTag: "volume-100",
		Provider:  "loop",
		Size:      1024,
		Tags: map[string]string{
			tags.JujuEnv: testing.EnvironmentTag.Id(),
			"a":          "b",
			"c":          "",
		},
	})
}

func (*volumesSuite) TestVolumeParamsStorageTags(c *gc.C) {
	volumeTag := names.NewVolumeTag("100")
	storageTag := names.NewStorageTag("mystore/0")
	unitTag := names.NewUnitTag("mysql/123")
	p, err := common.VolumeParams(
		&fakeVolume{tag: volumeTag},
		&fakeStorageInstance{tag: storageTag, owner: unitTag},
		testing.CustomEnvironConfig(c, nil),
		&fakePoolManager{},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(p, jc.DeepEquals, params.VolumeParams{
		VolumeTag: "volume-100",
		Provider:  "loop",
		Size:      1024,
		Tags: map[string]string{
			tags.JujuEnv:             testing.EnvironmentTag.Id(),
			tags.JujuStorageInstance: "mystore/0",
			tags.JujuStorageOwner:    "mysql/123",
		},
	})
}
