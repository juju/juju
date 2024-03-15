// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"context"
	"fmt"

	"github.com/juju/charm/v13"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coremodel "github.com/juju/juju/core/model"
	domainstorage "github.com/juju/juju/domain/storage"
	storageerrors "github.com/juju/juju/domain/storage/errors"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/storage/provider"
)

type validationSuite struct {
	testing.IsolationSuite

	modelType coremodel.ModelType
	charm     mockCharm
}

var _ = gc.Suite(&validationSuite{})

func (s *validationSuite) SetUpTest(_ *gc.C) {
	s.modelType = coremodel.IAAS
	s.charm.meta = &charm.Meta{
		Name: "storage-block2",
		Storage: map[string]charm.Storage{
			"multi1to10": {
				Name:     "multi1to10",
				Type:     charm.StorageBlock,
				CountMin: 1,
				CountMax: 10,
			},
			"multi2up": {
				Name:        "multi2up",
				Type:        charm.StorageBlock,
				CountMin:    2,
				CountMax:    -1,
				MinimumSize: 2 * 1024,
			},
		},
	}
}

func makeStorageCons(pool string, size, count uint64) storage.Constraints {
	return storage.Constraints{
		Pool:  pool,
		Size:  size,
		Count: count,
	}
}

type mockStoragePoolGetter struct{}

func (mockStoragePoolGetter) GetStoragePoolByName(_ context.Context, name string) (domainstorage.StoragePoolDetails, error) {
	switch name {
	case "loop-pool":
		return domainstorage.StoragePoolDetails{Name: name, Provider: "loop"}, nil
	case "rootfs":
		return domainstorage.StoragePoolDetails{Name: name, Provider: "rootfs"}, nil
	case "tmp":
		return domainstorage.StoragePoolDetails{Name: name, Provider: "tmpfs", Attrs: map[string]string{"storage-medium": "foo"}}, nil
	}
	return domainstorage.StoragePoolDetails{}, fmt.Errorf("storage pool %q not found%w", name, errors.Hide(storageerrors.PoolNotFoundError))
}

type mockCharm struct {
	meta *charm.Meta
}

func (m mockCharm) Meta() *charm.Meta {
	return m.meta
}

func (s *validationSuite) validateStorageConstraints(storage map[string]storage.Constraints) error {
	validator, err := domainstorage.NewStorageConstraintsValidator(s.modelType, provider.CommonStorageProviders(), mockStoragePoolGetter{})
	if err != nil {
		return errors.Trace(err)
	}
	return validator.ValidateStorageConstraintsAgainstCharm(
		context.Background(),
		storage,
		s.charm,
	)
}

func (s *validationSuite) TestNilRegistry(c *gc.C) {
	_, err := domainstorage.NewStorageConstraintsValidator(s.modelType, nil, mockStoragePoolGetter{})
	c.Assert(err, gc.ErrorMatches, "cannot create storage constraints validator with nil registry")
}

func (s *validationSuite) TestValidateStorageConstraintsAgainstCharmSuccess(c *gc.C) {
	storageCons := map[string]storage.Constraints{
		"multi1to10": makeStorageCons("loop-pool", 1024, 10),
		"multi2up":   makeStorageCons("loop-pool", 2048, 2),
	}
	err := s.validateStorageConstraints(storageCons)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *validationSuite) TestValidateStorageConstraintsAgainstCharmStoragePoolNotFound(c *gc.C) {
	storageCons := map[string]storage.Constraints{
		"multi1to10": makeStorageCons("ebs-fast", 1024, 10),
		"multi2up":   makeStorageCons("loop-pool", 2048, 2),
	}
	err := s.validateStorageConstraints(storageCons)
	c.Assert(err, gc.ErrorMatches, `storage pool "ebs-fast" not found`)
	c.Assert(err, jc.ErrorIs, storageerrors.PoolNotFoundError)
}

func (s *validationSuite) TestValidateStorageConstraintsAgainstCharmErrors(c *gc.C) {
	assertErr := func(storage map[string]storage.Constraints, expect string) {
		err := s.validateStorageConstraints(storage)
		c.Assert(err, gc.ErrorMatches, expect)
	}

	storageCons := map[string]storage.Constraints{
		"multi1to10": makeStorageCons("loop-pool", 1024, 1),
		"multi2up":   makeStorageCons("loop-pool", 2048, 1),
	}
	assertErr(storageCons, `charm "storage-block2" store "multi2up": 2 instances required, 1 specified`)

	storageCons["multi2up"] = makeStorageCons("loop-pool", 1024, 2)
	assertErr(storageCons, `charm "storage-block2" store "multi2up": minimum storage size is 2.0 GB, 1.0 GB specified`)

	storageCons["multi2up"] = makeStorageCons("loop-pool", 2048, 2)
	storageCons["multi1to10"] = makeStorageCons("loop-pool", 1024, 11)
	assertErr(storageCons, `charm "storage-block2" store "multi1to10": at most 10 instances supported, 11 specified`)

	storageCons["multi1to10"] = makeStorageCons("rootfs", 1024, 1)
	assertErr(storageCons, `"rootfs" provider does not support "block" storage`)
}

func (s *validationSuite) TestValidateStorageConstraintsAgainstCharmCaasBlockNotSupported(c *gc.C) {
	s.modelType = coremodel.CAAS
	storageCons := map[string]storage.Constraints{
		"multi1to10": makeStorageCons("loop-pool", 1024, 1),
		"multi2up":   makeStorageCons("loop-pool", 2048, 2),
	}
	err := s.validateStorageConstraints(storageCons)
	c.Assert(err, gc.ErrorMatches, `block storage on a container model not supported`)
}

func (s *validationSuite) TestValidateStorageConstraintsAgainstCharmCaas(c *gc.C) {
	s.modelType = coremodel.CAAS
	s.charm.meta = &charm.Meta{
		Name: "storage-block2",
		Storage: map[string]charm.Storage{
			"files": {
				Name:     "tmp",
				Type:     charm.StorageFilesystem,
				CountMin: 1,
				CountMax: -1,
			},
		},
	}

	storageCons := map[string]storage.Constraints{
		"files": makeStorageCons("tmp", 2048, 1),
	}
	err := s.validateStorageConstraints(storageCons)
	c.Assert(err, gc.ErrorMatches, `invalid storage config: storage medium "foo" not valid`)
}
