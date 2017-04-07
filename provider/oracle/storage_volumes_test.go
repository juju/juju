// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oracle_test

import (
	"github.com/juju/go-oracle-cloud/api"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/provider/oracle"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/testing"
	"github.com/juju/utils/clock"
	gc "gopkg.in/check.v1"
)

type oracleVolumeSource struct{}

var _ = gc.Suite(&oracleVolumeSource{})

func (o *oracleVolumeSource) NewVolumeSource(c *gc.C) storage.VolumeSource {
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
		&FakeStorageAPI{},
		clock.WallClock,
	)
	c.Assert(err, gc.IsNil)
	c.Assert(source, gc.NotNil)
	return source
}

func (o *oracleVolumeSource) TestCreateVolumesWithEmptyParams(c *gc.C) {
	source := o.NewVolumeSource(c)
	result, err := source.CreateVolumes(nil)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.NotNil)
}

func (o *oracleVolumeSource) TestCreateVolumes(c *gc.C) {
	source := o.NewVolumeSource(c)
	result, err := source.CreateVolumes([]storage.VolumeParams{
		storage.VolumeParams{
			Size:     uint64(10000),
			Provider: oracle.DefaultTypes[0],
		},
	})
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.NotNil)
}
