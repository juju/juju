// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/storage"
	"github.com/juju/juju/testing"
)

type volumeSuite struct {
	providerSuite
}

var _ = gc.Suite(&volumeSuite{})

func (s *volumeSuite) TestBuildMAASVolumeParametersNoVolumes(c *gc.C) {
	vInfo, err := buildMAASVolumeParameters(nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(vInfo, gc.HasLen, 0)
}

func (s *volumeSuite) TestBuildMAASVolumeParametersNoTags(c *gc.C) {
	vInfo, err := buildMAASVolumeParameters([]storage.VolumeParams{
		{Tag: names.NewVolumeTag("1"), Size: 2000000},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(vInfo, jc.DeepEquals, []volumeInfo{
		{sizeInGB: 0}, //root disk
		{"volume-1", 1954, nil},
	})
}

func (s *volumeSuite) TestBuildMAASVolumeParametersWithTags(c *gc.C) {
	vInfo, err := buildMAASVolumeParameters([]storage.VolumeParams{
		{Tag: names.NewVolumeTag("1"), Size: 2000000, Attributes: map[string]interface{}{"tags": "tag1,tag2"}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(vInfo, jc.DeepEquals, []volumeInfo{
		{sizeInGB: 0}, //root disk
		{"volume-1", 1954, []string{"tag1", "tag2"}},
	})
}

func (s *volumeSuite) TestInstanceVolumes(c *gc.C) {
	obj := s.testMAASObject.TestServer.NewNode(validVolumeJson)
	instance := maasInstance{maasObject: &obj, environ: s.makeEnviron()}
	mTag := names.NewMachineTag("1")
	volumes, attachments, err := instance.volumes(mTag)
	c.Assert(err, jc.ErrorIsNil)
	// Expect 2 volumes - root volume is ignored.
	c.Assert(volumes, gc.HasLen, 2)
	c.Assert(attachments, gc.HasLen, 2)
	c.Check(volumes, jc.DeepEquals, []storage.Volume{
		{
			// This volume has no id_path.
			Tag:        names.NewVolumeTag("1"),
			HardwareId: "",
			VolumeId:   "volume-1",
			Size:       476893,
			Persistent: false,
		},
		{
			Tag:        names.NewVolumeTag("2"),
			HardwareId: "id_for_sdc",
			VolumeId:   "volume-2",
			Size:       238764,
			Persistent: false,
		},
	})
	c.Assert(attachments, jc.DeepEquals, []storage.VolumeAttachment{
		{
			Volume:     names.NewVolumeTag("1"),
			DeviceName: "sdb",
			Machine:    mTag,
			ReadOnly:   false,
		},
		// Device name not set because there's a hardware id in the volume.
		{
			Volume:     names.NewVolumeTag("2"),
			DeviceName: "",
			Machine:    mTag,
			ReadOnly:   false,
		},
	})
}

func (s *volumeSuite) TestInstanceVolumesOldMass(c *gc.C) {
	obj := s.testMAASObject.TestServer.NewNode(`{"system_id": "node0"}`)
	instance := maasInstance{maasObject: &obj, environ: s.makeEnviron()}
	volumes, attachments, err := instance.volumes(names.NewMachineTag("1"))
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
        }
    ], 
    "constraint_map": {
        "1": "root",
        "2": "volume-1",
        "3": "volume-2"
    }
} 
`[1:]

type storageProviderSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&storageProviderSuite{})

func (*storageProviderSuite) TestValidateConfigInvalidConfig(c *gc.C) {
	p := maasStorageProvider{}
	cfg, err := storage.NewConfig("foo", maasStorageProviderType, map[string]interface{}{
		"invalid": "config",
	})
	c.Assert(err, jc.ErrorIsNil)
	err = p.ValidateConfig(cfg)
	c.Assert(err, gc.ErrorMatches, `unknown provider config option "invalid"`)
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
