// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	coreerrors "github.com/juju/juju/core/errors"
	domainstorage "github.com/juju/juju/domain/storage"
	storageerrors "github.com/juju/juju/domain/storage/errors"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/storage/provider"
	dummystorage "github.com/juju/juju/internal/storage/provider/dummy"
)

type storagePoolServiceSuite struct {
	testing.IsolationSuite

	state    *MockState
	registry storage.ProviderRegistry
}

var _ = gc.Suite(&storagePoolServiceSuite{})

const validationError = errors.ConstError("missing attribute foo")

func (s *storagePoolServiceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockState(ctrl)

	s.registry = storage.ChainedProviderRegistry{storage.StaticProviderRegistry{
		Providers: map[storage.ProviderType]storage.Provider{
			"ebs": &dummystorage.StorageProvider{
				ValidateConfigFunc: func(sp *storage.Config) error {
					if _, ok := sp.Attrs()["foo"]; !ok {
						return validationError
					}
					return nil
				},
			},
		},
	}, provider.CommonStorageProviders()}

	return ctrl
}

func (s *storagePoolServiceSuite) service(c *gc.C) *Service {
	return NewService(s.state, loggertesting.WrapCheckLog(c), modelStorageRegistryGetter(func() storage.ProviderRegistry {
		return s.registry
	}))
}

func (s *storagePoolServiceSuite) TestCreateStoragePool(c *gc.C) {
	defer s.setupMocks(c).Finish()

	sp := domainstorage.StoragePoolDetails{
		Name:     "ebs-fast",
		Provider: "ebs",
		Attrs: map[string]string{
			"foo": "foo val",
		},
	}
	s.state.EXPECT().CreateStoragePool(gomock.Any(), sp).Return(nil)

	err := s.service(c).CreateStoragePool(context.Background(), "ebs-fast", "ebs", PoolAttrs{"foo": "foo val"})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *storagePoolServiceSuite) TestCreateStoragePoolInvalidName(c *gc.C) {
	defer s.setupMocks(c).Finish()

	err := s.service(c).CreateStoragePool(context.Background(), "66invalid", "ebs", PoolAttrs{"foo": "foo val"})
	c.Assert(err, jc.ErrorIs, storageerrors.InvalidPoolNameError)
}

func (s *storagePoolServiceSuite) TestCreateStoragePoolMissingName(c *gc.C) {
	defer s.setupMocks(c).Finish()

	err := s.service(c).CreateStoragePool(context.Background(), "", "ebs", PoolAttrs{"foo": "foo val"})
	c.Assert(err, jc.ErrorIs, storageerrors.MissingPoolNameError)
}

func (s *storagePoolServiceSuite) TestCreateStoragePoolMissingType(c *gc.C) {
	defer s.setupMocks(c).Finish()

	err := s.service(c).CreateStoragePool(context.Background(), "ebs-fast", "", PoolAttrs{"foo": "foo val"})
	c.Assert(err, jc.ErrorIs, storageerrors.MissingPoolTypeError)
}

func (s *storagePoolServiceSuite) TestCreateStoragePoolValidates(c *gc.C) {
	defer s.setupMocks(c).Finish()

	err := s.service(c).CreateStoragePool(context.Background(), "ebs-fast", "ebs", PoolAttrs{"bar": "bar val"})
	c.Assert(err, jc.ErrorIs, validationError)
	c.Assert(err, gc.ErrorMatches, `.* missing attribute foo`)
}

func (s *storagePoolServiceSuite) TestDeleteStoragePool(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().DeleteStoragePool(gomock.Any(), "ebs-fast").Return(nil)

	err := s.service(c).DeleteStoragePool(context.Background(), "ebs-fast")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *storagePoolServiceSuite) TestReplaceStoragePool(c *gc.C) {
	defer s.setupMocks(c).Finish()

	sp := domainstorage.StoragePoolDetails{
		Name:     "ebs-fast",
		Provider: "ebs",
		Attrs: map[string]string{
			"foo": "foo val",
		},
	}
	s.state.EXPECT().ReplaceStoragePool(gomock.Any(), sp).Return(nil)

	err := s.service(c).ReplaceStoragePool(context.Background(), "ebs-fast", "ebs", PoolAttrs{"foo": "foo val"})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *storagePoolServiceSuite) TestReplaceStoragePoolInvalidName(c *gc.C) {
	defer s.setupMocks(c).Finish()

	err := s.service(c).ReplaceStoragePool(context.Background(), "66invalid", "ebs", PoolAttrs{"foo": "foo val"})
	c.Assert(err, jc.ErrorIs, storageerrors.InvalidPoolNameError)
}

func (s *storagePoolServiceSuite) TestReplaceStoragePoolMissingName(c *gc.C) {
	defer s.setupMocks(c).Finish()

	err := s.service(c).ReplaceStoragePool(context.Background(), "", "ebs", PoolAttrs{"foo": "foo val"})
	c.Assert(err, jc.ErrorIs, storageerrors.MissingPoolNameError)
}

func (s *storagePoolServiceSuite) TestReplaceStoragePoolExistingProvider(c *gc.C) {
	defer s.setupMocks(c).Finish()

	sp := domainstorage.StoragePoolDetails{
		Name:     "ebs-fast",
		Provider: "ebs",
		Attrs: map[string]string{
			"foo": "foo val",
		},
	}
	s.state.EXPECT().GetStoragePoolByName(gomock.Any(), "ebs-fast").Return(sp, nil)
	s.state.EXPECT().ReplaceStoragePool(gomock.Any(), sp).Return(nil)

	err := s.service(c).ReplaceStoragePool(context.Background(), "ebs-fast", "", PoolAttrs{"foo": "foo val"})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *storagePoolServiceSuite) TestReplaceStoragePoolValidates(c *gc.C) {
	defer s.setupMocks(c).Finish()

	err := s.service(c).ReplaceStoragePool(context.Background(), "ebs-fast", "ebs", PoolAttrs{"bar": "bar val"})
	c.Assert(err, jc.ErrorIs, validationError)
	c.Assert(err, gc.ErrorMatches, `.* missing attribute foo`)
}

func (s *storagePoolServiceSuite) builtInPools(c *gc.C) []*storage.Config {
	builtin, err := domainstorage.BuiltInStoragePools()
	c.Assert(err, jc.ErrorIsNil)
	result := make([]*storage.Config, len(builtin))
	for i, p := range builtin {
		sc, err := storage.NewConfig(p.Name, storage.ProviderType(p.Provider), nil)
		c.Assert(err, jc.ErrorIsNil)
		result[i] = sc
	}
	return result
}

func (s *storagePoolServiceSuite) TestAllStoragePools(c *gc.C) {
	defer s.setupMocks(c).Finish()

	sp := domainstorage.StoragePoolDetails{
		Name:     "ebs-fast",
		Provider: "ebs",
		Attrs: map[string]string{
			"foo": "foo val",
		},
	}
	s.state.EXPECT().ListStoragePools(gomock.Any(), domainstorage.NilNames, domainstorage.NilProviders).Return([]domainstorage.StoragePoolDetails{sp}, nil)

	got, err := s.service(c).AllStoragePools(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	expected, err := storage.NewConfig("ebs-fast", "ebs", storage.Attrs{"foo": "foo val"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, jc.SameContents, append(s.builtInPools(c), expected))
}

func (s *storagePoolServiceSuite) TestListStoragePoolsValidFilter(c *gc.C) {
	defer s.setupMocks(c).Finish()

	sp := domainstorage.StoragePoolDetails{
		Name:     "ebs-fast",
		Provider: "ebs",
		Attrs: map[string]string{
			"foo": "foo val",
		},
	}
	s.state.EXPECT().ListStoragePools(gomock.Any(), domainstorage.Names{"ebs-fast"}, domainstorage.Providers{"ebs"}).
		Return([]domainstorage.StoragePoolDetails{sp}, nil)

	got, err := s.service(c).ListStoragePools(context.Background(), domainstorage.Names{"ebs-fast"}, domainstorage.Providers{"ebs"})
	c.Assert(err, jc.ErrorIsNil)
	expected, err := storage.NewConfig("ebs-fast", "ebs", storage.Attrs{"foo": "foo val"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, jc.SameContents, append(s.builtInPools(c), expected))
}

func (s *storagePoolServiceSuite) TestListStoragePoolsInvalidFilterName(c *gc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.service(c).ListStoragePools(context.Background(), domainstorage.Names{"666invalid"}, domainstorage.NilProviders)
	c.Assert(err, jc.ErrorIs, storageerrors.InvalidPoolNameError)
	c.Assert(err, gc.ErrorMatches, `pool name "666invalid" not valid`)
}

func (s *storagePoolServiceSuite) TestListStoragePoolsInvalidFilterProvider(c *gc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.service(c).ListStoragePools(context.Background(), domainstorage.NilNames, domainstorage.Providers{"invalid"})
	c.Assert(err, jc.ErrorIs, coreerrors.NotFound)
	c.Assert(err, gc.ErrorMatches, `storage provider "invalid" not found`)
}

func (s *storagePoolServiceSuite) TestGetStoragePoolByName(c *gc.C) {
	defer s.setupMocks(c).Finish()

	sp := domainstorage.StoragePoolDetails{
		Name:     "ebs-fast",
		Provider: "ebs",
		Attrs: map[string]string{
			"foo": "foo val",
		},
	}
	s.state.EXPECT().GetStoragePoolByName(gomock.Any(), "ebs-fast").Return(sp, nil)

	got, err := s.service(c).GetStoragePoolByName(context.Background(), "ebs-fast")
	c.Assert(err, jc.ErrorIsNil)
	expected, err := storage.NewConfig("ebs-fast", "ebs", storage.Attrs{"foo": "foo val"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, jc.DeepEquals, expected)
}
