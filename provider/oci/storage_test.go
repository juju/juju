// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oci_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/provider/oci"
	"github.com/juju/juju/storage"
)

type storageSuite struct {
	commonSuite

	provider storage.Provider
}

var _ = gc.Suite(&storageSuite{})

func (s *storageSuite) SetUpTest(c *gc.C) {
	s.commonSuite.SetUpTest(c)

	var err error
	s.provider, err = s.env.StorageProvider(oci.OciStorageProviderType)
	c.Assert(err, gc.IsNil)
}

func (s *storageSuite) TestVolumeSource(c *gc.C) {
	source, err := s.provider.VolumeSource(nil)
	c.Assert(err, gc.IsNil)
	c.Assert(source, gc.NotNil)
	cfg, err := storage.NewConfig("iscsi", oci.OciStorageProviderType,
		map[string]interface{}{
			oci.OciVolumeType: oci.IscsiPool,
		})
	c.Assert(err, gc.IsNil)
	c.Assert(cfg, gc.NotNil)

	source, err = s.provider.VolumeSource(cfg)
	c.Assert(err, gc.IsNil)
	c.Assert(source, gc.NotNil)
}

func (s *storageSuite) TestSupports(c *gc.C) {
	ok := s.provider.Supports(storage.StorageKindBlock)
	c.Assert(ok, jc.IsTrue)

	ok = s.provider.Supports(storage.StorageKindFilesystem)
	c.Assert(ok, jc.IsFalse)
}

func (s *storageSuite) TestDynamic(c *gc.C) {
	ok := s.provider.Dynamic()
	c.Assert(ok, jc.IsTrue)
}

func (s *storageSuite) TestValidateConfig(c *gc.C) {
	cfg, err := storage.NewConfig("iscsi", oci.OciStorageProviderType,
		map[string]interface{}{
			oci.OciVolumeType: oci.IscsiPool,
		})
	c.Assert(err, gc.IsNil)
	err = s.provider.ValidateConfig(cfg)
	c.Assert(err, gc.IsNil)
}

func (s *storageSuite) TestValidateConfigWithError(c *gc.C) {
	cfg, err := storage.NewConfig("random-pool", oci.OciStorageProviderType,
		map[string]interface{}{
			oci.OciVolumeType: "no-idea-what-I-am",
		})
	c.Assert(err, gc.IsNil)

	err = s.provider.ValidateConfig(cfg)
	c.Assert(err, gc.ErrorMatches, "invalid volume-type \"no-idea-what-I-am\"")
}
