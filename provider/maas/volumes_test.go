// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"github.com/juju/gomaasapi"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/testing"
)

type volumeSuite struct {
	providerSuite
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
	instance := maas2Instance{
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

func (s *volumeSuite) TestInstanceVolumes(c *gc.C) {
	obj := s.testMAASObject.TestServer.NewNode(validVolumeJson)
	statusGetter := func(context.ProviderCallContext, instance.Id) (string, string) {
		return "unknown", "FAKE"
	}

	instance := maas1Instance{&obj, nil, statusGetter}
	mTag := names.NewMachineTag("1")
	volumes, attachments, err := instance.volumes(mTag, []names.VolumeTag{
		names.NewVolumeTag("1"),
		names.NewVolumeTag("2"),
	})
	c.Assert(err, jc.ErrorIsNil)
	// Expect 2 volumes - root volume is ignored.
	c.Assert(volumes, gc.HasLen, 2)
	c.Assert(attachments, gc.HasLen, 2)
	c.Check(volumes, jc.DeepEquals, []storage.Volume{
		{
			// This volume has no id_path.
			names.NewVolumeTag("1"),
			storage.VolumeInfo{
				HardwareId: "",
				VolumeId:   "volume-1",
				Size:       476893,
				Persistent: false,
			},
		},
		{
			names.NewVolumeTag("2"),
			storage.VolumeInfo{
				HardwareId: "id_for_sdc",
				VolumeId:   "volume-2",
				Size:       238764,
				Persistent: false,
			},
		},
	})
	c.Assert(attachments, jc.DeepEquals, []storage.VolumeAttachment{
		{
			names.NewVolumeTag("1"),
			mTag,
			storage.VolumeAttachmentInfo{
				DeviceName: "sdb",
				ReadOnly:   false,
			},
		},
		// Device name not set because there's a hardware id in the volume.
		{
			names.NewVolumeTag("2"),
			mTag,
			storage.VolumeAttachmentInfo{
				DeviceName: "",
				ReadOnly:   false,
			},
		},
	})
}

func (s *volumeSuite) TestInstanceVolumesOldMass(c *gc.C) {
	obj := s.testMAASObject.TestServer.NewNode(`{"system_id": "node0"}`)
	statusGetter := func(context.ProviderCallContext, instance.Id) (string, string) {
		// status, substatus or status info.
		return "provisioning", "substatus"
	}

	instance := maas1Instance{&obj, nil, statusGetter}
	volumes, attachments, err := instance.volumes(names.NewMachineTag("1"), []names.VolumeTag{
		names.NewVolumeTag("1"),
		names.NewVolumeTag("2"),
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(volumes, gc.HasLen, 0)
	c.Assert(attachments, gc.HasLen, 0)
}

var validVolumeJson = `
{
    "system_id": "node0",
    "physicalblockdevice_set": [
        {
            "name": "sda",
            "tags": [
                "ssd",
                "sata"
            ],
            "id": 1,
            "id_path": "/dev/disk/by-id/id_for_sda",
            "path": "/dev/sda",
            "model": "Samsung_SSD_850_EVO_250GB",
            "block_size": 4096,
            "serial": "S21NNSAFC38075L",
            "size": 250059350016
        },
        {
            "name": "sdb",
            "tags": [
                "ssd",
                "sata"
            ],
            "id": 2,
            "path": "/dev/sdb",
            "model": "Samsung_SSD_850_EVO_500GB",
            "block_size": 4096,
            "serial": "S21NNSAFC38076L",
            "size": 500059350016
        },
        {
            "name": "sdb",
            "tags": [
                "ssd",
                "sata"
            ],
            "id": 3,
            "id_path": "/dev/disk/by-id/id_for_sdc",
            "path": "/dev/sdc",
            "model": "Samsung_SSD_850_EVO_250GB",
            "block_size": 4096,
            "serial": "S21NNSAFC38999L",
            "size": 250362438230
        },
        {
            "name": "sdd",
            "tags": [
                "ssd",
                "sata"
            ],
            "id": 4,
            "id_path": "/dev/disk/by-id/id_for_sdd",
            "path": "/dev/sdd",
            "model": "Samsung_SSD_850_EVO_250GB",
            "block_size": 4096,
            "serial": "S21NNSAFC386666L",
            "size": 250362438230
        },
        {
            "name": "sde",
            "tags": [
                "ssd",
                "sata"
            ],
            "id": 666,
            "id_path": "/dev/disk/by-id/id_for_sde",
            "path": "/dev/sde",
            "model": "Samsung_SSD_850_EVO_250GB",
            "block_size": 4096,
            "serial": "S21NNSAFC388888L",
			"size": 250362438230,
			"partitions": [
				{
					"id": 1,
					"name": "sde-part1",
					"path": "/dev/disk/by-dname/sde-part1",
					"size": 280362438231
				}
			]
		}
    ],
    "constraint_map": {
        "1": "root",
        "2": "1",
        "3": "2",
		"4": "3",
		"5": "partition:1"
    }
}
`[1:]

type storageProviderSuite struct {
	testing.BaseSuite
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
