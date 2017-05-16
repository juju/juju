// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oracle_test

import (
	"errors"

	"github.com/juju/go-oracle-cloud/api"
	"github.com/juju/go-oracle-cloud/response"
	"github.com/juju/utils/clock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/provider/oracle"
	oracletesting "github.com/juju/juju/provider/oracle/testing"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/testing"
)

type oracleVolumeSource struct{}

var _ = gc.Suite(&oracleVolumeSource{})

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
	result, err := source.CreateVolumes([]storage.VolumeParams{
		storage.VolumeParams{
			Size:     uint64(10000),
			Provider: oracle.DefaultTypes[0],
		},
	})
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.NotNil)
	for _, val := range result {
		c.Assert(val.Error, gc.IsNil)
	}
}

func (o *oracleVolumeSource) TestCreateVolumesWithoutExist(c *gc.C) {
	source := o.NewVolumeSource(c, &oracletesting.FakeStorageAPI{
		FakeComposer: oracletesting.FakeComposer{
			Compose: "/Compute-acme/jack.jones@example.com/allowed_video_servers",
		},
		FakeStorageVolume: oracletesting.FakeStorageVolume{
			StorageVolumeErr: &api.ErrNotFound{},
			All:              oracletesting.DefaultAllStorageVolumes,
			AllErr:           nil,
			Create:           oracletesting.DefaultAllStorageVolumes.Result[0],
			CreateErr:        nil,
			DeleteErr:        nil,
		},
	}, nil)
	result, err := source.CreateVolumes([]storage.VolumeParams{
		storage.VolumeParams{
			Size:     uint64(10000),
			Provider: oracle.DefaultTypes[0],
		},
	})
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.NotNil)
}

func (o *oracleVolumeSource) TestCreatevolumesWithErrors(c *gc.C) {
	for _, fake := range []*oracletesting.FakeStorageAPI{
		&oracletesting.FakeStorageAPI{
			FakeComposer: oracletesting.FakeComposer{
				Compose: "/Compute-acme/jack.jones@example.com/allowed_video_servers",
			},
			FakeStorageVolume: oracletesting.FakeStorageVolume{
				StorageVolumeErr: errors.New("FakeStroageVolumeErr"),
				AllErr:           nil,
				CreateErr:        nil,
				DeleteErr:        nil,
			},
		},
		&oracletesting.FakeStorageAPI{
			FakeComposer: oracletesting.FakeComposer{
				Compose: "/Compute-acme/jack.jones@example.com/allowed_video_servers",
			},
			FakeStorageVolume: oracletesting.FakeStorageVolume{
				StorageVolume: response.StorageVolume{
					Size: 31231,
				},
				StorageVolumeErr: nil,
				AllErr:           nil,
				CreateErr:        nil,
				DeleteErr:        nil,
			},
		},
		&oracletesting.FakeStorageAPI{
			FakeComposer: oracletesting.FakeComposer{
				Compose: "/Compute-acme/jack.jones@example.com/allowed_video_servers",
			},
			FakeStorageVolume: oracletesting.FakeStorageVolume{
				StorageVolumeErr: &api.ErrNotFound{},
				AllErr:           nil,
				CreateErr:        errors.New("FakeStoraveVolumeErr"),
				DeleteErr:        nil,
			},
		},
	} {
		source := o.NewVolumeSource(c, fake, nil)
		results, err := source.CreateVolumes([]storage.VolumeParams{
			storage.VolumeParams{
				Size:     uint64(10000),
				Provider: oracle.DefaultTypes[0],
			},
		})
		c.Assert(err, gc.IsNil)
		for _, val := range results {
			c.Assert(val.Error, gc.NotNil)
		}
	}
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
	errs, err := source.DestroyVolumes([]string{})
	c.Assert(err, gc.IsNil)
	c.Assert(errs, gc.NotNil)
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
