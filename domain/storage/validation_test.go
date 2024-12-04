// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"context"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coremodel "github.com/juju/juju/core/model"
	domainstorage "github.com/juju/juju/domain/storage"
	storageerrors "github.com/juju/juju/domain/storage/errors"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/storage/provider"
)

type validationSuite struct {
	testing.IsolationSuite

	modelType coremodel.ModelType
	meta      *charm.Meta
}

var _ = gc.Suite(&validationSuite{})

func (s *validationSuite) SetUpTest(_ *gc.C) {
	s.modelType = coremodel.IAAS
	s.meta = &charm.Meta{
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

func makeStorageDirective(pool string, size, count uint64) storage.Directive {
	return storage.Directive{
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
	return domainstorage.StoragePoolDetails{}, errors.Errorf("storage pool %q not found", name).Add(storageerrors.PoolNotFoundError)
}

func (s *validationSuite) validateStorageDirectives(storage map[string]storage.Directive) error {
	validator, err := domainstorage.NewStorageDirectivesValidator(s.modelType, provider.CommonStorageProviders(), mockStoragePoolGetter{})
	if err != nil {
		return errors.Capture(err)
	}
	return validator.ValidateStorageDirectivesAgainstCharm(
		context.Background(),
		storage,
		s.meta,
	)
}

func (s *validationSuite) TestNilRegistry(c *gc.C) {
	_, err := domainstorage.NewStorageDirectivesValidator(s.modelType, nil, mockStoragePoolGetter{})
	c.Assert(err, gc.ErrorMatches, "cannot create storage directives validator with nil registry")
}

func (s *validationSuite) TestValidateStorageDirectivesAgainstCharmSuccess(c *gc.C) {
	storageDirectives := map[string]storage.Directive{
		"multi1to10": makeStorageDirective("loop-pool", 1024, 10),
		"multi2up":   makeStorageDirective("loop-pool", 2048, 2),
	}
	err := s.validateStorageDirectives(storageDirectives)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *validationSuite) TestValidateStorageDirectivesAgainstCharmStoragePoolNotFound(c *gc.C) {
	storageDirectives := map[string]storage.Directive{
		"multi1to10": makeStorageDirective("ebs-fast", 1024, 10),
		"multi2up":   makeStorageDirective("loop-pool", 2048, 2),
	}
	err := s.validateStorageDirectives(storageDirectives)
	c.Assert(err, gc.ErrorMatches, `storage pool "ebs-fast" not found`)
	c.Assert(err, jc.ErrorIs, storageerrors.PoolNotFoundError)
}

func (s *validationSuite) TestValidateStorageDirectivesAgainstCharmErrors(c *gc.C) {
	assertErr := func(storage map[string]storage.Directive, expect string) {
		err := s.validateStorageDirectives(storage)
		c.Assert(err, gc.ErrorMatches, expect)
	}

	storageDirectives := map[string]storage.Directive{
		"multi1to10": makeStorageDirective("loop-pool", 1024, 1),
		"multi2up":   makeStorageDirective("loop-pool", 2048, 1),
	}
	assertErr(storageDirectives, `charm "storage-block2" store "multi2up": 2 instances required, 1 specified`)

	storageDirectives["multi2up"] = makeStorageDirective("loop-pool", 1024, 2)
	assertErr(storageDirectives, `charm "storage-block2" store "multi2up": minimum storage size is 2.0 GB, 1.0 GB specified`)

	storageDirectives["multi2up"] = makeStorageDirective("loop-pool", 2048, 2)
	storageDirectives["multi1to10"] = makeStorageDirective("loop-pool", 1024, 11)
	assertErr(storageDirectives, `charm "storage-block2" store "multi1to10": at most 10 instances supported, 11 specified`)

	storageDirectives["multi1to10"] = makeStorageDirective("rootfs", 1024, 1)
	assertErr(storageDirectives, `"rootfs" provider does not support "block" storage`)
}

func (s *validationSuite) TestValidateStorageDirectivesAgainstCharmCaasBlockNotSupported(c *gc.C) {
	s.modelType = coremodel.CAAS
	storageDirectives := map[string]storage.Directive{
		"multi1to10": makeStorageDirective("loop-pool", 1024, 1),
		"multi2up":   makeStorageDirective("loop-pool", 2048, 2),
	}
	err := s.validateStorageDirectives(storageDirectives)
	c.Assert(err, gc.ErrorMatches, `block storage on a container model not supported`)
}

func (s *validationSuite) TestValidateStorageDirectivesAgainstCharmCaas(c *gc.C) {
	s.modelType = coremodel.CAAS
	s.meta = &charm.Meta{
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

	storageDirectives := map[string]storage.Directive{
		"files": makeStorageDirective("tmp", 2048, 1),
	}
	err := s.validateStorageDirectives(storageDirectives)
	c.Assert(err, gc.ErrorMatches, `invalid storage config: storage medium "foo" not valid`)
}
