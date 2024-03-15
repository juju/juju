// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"github.com/juju/charm/v13"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coremodel "github.com/juju/juju/core/model"
	domainstorage "github.com/juju/juju/domain/storage"
	"github.com/juju/juju/internal/storage"
)

type defaultsSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&defaultsSuite{})

func makeStorageDefaults(b, f string) domainstorage.StorageDefaults {
	var result domainstorage.StorageDefaults
	if b != "" {
		result.DefaultBlockSource = &b
	}
	if f != "" {
		result.DefaultFilesystemSource = &f
	}
	return result
}

func (s *defaultsSuite) assertAddApplicationStorageConstraintsDefaults(c *gc.C, pool string, cons, expect map[string]storage.Constraints) {
	err := domainstorage.StorageConstraintsWithDefaults(
		map[string]charm.Storage{
			"data":    {Name: "data", Type: charm.StorageBlock, CountMin: 1, CountMax: -1},
			"allecto": {Name: "allecto", Type: charm.StorageBlock, CountMin: 0, CountMax: -1},
		},
		coremodel.IAAS,
		makeStorageDefaults(pool, ""),
		cons,
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cons, jc.DeepEquals, expect)
}

func (s *defaultsSuite) TestAddApplicationStorageConstraintsNoConstraintsUsed(c *gc.C) {
	storageCons := map[string]storage.Constraints{
		"data": makeStorageCons("", 0, 0),
	}
	expectedCons := map[string]storage.Constraints{
		"data":    makeStorageCons("loop", 1024, 1),
		"allecto": makeStorageCons("loop", 1024, 0),
	}
	s.assertAddApplicationStorageConstraintsDefaults(c, "loop-pool", storageCons, expectedCons)
}

func (s *defaultsSuite) TestAddApplicationStorageConstraintsJustCount(c *gc.C) {
	storageCons := map[string]storage.Constraints{
		"data": makeStorageCons("", 0, 1),
	}
	expectedCons := map[string]storage.Constraints{
		"data":    makeStorageCons("loop-pool", 1024, 1),
		"allecto": makeStorageCons("loop", 1024, 0),
	}
	s.assertAddApplicationStorageConstraintsDefaults(c, "loop-pool", storageCons, expectedCons)
}

func (s *defaultsSuite) TestAddApplicationStorageConstraintsDefaultPool(c *gc.C) {
	storageCons := map[string]storage.Constraints{
		"data": makeStorageCons("", 2048, 1),
	}
	expectedCons := map[string]storage.Constraints{
		"data":    makeStorageCons("loop-pool", 2048, 1),
		"allecto": makeStorageCons("loop", 1024, 0),
	}
	s.assertAddApplicationStorageConstraintsDefaults(c, "loop-pool", storageCons, expectedCons)
}

func (s *defaultsSuite) TestAddApplicationStorageConstraintsConstraintPool(c *gc.C) {
	storageCons := map[string]storage.Constraints{
		"data": makeStorageCons("loop-pool", 2048, 1),
	}
	expectedCons := map[string]storage.Constraints{
		"data":    makeStorageCons("loop-pool", 2048, 1),
		"allecto": makeStorageCons("loop", 1024, 0),
	}
	s.assertAddApplicationStorageConstraintsDefaults(c, "", storageCons, expectedCons)
}

func (s *defaultsSuite) TestAddApplicationStorageConstraintsNoUserDefaultPool(c *gc.C) {
	storageCons := map[string]storage.Constraints{
		"data": makeStorageCons("", 2048, 1),
	}
	expectedCons := map[string]storage.Constraints{
		"data":    makeStorageCons("loop", 2048, 1),
		"allecto": makeStorageCons("loop", 1024, 0),
	}
	s.assertAddApplicationStorageConstraintsDefaults(c, "", storageCons, expectedCons)
}

func (s *defaultsSuite) TestAddApplicationStorageConstraintsDefaultSizeFallback(c *gc.C) {
	storageCons := map[string]storage.Constraints{
		"data": makeStorageCons("loop-pool", 0, 1),
	}
	expectedCons := map[string]storage.Constraints{
		"data":    makeStorageCons("loop-pool", 1024, 1),
		"allecto": makeStorageCons("loop", 1024, 0),
	}
	s.assertAddApplicationStorageConstraintsDefaults(c, "loop-pool", storageCons, expectedCons)
}

func (s *defaultsSuite) TestAddApplicationStorageConstraintsDefaultSizeFromCharm(c *gc.C) {
	storageCons := map[string]storage.Constraints{
		"multi1to10": makeStorageCons("loop", 0, 3),
	}
	expectedCons := map[string]storage.Constraints{
		"multi1to10": makeStorageCons("loop", 1024, 3),
		"multi2up":   makeStorageCons("loop", 2048, 2),
	}
	err := domainstorage.StorageConstraintsWithDefaults(
		map[string]charm.Storage{
			"multi1to10": {Name: "multi1to10", Type: charm.StorageBlock, CountMin: 1, CountMax: 10},
			"multi2up":   {Name: "multi2up", Type: charm.StorageBlock, CountMin: 2, CountMax: -1, MinimumSize: 2 * 1024},
		},
		coremodel.IAAS,
		makeStorageDefaults("", ""),
		storageCons,
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(storageCons, jc.DeepEquals, expectedCons)
}

func (s *defaultsSuite) TestProviderFallbackToType(c *gc.C) {
	storageCons := map[string]storage.Constraints{}
	expectedCons := map[string]storage.Constraints{
		"data":  makeStorageCons("loop", 1024, 1),
		"files": makeStorageCons("rootfs", 1024, 1),
	}
	err := domainstorage.StorageConstraintsWithDefaults(
		map[string]charm.Storage{
			"data":  {Name: "data", Type: charm.StorageBlock, CountMin: 1, CountMax: 1},
			"files": {Name: "files", Type: charm.StorageFilesystem, CountMin: 1, CountMax: 1},
		},
		coremodel.IAAS,
		makeStorageDefaults("", ""),
		storageCons,
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(storageCons, jc.DeepEquals, expectedCons)
}

func (s *defaultsSuite) TestProviderFallbackToTypeCaas(c *gc.C) {
	storageCons := map[string]storage.Constraints{}
	expectedCons := map[string]storage.Constraints{
		"files": makeStorageCons("kubernetes", 1024, 1),
	}
	err := domainstorage.StorageConstraintsWithDefaults(
		map[string]charm.Storage{
			"files": {Name: "files", Type: charm.StorageFilesystem, CountMin: 1, CountMax: 1},
		},
		coremodel.CAAS,
		makeStorageDefaults("", ""),
		storageCons,
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(storageCons, jc.DeepEquals, expectedCons)
}

func (s *defaultsSuite) TestProviderFallbackToTypeWithoutConstraints(c *gc.C) {
	storageCons := map[string]storage.Constraints{}
	expectedCons := map[string]storage.Constraints{
		"data":  makeStorageCons("loop", 1024, 1),
		"files": makeStorageCons("rootfs", 1024, 1),
	}
	err := domainstorage.StorageConstraintsWithDefaults(
		map[string]charm.Storage{
			"data":  {Name: "data", Type: charm.StorageBlock, CountMin: 1, CountMax: 1},
			"files": {Name: "files", Type: charm.StorageFilesystem, CountMin: 1, CountMax: 1},
		},
		coremodel.IAAS,
		makeStorageDefaults("ebs", "tmpfs"),
		storageCons,
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(storageCons, jc.DeepEquals, expectedCons)
}

func (s *defaultsSuite) TestProviderFallbackToTypeWithoutConstraintsCaas(c *gc.C) {
	storageCons := map[string]storage.Constraints{}
	expectedCons := map[string]storage.Constraints{
		"files": makeStorageCons("kubernetes", 1024, 1),
	}
	err := domainstorage.StorageConstraintsWithDefaults(
		map[string]charm.Storage{
			"files": {Name: "files", Type: charm.StorageFilesystem, CountMin: 1, CountMax: 1},
		},
		coremodel.CAAS,
		makeStorageDefaults("", "tmpfs"),
		storageCons,
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(storageCons, jc.DeepEquals, expectedCons)
}

func (s *defaultsSuite) TestProviderFallbackToDefaults(c *gc.C) {
	storageCons := map[string]storage.Constraints{
		"data":  makeStorageCons("", 2048, 1),
		"files": makeStorageCons("", 4096, 2),
	}
	expectedCons := map[string]storage.Constraints{
		"data":  makeStorageCons("ebs", 2048, 1),
		"files": makeStorageCons("tmpfs", 4096, 2),
	}
	err := domainstorage.StorageConstraintsWithDefaults(
		map[string]charm.Storage{
			"data":  {Name: "data", Type: charm.StorageBlock, CountMin: 1, CountMax: 2},
			"files": {Name: "files", Type: charm.StorageFilesystem, CountMin: 1, CountMax: 2},
		},
		coremodel.IAAS,
		makeStorageDefaults("ebs", "tmpfs"),
		storageCons,
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(storageCons, jc.DeepEquals, expectedCons)
}

func (s *defaultsSuite) TestProviderFallbackToDefaultsCaas(c *gc.C) {
	storageCons := map[string]storage.Constraints{
		"files": makeStorageCons("", 4096, 2),
	}
	expectedCons := map[string]storage.Constraints{
		"files": makeStorageCons("tmpfs", 4096, 2),
	}
	err := domainstorage.StorageConstraintsWithDefaults(
		map[string]charm.Storage{
			"files": {Name: "files", Type: charm.StorageFilesystem, CountMin: 1, CountMax: 2},
		},
		coremodel.CAAS,
		makeStorageDefaults("", "tmpfs"),
		storageCons,
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(storageCons, jc.DeepEquals, expectedCons)
}
