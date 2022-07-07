// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"github.com/juju/gomaasapi/v2"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/storage"
)

type volumeSuite struct {
	maasSuite
}

var _ = gc.Suite(&volumeSuite{})

func (s *volumeSuite) TestBuildMAASVolumeParametersNoVolumes(c *gc.C) {
	vInfo, err := buildMAASVolumeParameters(nil, constraints.Value{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(vInfo, gc.HasLen, 0)
}

func (s *volumeSuite) TestBuildMAASVolumeParametersJustRootDisk(c *gc.C) {
	var cons constraints.Value
	rootSize := uint64(20000)
	cons.RootDisk = &rootSize
	vInfo, err := buildMAASVolumeParameters(nil, cons)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(vInfo, jc.DeepEquals, []volumeInfo{
		{"root", 20, nil},
	})
}

func (s *volumeSuite) TestBuildMAASVolumeParametersNoTags(c *gc.C) {
	vInfo, err := buildMAASVolumeParameters([]storage.VolumeParams{
		{Tag: names.NewVolumeTag("1"), Size: 2000000},
	}, constraints.Value{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(vInfo, jc.DeepEquals, []volumeInfo{
		{"root", 0, nil}, //root disk
		{"1", 1954, nil},
	})
}

func (s *volumeSuite) TestBuildMAASVolumeParametersWithRootDisk(c *gc.C) {
	var cons constraints.Value
	rootSize := uint64(20000)
	cons.RootDisk = &rootSize
	vInfo, err := buildMAASVolumeParameters([]storage.VolumeParams{
		{Tag: names.NewVolumeTag("1"), Size: 2000000},
	}, cons)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(vInfo, jc.DeepEquals, []volumeInfo{
		{"root", 20, nil}, //root disk
		{"1", 1954, nil},
	})
}

func (s *volumeSuite) TestBuildMAASVolumeParametersWithTags(c *gc.C) {
	vInfo, err := buildMAASVolumeParameters([]storage.VolumeParams{
		{Tag: names.NewVolumeTag("1"), Size: 2000000, Attributes: map[string]interface{}{"tags": "tag1,tag2"}},
	}, constraints.Value{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(vInfo, jc.DeepEquals, []volumeInfo{
		{"root", 0, nil}, //root disk
		{"1", 1954, []string{"tag1", "tag2"}},
	})
}

func (s *volumeSuite) TestInstanceVolumesMAAS2(c *gc.C) {
	instance := maasInstance{
		machine: &fakeMachine{},
		constraintMatches: gomaasapi.ConstraintMatches{
			Storage: map[string][]gomaasapi.StorageDevice{
				"root": {&fakeBlockDevice{name: "sda", idPath: "/dev/disk/by-dname/sda", size: 250059350016}},
				"1":    {&fakeBlockDevice{name: "sdb", idPath: "/dev/sdb", size: 500059350016}},
				"2":    {&fakeBlockDevice{name: "sdc", idPath: "/dev/disk/by-id/foo", size: 250362438230}},
				"3": {
					&fakeBlockDevice{name: "sdd", idPath: "/dev/disk/by-dname/sdd", size: 250362438230},
					&fakeBlockDevice{name: "sde", idPath: "/dev/disk/by-dname/sde", size: 250362438230},
				},
				"4": {
					&fakeBlockDevice{name: "sdf", idPath: "/dev/disk/by-id/wwn-drbr", size: 280362438231},
				},
				"5": {
					&fakePartition{name: "sde-part1", path: "/dev/disk/by-dname/sde-part1", size: 280362438231},
				},
				"6": {
					&fakeBlockDevice{name: "sdg", idPath: "/dev/disk/by-dname/sdg", size: 280362438231},
				},
			},
		},
	}
	mTag := names.NewMachineTag("1")
	volumes, attachments, err := instance.volumes(mTag, []names.VolumeTag{
		names.NewVolumeTag("1"),
		names.NewVolumeTag("2"),
		names.NewVolumeTag("3"),
		names.NewVolumeTag("4"),
		names.NewVolumeTag("5"),
	})
	c.Assert(err, jc.ErrorIsNil)
	// Expect 4 volumes - root volume is ignored, as are volumes
	// with tags we did not request.
	c.Assert(volumes, gc.HasLen, 5)
	c.Assert(attachments, gc.HasLen, 5)
	c.Check(volumes, jc.SameContents, []storage.Volume{{
		names.NewVolumeTag("1"),
		storage.VolumeInfo{
			VolumeId: "volume-1",
			Size:     476893,
		},
	}, {
		names.NewVolumeTag("2"),
		storage.VolumeInfo{
			VolumeId:   "volume-2",
			Size:       238764,
			HardwareId: "foo",
		},
	}, {
		names.NewVolumeTag("3"),
		storage.VolumeInfo{
			VolumeId: "volume-3",
			Size:     238764,
		},
	}, {
		names.NewVolumeTag("4"),
		storage.VolumeInfo{
			VolumeId: "volume-4",
			Size:     267374,
			WWN:      "drbr",
		},
	}, {
		names.NewVolumeTag("5"),
		storage.VolumeInfo{
			VolumeId: "volume-5",
			Size:     267374,
		},
	}})
	c.Assert(attachments, jc.SameContents, []storage.VolumeAttachment{{
		names.NewVolumeTag("1"),
		mTag,
		storage.VolumeAttachmentInfo{
			DeviceName: "sdb",
		},
	}, {
		names.NewVolumeTag("2"),
		mTag,
		storage.VolumeAttachmentInfo{},
	}, {
		names.NewVolumeTag("3"),
		mTag,
		storage.VolumeAttachmentInfo{
			DeviceLink: "/dev/disk/by-dname/sdd",
		},
	}, {
		names.NewVolumeTag("4"),
		mTag,
		storage.VolumeAttachmentInfo{},
	}, {
		names.NewVolumeTag("5"),
		mTag,
		storage.VolumeAttachmentInfo{
			DeviceLink: "/dev/disk/by-dname/sde-part1",
		},
	}})
}

type storageProviderSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&storageProviderSuite{})

func (*storageProviderSuite) TestValidateConfigTags(c *gc.C) {
	p := maasStorageProvider{}
	validate := func(tags interface{}) {
		cfg, err := storage.NewConfig("foo", maasStorageProviderType, map[string]interface{}{
			"tags": tags,
		})
		c.Assert(err, jc.ErrorIsNil)
		err = p.ValidateConfig(cfg)
		c.Assert(err, jc.ErrorIsNil)
	}
	validate("singular")
	validate("mul,ti,ple")
	validate(" leading, spaces")
	validate("trailing ,spaces ")
	validate(" and,everything, in ,  between ")
}

func (*storageProviderSuite) TestValidateConfigInvalidConfig(c *gc.C) {
	p := maasStorageProvider{}
	cfg, err := storage.NewConfig("foo", maasStorageProviderType, map[string]interface{}{
		"tags": "white space",
	})
	c.Assert(err, jc.ErrorIsNil)
	err = p.ValidateConfig(cfg)
	c.Assert(err, gc.ErrorMatches, `tags may not contain whitespace: "white space"`)
}

func (*storageProviderSuite) TestValidateConfigUnknownAttribute(c *gc.C) {
	p := maasStorageProvider{}
	cfg, err := storage.NewConfig("foo", maasStorageProviderType, map[string]interface{}{
		"unknown": "config",
	})
	c.Assert(err, jc.ErrorIsNil)
	err = p.ValidateConfig(cfg)
	c.Assert(err, jc.ErrorIsNil) // unknown attributes are ignored
}

func (s *storageProviderSuite) TestSupports(c *gc.C) {
	p := maasStorageProvider{}
	c.Assert(p.Supports(storage.StorageKindBlock), jc.IsTrue)
	c.Assert(p.Supports(storage.StorageKindFilesystem), jc.IsFalse)
}

func (s *storageProviderSuite) TestScope(c *gc.C) {
	p := maasStorageProvider{}
	c.Assert(p.Scope(), gc.Equals, storage.ScopeEnviron)
}
