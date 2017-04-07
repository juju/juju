// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oracle_test

import (
	"errors"

	"github.com/juju/go-oracle-cloud/api"
	"github.com/juju/go-oracle-cloud/response"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/provider/oracle"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/testing"
	"github.com/juju/utils/clock"
	gc "gopkg.in/check.v1"
)

type oracleVolumeSource struct{}

var _ = gc.Suite(&oracleVolumeSource{})

func (o *oracleVolumeSource) NewVolumeSource(c *gc.C, fake *FakeStorageAPI) storage.VolumeSource {
	environ, err := oracle.NewOracleEnviron(
		oracle.DefaultProvider,
		environs.OpenParams{
			Config: testing.ModelConfig(c),
		},
		&api.Client{},
	)
	c.Assert(err, gc.IsNil)
	c.Assert(environ, gc.NotNil)
	source, err := oracle.NewOracleVolumeSource(environ,
		"controller-uuid",
		"some-uuid-things-with-magic",
		fake,
		clock.WallClock,
	)
	c.Assert(err, gc.IsNil)
	c.Assert(source, gc.NotNil)
	return source
}

func (o *oracleVolumeSource) TestCreateVolumesWithEmptyParams(c *gc.C) {
	source := o.NewVolumeSource(c, DefaultFakeStorageAPI)
	result, err := source.CreateVolumes(nil)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.NotNil)
}

func (o *oracleVolumeSource) TestCreateVolumes(c *gc.C) {
	source := o.NewVolumeSource(c, DefaultFakeStorageAPI)
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
	source := o.NewVolumeSource(c, &FakeStorageAPI{
		FakeComposer: FakeComposer{
			compose: "/Compute-acme/jack.jones@example.com/allowed_video_servers",
		},
		FakeStorageVolume: FakeStorageVolume{
			StorageVolumeErr: &api.ErrNotFound{},
			All:              DefaultAllStorageVolumes,
			AllErr:           nil,
			Create:           DefaultAllStorageVolumes.Result[0],
			CreateErr:        nil,
			DeleteErr:        nil,
		},
	})
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
	for _, fake := range []*FakeStorageAPI{
		&FakeStorageAPI{
			FakeComposer: FakeComposer{
				compose: "/Compute-acme/jack.jones@example.com/allowed_video_servers",
			},
			FakeStorageVolume: FakeStorageVolume{
				StorageVolumeErr: errors.New("FakeStroageVolumeErr"),
				AllErr:           nil,
				CreateErr:        nil,
				DeleteErr:        nil,
			},
		},
		&FakeStorageAPI{
			FakeComposer: FakeComposer{
				compose: "/Compute-acme/jack.jones@example.com/allowed_video_servers",
			},
			FakeStorageVolume: FakeStorageVolume{
				StorageVolume: response.StorageVolume{
					Size: 31231,
				},
				StorageVolumeErr: nil,
				AllErr:           nil,
				CreateErr:        nil,
				DeleteErr:        nil,
			},
		},
		&FakeStorageAPI{
			FakeComposer: FakeComposer{
				compose: "/Compute-acme/jack.jones@example.com/allowed_video_servers",
			},
			FakeStorageVolume: FakeStorageVolume{
				StorageVolumeErr: &api.ErrNotFound{},
				AllErr:           nil,
				CreateErr:        errors.New("FakeStoraveVolumeErr"),
				DeleteErr:        nil,
			},
		},
	} {
		source := o.NewVolumeSource(c, fake)
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
