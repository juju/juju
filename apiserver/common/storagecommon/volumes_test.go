// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storagecommon_test

import (
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common/storagecommon"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
)

type volumesSuite struct{}

var _ = gc.Suite(&volumesSuite{})

func (s *volumesSuite) TestVolumeParams(c *gc.C) {
	s.testVolumeParams(c, &state.VolumeParams{
		Pool: "loop",
		Size: 1024,
	}, nil)
}

func (s *volumesSuite) TestVolumeParamsAlreadyProvisioned(c *gc.C) {
	s.testVolumeParams(c, nil, &state.VolumeInfo{
		Pool: "loop",
		Size: 1024,
	})
}

func (*volumesSuite) testVolumeParams(c *gc.C, volumeParams *state.VolumeParams, info *state.VolumeInfo) {
	tag := names.NewVolumeTag("100")
	p, err := storagecommon.VolumeParams(
		&fakeVolume{tag: tag, params: volumeParams, info: info},
		nil, // StorageInstance
		testing.CustomModelConfig(c, testing.Attrs{
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
			tags.JujuModel: testing.ModelTag.Id(),
			"a":            "b",
			"c":            "",
		},
	})
}

func (*volumesSuite) TestVolumeParamsStorageTags(c *gc.C) {
	volumeTag := names.NewVolumeTag("100")
	storageTag := names.NewStorageTag("mystore/0")
	unitTag := names.NewUnitTag("mysql/123")
	p, err := storagecommon.VolumeParams(
		&fakeVolume{tag: volumeTag, params: &state.VolumeParams{
			Pool: "loop", Size: 1024,
		}},
		&fakeStorageInstance{tag: storageTag, owner: unitTag},
		testing.CustomModelConfig(c, nil),
		&fakePoolManager{},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(p, jc.DeepEquals, params.VolumeParams{
		VolumeTag: "volume-100",
		Provider:  "loop",
		Size:      1024,
		Tags: map[string]string{
			tags.JujuModel:           testing.ModelTag.Id(),
			tags.JujuStorageInstance: "mystore/0",
			tags.JujuStorageOwner:    "mysql/123",
		},
	})
}
