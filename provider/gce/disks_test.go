// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/provider/gce"
	//"github.com/juju/juju/provider/gce/google"
	"github.com/juju/juju/storage"
)

type storageSuite struct {
	gce.BaseSuite
}

var _ = gc.Suite(&storageSuite{})

func (s *storageSuite) TestValidateConfig(c *gc.C) {
	// This test is fake, since ValidateConfig does nothing yet.
	cfg := &storage.Config{}
	p := gce.GCEStorageProvider()
	err := p.ValidateConfig(cfg)
	c.Check(err, jc.ErrorIsNil)
}

func (s *storageSuite) TestBlockStorageSupport(c *gc.C) {
	p := gce.GCEStorageProvider()
	supports := p.Supports(storage.StorageKindBlock)
	c.Check(supports, jc.IsTrue)
}

func (s *storageSuite) TestFSStorageSupport(c *gc.C) {
	p := gce.GCEStorageProvider()
	supports := p.Supports(storage.StorageKindFilesystem)
	c.Check(supports, jc.IsFalse)
}

func (s *storageSuite) TestFSSource(c *gc.C) {
	p := gce.GCEStorageProvider()
	eConfig := &config.Config{}
	sConfig := &storage.Config{}
	_, err := p.FilesystemSource(eConfig, sConfig)
	c.Check(err, gc.ErrorMatches, "filesystems not supported")
}

func (s *storageSuite) TestCreateVolumes(c *gc.C) {

}
