// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oracle_test

import (
	"errors"

	"github.com/juju/go-oracle-cloud/api"
	"github.com/juju/go-oracle-cloud/response"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/clock"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/provider/oracle"
	oracletesting "github.com/juju/juju/provider/oracle/testing"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/testing"
)

type oracleVolumeSource struct {
	testing.BaseSuite
}

var _ = gc.Suite(&oracleVolumeSource{})

func (s *oracleVolumeSource) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	oracletesting.DefaultFakeStorageAPI.ResetCalls()
}

func (o *oracleVolumeSource) NewVolumeSource(
	c *gc.C,
	fakestorage *oracletesting.FakeStorageAPI,
	fakeenv *oracletesting.FakeEnvironAPI,
) storage.VolumeSource {

	var client oracle.EnvironAPI
	if fakeenv == nil {
		client = &api.Client{}
	} else {
		client = fakeenv
	}

	environ, err := oracle.NewOracleEnviron(
		&oracle.EnvironProvider{},
		environs.OpenParams{
			Config: testing.ModelConfig(c),
		},
		client,
		&advancingClock,
	)
	c.Assert(err, gc.IsNil)
	c.Assert(environ, gc.NotNil)
	source, err := oracle.NewOracleVolumeSource(environ,
		"controller-uuid",
		"some-uuid-things-with-magic",
		fakestorage,
		clock.WallClock,
	)
	c.Assert(err, gc.IsNil)
	c.Assert(source, gc.NotNil)
	return source
}

func (o *oracleVolumeSource) TestCreateVolumesWithEmptyParams(c *gc.C) {
	source := o.NewVolumeSource(c, oracletesting.DefaultFakeStorageAPI, nil)
	result, err := source.CreateVolumes(nil)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.NotNil)
}

func (o *oracleVolumeSource) TestCreateVolumes(c *gc.C) {
	source := o.NewVolumeSource(c, oracletesting.DefaultFakeStorageAPI, nil)
	results, err := source.CreateVolumes([]storage.VolumeParams{
		storage.VolumeParams{
			Size:     uint64(10000),
			Provider: oracle.DefaultTypes[0],
		},
	})
	c.Assert(err, gc.IsNil)
	c.Assert(results, gc.HasLen, 1)
	c.Assert(results[0].Error, jc.ErrorIsNil)
}

func (o *oracleVolumeSource) TestCreateVolumesAlreadyExists(c *gc.C) {
	source := o.NewVolumeSource(c, &oracletesting.FakeStorageAPI{
		FakeComposer: oracletesting.FakeComposer{
			Compose: "/Compute-acme/jack.jones@example.com/allowed_video_servers",
		},
		FakeStorageVolume: oracletesting.FakeStorageVolume{
			StorageVolume: response.StorageVolume{
				Name: "foo",
				Size: 123 * 1024 * 1024,
				Tags: []string{
					"juju-model-uuid=some-uuid-things-with-magic",
				},
			},
			CreateErr: &api.ErrStatusConflict{},
		},
	}, nil)
	volumeTag := names.NewVolumeTag("666")
	results, err := source.CreateVolumes([]storage.VolumeParams{{
		Tag:      volumeTag,
		Size:     uint64(10000),
		Provider: oracle.DefaultTypes[0],
	}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, 1)
	c.Assert(results[0].Error, jc.ErrorIsNil)
	c.Assert(results[0].Volume, jc.DeepEquals, &storage.Volume{
		volumeTag,
		storage.VolumeInfo{
			VolumeId:   "foo",
			Size:       123,
			Persistent: true,
		},
	})
}

func (o *oracleVolumeSource) TestCreateVolumeError(c *gc.C) {
	fake := &oracletesting.FakeStorageAPI{
		FakeComposer: oracletesting.FakeComposer{
			Compose: "/Compute-acme/jack.jones@example.com/allowed_video_servers",
		},
		FakeStorageVolume: oracletesting.FakeStorageVolume{
			CreateErr: errors.New("FakeStorageVolumeErr"),
		},
	}
	source := o.NewVolumeSource(c, fake, nil)
	results, err := source.CreateVolumes([]storage.VolumeParams{
		storage.VolumeParams{
			Size:     uint64(10000),
			Provider: oracle.DefaultTypes[0],
		},
	})
	c.Assert(err, gc.IsNil)
	c.Assert(results, gc.HasLen, 1)
	c.Assert(results[0].Error, gc.NotNil)
	c.Assert(results[0].Error, gc.ErrorMatches, "FakeStorageVolumeErr")
}

func (o *oracleVolumeSource) TestListVolumes(c *gc.C) {
	source := o.NewVolumeSource(c, oracletesting.DefaultFakeStorageAPI, nil)
	volumes, err := source.ListVolumes()
	c.Assert(err, gc.IsNil)
	c.Assert(volumes, gc.NotNil)
}

func (o *oracleVolumeSource) TestListVolumesWithErrors(c *gc.C) {
	for _, fake := range []*oracletesting.FakeStorageAPI{
		&oracletesting.FakeStorageAPI{
			FakeComposer: oracletesting.FakeComposer{
				Compose: "/Compute-acme/jack.jones@example.com/allowed_video_servers",
			},
			FakeStorageVolume: oracletesting.FakeStorageVolume{
				AllErr: errors.New("FakeStorageVolumeErr"),
			},
		},
	} {
		source := o.NewVolumeSource(c, fake, nil)
		_, err := source.ListVolumes()
		c.Assert(err, gc.NotNil)
	}
}

func (o *oracleVolumeSource) TestDescribeVolumes(c *gc.C) {
	source := o.NewVolumeSource(c, oracletesting.DefaultFakeStorageAPI, nil)
	volumes, err := source.DescribeVolumes([]string{})
	c.Assert(err, gc.IsNil)
	c.Assert(volumes, gc.NotNil)

	volumes, err = source.DescribeVolumes([]string{"JujuTools_storage"})
	c.Assert(err, gc.IsNil)
	c.Assert(volumes, gc.NotNil)
}

func (o *oracleVolumeSource) TestDescribeVolumesWithErrors(c *gc.C) {
	for _, fake := range []*oracletesting.FakeStorageAPI{
		&oracletesting.FakeStorageAPI{
			FakeComposer: oracletesting.FakeComposer{
				Compose: "/Compute-acme/jack.jones@example.com/allowed_video_servers",
			},
			FakeStorageVolume: oracletesting.FakeStorageVolume{
				AllErr: errors.New("FakeStorageVolumeErr"),
			},
		},
	} {
		source := o.NewVolumeSource(c, fake, nil)
		_, err := source.DescribeVolumes([]string{"JujuTools_storage"})
		c.Assert(err, gc.NotNil)
	}
}

func (o *oracleVolumeSource) TestDestroyVolumes(c *gc.C) {
	source := o.NewVolumeSource(c, oracletesting.DefaultFakeStorageAPI, nil)
	errs, err := source.DestroyVolumes([]string{"foo"})
	c.Assert(err, gc.IsNil)
	c.Assert(errs, jc.DeepEquals, []error{nil})
}

func (o *oracleVolumeSource) TestDestroyVolumesWithErrors(c *gc.C) {
	for _, fake := range []*oracletesting.FakeStorageAPI{
		&oracletesting.FakeStorageAPI{
			FakeComposer: oracletesting.FakeComposer{
				Compose: "/Compute-acme/jack.jones@example.com/allowed_video_servers",
			},
			FakeStorageVolume: oracletesting.FakeStorageVolume{
				DeleteErr: errors.New("FakeStorageVolumeErr"),
			},
		},
	} {
		source := o.NewVolumeSource(c, fake, nil)
		errs, err := source.DestroyVolumes([]string{"JujuTools_storage"})
		c.Assert(err, gc.IsNil)
		for _, val := range errs {
			c.Assert(val, gc.NotNil)
		}
	}
}

func (o *oracleVolumeSource) TestReleaseVolumes(c *gc.C) {
	o.PatchValue(
		&oracletesting.DefaultFakeStorageAPI.StorageVolume.Tags,
		[]string{"abc", "juju-model-uuid=foo", "bar=baz"},
	)

	source := o.NewVolumeSource(c, oracletesting.DefaultFakeStorageAPI, nil)
	errs, err := source.ReleaseVolumes([]string{"foo"})
	c.Assert(err, gc.IsNil)
	c.Assert(errs, jc.DeepEquals, []error{nil})

	oracletesting.DefaultFakeStorageAPI.FakeStorageVolume.CheckCallNames(
		c, "StorageVolumeDetails", "UpdateStorageVolume",
	)
	updateCall := oracletesting.DefaultFakeStorageAPI.FakeStorageVolume.Calls()[1]
	arg0 := updateCall.Args[0].(api.StorageVolumeParams)
	c.Assert(arg0.Tags, jc.DeepEquals, []string{"abc", "bar=baz"})
}

func (o *oracleVolumeSource) TestReleaseVolumesUnchanged(c *gc.C) {
	// Volume has no tags, which means there's no update required.
	o.PatchValue(
		&oracletesting.DefaultFakeStorageAPI.StorageVolume.Tags,
		[]string{},
	)

	source := o.NewVolumeSource(c, oracletesting.DefaultFakeStorageAPI, nil)
	errs, err := source.ReleaseVolumes([]string{"foo"})
	c.Assert(err, gc.IsNil)
	c.Assert(errs, jc.DeepEquals, []error{nil})

	oracletesting.DefaultFakeStorageAPI.FakeStorageVolume.CheckCallNames(
		c, "StorageVolumeDetails",
	)
}

func (o *oracleVolumeSource) TestImportVolume(c *gc.C) {
	// Volume has no tags, which means there's no update required.
	o.PatchValue(
		&oracletesting.DefaultFakeStorageAPI.StorageVolume.Tags,
		[]string{"abc"},
	)

	source := o.NewVolumeSource(c, oracletesting.DefaultFakeStorageAPI, nil)
	c.Assert(source, gc.Implements, new(storage.VolumeImporter))

	info, err := source.(storage.VolumeImporter).ImportVolume("foo", map[string]string{"bar": "baz"})
	c.Assert(err, gc.IsNil)
	c.Assert(info, jc.DeepEquals, storage.VolumeInfo{
		VolumeId:   "/Compute-a432100/sgiulitti@cloudbase.com/JujuTools_storage",
		Size:       10,
		Persistent: true,
	})

	oracletesting.DefaultFakeStorageAPI.FakeStorageVolume.CheckCallNames(
		c, "StorageVolumeDetails", "UpdateStorageVolume",
	)
	updateCall := oracletesting.DefaultFakeStorageAPI.FakeStorageVolume.Calls()[1]
	arg0 := updateCall.Args[0].(api.StorageVolumeParams)
	c.Assert(arg0.Tags, jc.DeepEquals, []string{"abc", "bar=baz"})
}

func (o *oracleVolumeSource) TestValidateVolumeParamsWithError(c *gc.C) {
	source := o.NewVolumeSource(c, nil, nil)
	err := source.ValidateVolumeParams(
		storage.VolumeParams{
			Size: uint64(3921739812739812739),
		},
	)
	c.Assert(err, gc.NotNil)
}

func (o *oracleVolumeSource) TestValidateVolumeParams(c *gc.C) {
	source := o.NewVolumeSource(c, nil, nil)
	err := source.ValidateVolumeParams(
		storage.VolumeParams{
			Size: uint64(9999),
		},
	)
	c.Assert(err, gc.IsNil)
}

func (o *oracleVolumeSource) TestAttachVolumes(c *gc.C) {
	source := o.NewVolumeSource(c, oracletesting.DefaultFakeStorageAPI, oracletesting.DefaultEnvironAPI)
	_, err := source.AttachVolumes([]storage.VolumeAttachmentParams{
		storage.VolumeAttachmentParams{
			AttachmentParams: storage.AttachmentParams{
				Provider:   oracle.DefaultTypes[0],
				InstanceId: "0",
				ReadOnly:   false,
			},
			VolumeId: "1",
		},
	})
	c.Assert(err, gc.IsNil)
}

func (o *oracleVolumeSource) TestDetachVolumes(c *gc.C) {
	source := o.NewVolumeSource(c, oracletesting.DefaultFakeStorageAPI, nil)
	errs, err := source.DetachVolumes([]storage.VolumeAttachmentParams{
		storage.VolumeAttachmentParams{
			AttachmentParams: storage.AttachmentParams{
				Provider:   oracle.DefaultTypes[0],
				InstanceId: "JujuTools_storage",
				ReadOnly:   false,
			},
			VolumeId: "1",
		},
	})
	c.Assert(err, gc.IsNil)
	c.Assert(errs, gc.NotNil)
}

func (o *oracleVolumeSource) TestDetachVolumesWithErrors(c *gc.C) {
	source := o.NewVolumeSource(c, &oracletesting.FakeStorageAPI{
		FakeComposer: oracletesting.FakeComposer{
			Compose: "/Compute-acme/jack.jones@example.com/allowed_video_servers",
		},
		FakeStorageAttachment: oracletesting.FakeStorageAttachment{
			AllErr: errors.New("FakeStorageAttachmentErr"),
		}}, oracletesting.DefaultEnvironAPI)
	_, err := source.DetachVolumes([]storage.VolumeAttachmentParams{
		storage.VolumeAttachmentParams{
			AttachmentParams: storage.AttachmentParams{
				Provider:   oracle.DefaultTypes[0],
				InstanceId: "JujuTools_storage",
				ReadOnly:   false,
			},
			VolumeId: "1",
		},
	})
	c.Assert(err, gc.NotNil)

	source = o.NewVolumeSource(c, &oracletesting.FakeStorageAPI{
		FakeComposer: oracletesting.FakeComposer{
			Compose: "/Compute-acme/jack.jones@example.com/allowed_video_servers",
		},
		FakeStorageAttachment: oracletesting.FakeStorageAttachment{
			All: response.AllStorageAttachments{
				Result: []response.StorageAttachment{
					response.StorageAttachment{
						Account:             nil,
						Hypervisor:          nil,
						Index:               1,
						Instance_name:       "/Compute-a432100/sgiulitti@cloudbase.com/JujuTools/ebc4ce91-56bb-4120-ba78-13762597f837",
						Storage_volume_name: "/Compute-a432100/sgiulitti@cloudbase.com/JujuTools_storage",
						Name:                "/Compute-a432100/sgiulitti@cloudbase.com/JujuTools/ebc4ce91-56bb-4120-ba78-13762597f837/1f90e657-f852-45ad-afbf-9a94f640a7ae",
						Readonly:            false,
						State:               "attached",
						Uri:                 "https://compute.uscom-central-1.oraclecloud.com/storage/attachment/Compute-a432100/sgiulitti%40cloudbase.com/JujuTools/ebc4ce91-56bb-4120-ba78-13762597f837/1f90e657-f852-45ad-afbf-9a94f640a7ae",
					},
				},
			},
			DeleteErr: errors.New("FakeStorageAttachmentErr")}}, oracletesting.DefaultEnvironAPI)
	results, err := source.DetachVolumes([]storage.VolumeAttachmentParams{
		storage.VolumeAttachmentParams{
			AttachmentParams: storage.AttachmentParams{
				Provider:   oracle.DefaultTypes[0],
				InstanceId: "JujuTools_storage",
				ReadOnly:   false,
			},
			VolumeId: "1",
		},
	})

	c.Assert(err, gc.IsNil)
	for _, val := range results {
		c.Assert(val, gc.NotNil)
	}
}
