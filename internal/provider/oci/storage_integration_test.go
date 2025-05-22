// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oci_test

import (
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/provider/oci"
	"github.com/juju/juju/internal/storage"
)

type storageSuite struct {
	commonSuite

	provider storage.Provider
}

func TestStorageSuite(t *stdtesting.T) {
	tc.Run(t, &storageSuite{})
}

func (s *storageSuite) SetUpTest(c *tc.C) {
	s.commonSuite.SetUpTest(c)

	var err error
	s.provider, err = s.env.StorageProvider(oci.OciStorageProviderType)
	c.Assert(err, tc.IsNil)
}

func (s *storageSuite) TestVolumeSource(c *tc.C) {
	source, err := s.provider.VolumeSource(nil)
	c.Assert(err, tc.IsNil)
	c.Assert(source, tc.NotNil)
	cfg, err := storage.NewConfig("iscsi", oci.OciStorageProviderType,
		map[string]interface{}{
			oci.OciVolumeType: oci.IscsiPool,
		})
	c.Assert(err, tc.IsNil)
	c.Assert(cfg, tc.NotNil)

	source, err = s.provider.VolumeSource(cfg)
	c.Assert(err, tc.IsNil)
	c.Assert(source, tc.NotNil)
}

func (s *storageSuite) TestSupports(c *tc.C) {
	ok := s.provider.Supports(storage.StorageKindBlock)
	c.Assert(ok, tc.IsTrue)

	ok = s.provider.Supports(storage.StorageKindFilesystem)
	c.Assert(ok, tc.IsFalse)
}

func (s *storageSuite) TestDynamic(c *tc.C) {
	ok := s.provider.Dynamic()
	c.Assert(ok, tc.IsTrue)
}

func (s *storageSuite) TestValidateConfig(c *tc.C) {
	cfg, err := storage.NewConfig("iscsi", oci.OciStorageProviderType,
		map[string]interface{}{
			oci.OciVolumeType: oci.IscsiPool,
		})
	c.Assert(err, tc.IsNil)
	err = s.provider.ValidateConfig(cfg)
	c.Assert(err, tc.IsNil)
}

func (s *storageSuite) TestValidateConfigWithError(c *tc.C) {
	cfg, err := storage.NewConfig("random-pool", oci.OciStorageProviderType,
		map[string]interface{}{
			oci.OciVolumeType: "no-idea-what-I-am",
		})
	c.Assert(err, tc.IsNil)

	err = s.provider.ValidateConfig(cfg)
	c.Assert(err, tc.ErrorMatches, "invalid volume-type \"no-idea-what-I-am\"")
}
