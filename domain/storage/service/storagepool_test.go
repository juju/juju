// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	coreerrors "github.com/juju/juju/core/errors"
	domainstorage "github.com/juju/juju/domain/storage"
	storageerrors "github.com/juju/juju/domain/storage/errors"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/storage/provider"
	dummystorage "github.com/juju/juju/internal/storage/provider/dummy"
	"github.com/juju/juju/internal/testhelpers"
)

type storagePoolServiceSuite struct {
	testhelpers.IsolationSuite

	state    *MockState
	registry storage.ProviderRegistry
}

func TestStoragePoolServiceSuite(t *testing.T) {
	tc.Run(t, &storagePoolServiceSuite{})
}

const validationError = errors.ConstError("missing attribute foo")

func (s *storagePoolServiceSuite) setupMocks(c *tc.C) *gomock.Controller {
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

func (s *storagePoolServiceSuite) service(c *tc.C) *Service {
	return NewService(s.state, loggertesting.WrapCheckLog(c), modelStorageRegistryGetter(func() storage.ProviderRegistry {
		return s.registry
	}))
}

func (s *storagePoolServiceSuite) TestCreateStoragePool(c *tc.C) {
	defer s.setupMocks(c).Finish()

	sp := domainstorage.StoragePool{
		Name:     "ebs-fast",
		Provider: "ebs",
		Attrs: map[string]string{
			"foo": "foo val",
		},
	}
	s.state.EXPECT().CreateStoragePool(gomock.Any(), sp).Return(nil)

	err := s.service(c).CreateStoragePool(c.Context(), "ebs-fast", "ebs", PoolAttrs{"foo": "foo val"})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *storagePoolServiceSuite) TestCreateStoragePoolInvalidName(c *tc.C) {
	defer s.setupMocks(c).Finish()

	err := s.service(c).CreateStoragePool(c.Context(), "66invalid", "ebs", PoolAttrs{"foo": "foo val"})
	c.Assert(err, tc.ErrorIs, storageerrors.InvalidPoolNameError)
}

func (s *storagePoolServiceSuite) TestCreateStoragePoolMissingName(c *tc.C) {
	defer s.setupMocks(c).Finish()

	err := s.service(c).CreateStoragePool(c.Context(), "", "ebs", PoolAttrs{"foo": "foo val"})
	c.Assert(err, tc.ErrorIs, storageerrors.MissingPoolNameError)
}

func (s *storagePoolServiceSuite) TestCreateStoragePoolMissingType(c *tc.C) {
	defer s.setupMocks(c).Finish()

	err := s.service(c).CreateStoragePool(c.Context(), "ebs-fast", "", PoolAttrs{"foo": "foo val"})
	c.Assert(err, tc.ErrorIs, storageerrors.MissingPoolTypeError)
}

func (s *storagePoolServiceSuite) TestCreateStoragePoolValidates(c *tc.C) {
	defer s.setupMocks(c).Finish()

	err := s.service(c).CreateStoragePool(c.Context(), "ebs-fast", "ebs", PoolAttrs{"bar": "bar val"})
	c.Assert(err, tc.ErrorIs, validationError)
	c.Assert(err, tc.ErrorMatches, `.* missing attribute foo`)
}

func (s *storagePoolServiceSuite) TestDeleteStoragePool(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().DeleteStoragePool(gomock.Any(), "ebs-fast").Return(nil)

	err := s.service(c).DeleteStoragePool(c.Context(), "ebs-fast")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *storagePoolServiceSuite) TestReplaceStoragePool(c *tc.C) {
	defer s.setupMocks(c).Finish()

	sp := domainstorage.StoragePool{
		Name:     "ebs-fast",
		Provider: "ebs",
		Attrs: map[string]string{
			"foo": "foo val",
		},
	}
	s.state.EXPECT().ReplaceStoragePool(gomock.Any(), sp).Return(nil)

	err := s.service(c).ReplaceStoragePool(c.Context(), "ebs-fast", "ebs", PoolAttrs{"foo": "foo val"})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *storagePoolServiceSuite) TestReplaceStoragePoolInvalidName(c *tc.C) {
	defer s.setupMocks(c).Finish()

	err := s.service(c).ReplaceStoragePool(c.Context(), "66invalid", "ebs", PoolAttrs{"foo": "foo val"})
	c.Assert(err, tc.ErrorIs, storageerrors.InvalidPoolNameError)
}

func (s *storagePoolServiceSuite) TestReplaceStoragePoolMissingName(c *tc.C) {
	defer s.setupMocks(c).Finish()

	err := s.service(c).ReplaceStoragePool(c.Context(), "", "ebs", PoolAttrs{"foo": "foo val"})
	c.Assert(err, tc.ErrorIs, storageerrors.MissingPoolNameError)
}

func (s *storagePoolServiceSuite) TestReplaceStoragePoolExistingProvider(c *tc.C) {
	defer s.setupMocks(c).Finish()

	sp := domainstorage.StoragePool{
		Name:     "ebs-fast",
		Provider: "ebs",
		Attrs: map[string]string{
			"foo": "foo val",
		},
	}
	s.state.EXPECT().GetStoragePoolByName(gomock.Any(), "ebs-fast").Return(sp, nil)
	s.state.EXPECT().ReplaceStoragePool(gomock.Any(), sp).Return(nil)

	err := s.service(c).ReplaceStoragePool(c.Context(), "ebs-fast", "", PoolAttrs{"foo": "foo val"})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *storagePoolServiceSuite) TestReplaceStoragePoolValidates(c *tc.C) {
	defer s.setupMocks(c).Finish()

	err := s.service(c).ReplaceStoragePool(c.Context(), "ebs-fast", "ebs", PoolAttrs{"bar": "bar val"})
	c.Assert(err, tc.ErrorIs, validationError)
	c.Assert(err, tc.ErrorMatches, `.* missing attribute foo`)
}

func (s *storagePoolServiceSuite) TestListStoragePoolsWithoutBuiltins(c *tc.C) {
	defer s.setupMocks(c).Finish()

	sp := domainstorage.StoragePool{
		Name:     "ebs-fast",
		Provider: "ebs",
		Attrs: map[string]string{
			"foo": "foo val",
		},
	}
	s.state.EXPECT().ListStoragePoolsWithoutBuiltins(gomock.Any()).Return([]domainstorage.StoragePool{sp}, nil)

	got, err := s.service(c).ListStoragePoolsWithoutBuiltins(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(got, tc.SameContents, []domainstorage.StoragePool{sp})
}

func (s *storagePoolServiceSuite) TestListStoragePools(c *tc.C) {
	defer s.setupMocks(c).Finish()

	sp := domainstorage.StoragePool{
		Name:     "ebs-fast",
		Provider: "ebs",
		Attrs: map[string]string{
			"foo": "foo val",
		},
	}
	s.state.EXPECT().ListStoragePools(gomock.Any()).Return([]domainstorage.StoragePool{sp}, nil)

	got, err := s.service(c).ListStoragePools(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(got, tc.SameContents, []domainstorage.StoragePool{sp})
}

func (s *storagePoolServiceSuite) TestListStoragePoolsByNamesAndProviders(c *tc.C) {
	defer s.setupMocks(c).Finish()

	sp := domainstorage.StoragePool{
		Name:     "ebs-fast",
		Provider: "ebs",
		Attrs: map[string]string{
			"foo": "foo val",
		},
	}
	s.state.EXPECT().ListStoragePoolsByNamesAndProviders(gomock.Any(), domainstorage.Names{"ebs-fast"}, domainstorage.Providers{"ebs"}).
		Return([]domainstorage.StoragePool{sp}, nil)

	got, err := s.service(c).ListStoragePoolsByNamesAndProviders(c.Context(), domainstorage.Names{"ebs-fast"}, domainstorage.Providers{"ebs"})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(got, tc.SameContents, []domainstorage.StoragePool{sp})
}

func (s *storagePoolServiceSuite) TestListStoragePoolsByNamesAndProvidersEmptyArgs(c *tc.C) {
	defer s.setupMocks(c).Finish()

	got, err := s.service(c).ListStoragePoolsByNamesAndProviders(c.Context(), domainstorage.Names{}, domainstorage.Providers{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(got, tc.HasLen, 0)
}

func (s *storagePoolServiceSuite) TestListStoragePoolsByNamesAndProvidersInvalidNames(c *tc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.service(c).ListStoragePoolsByNamesAndProviders(c.Context(), domainstorage.Names{"666invalid"}, domainstorage.Providers{"ebs"})
	c.Assert(err, tc.ErrorIs, storageerrors.InvalidPoolNameError)
	c.Assert(err, tc.ErrorMatches, `pool name "666invalid" not valid`)
}

func (s *storagePoolServiceSuite) TestListStoragePoolsByNamesAndProvidersInvalidProviders(c *tc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.service(c).ListStoragePoolsByNamesAndProviders(c.Context(), domainstorage.Names{"loop"}, domainstorage.Providers{"invalid"})
	c.Assert(err, tc.ErrorIs, coreerrors.NotFound)
	c.Assert(err, tc.ErrorMatches, `storage provider "invalid" not found`)
}

func (s *storagePoolServiceSuite) TestListStoragePoolsByNames(c *tc.C) {
	defer s.setupMocks(c).Finish()

	sp := domainstorage.StoragePool{
		Name:     "ebs-fast",
		Provider: "ebs",
		Attrs: map[string]string{
			"foo": "foo val",
		},
	}
	s.state.EXPECT().ListStoragePoolsByNames(gomock.Any(), domainstorage.Names{"ebs-fast"}).
		Return([]domainstorage.StoragePool{sp}, nil)

	got, err := s.service(c).ListStoragePoolsByNames(c.Context(), domainstorage.Names{"ebs-fast"})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(got, tc.SameContents, []domainstorage.StoragePool{sp})
}

func (s *storagePoolServiceSuite) TestListStoragePoolsByNamesEmptyArgs(c *tc.C) {
	defer s.setupMocks(c).Finish()

	got, err := s.service(c).ListStoragePoolsByNames(c.Context(), domainstorage.Names{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(got, tc.HasLen, 0)
}

func (s *storagePoolServiceSuite) TestListStoragePoolsByNamesInvalidNames(c *tc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.service(c).ListStoragePoolsByNames(c.Context(), domainstorage.Names{"666invalid"})
	c.Assert(err, tc.ErrorIs, storageerrors.InvalidPoolNameError)
	c.Assert(err, tc.ErrorMatches, `pool name "666invalid" not valid`)
}

func (s *storagePoolServiceSuite) TestListStoragePoolsByProviders(c *tc.C) {
	defer s.setupMocks(c).Finish()

	sp := domainstorage.StoragePool{
		Name:     "ebs-fast",
		Provider: "ebs",
		Attrs: map[string]string{
			"foo": "foo val",
		},
	}
	s.state.EXPECT().ListStoragePoolsByProviders(gomock.Any(), domainstorage.Providers{"ebs"}).
		Return([]domainstorage.StoragePool{sp}, nil)

	got, err := s.service(c).ListStoragePoolsByProviders(c.Context(), domainstorage.Providers{"ebs"})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(got, tc.SameContents, []domainstorage.StoragePool{sp})
}

func (s *storagePoolServiceSuite) TestListStoragePoolsByProvidersEmptyArgs(c *tc.C) {
	defer s.setupMocks(c).Finish()

	got, err := s.service(c).ListStoragePoolsByProviders(c.Context(), domainstorage.Providers{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(got, tc.HasLen, 0)
}

func (s *storagePoolServiceSuite) TestListStoragePoolsByProvidersInvalidProviders(c *tc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.service(c).ListStoragePoolsByProviders(c.Context(), domainstorage.Providers{"invalid"})
	c.Assert(err, tc.ErrorIs, coreerrors.NotFound)
	c.Assert(err, tc.ErrorMatches, `storage provider "invalid" not found`)
}

func (s *storagePoolServiceSuite) TestGetStoragePoolByName(c *tc.C) {
	defer s.setupMocks(c).Finish()

	sp := domainstorage.StoragePool{
		Name:     "ebs-fast",
		Provider: "ebs",
		Attrs: map[string]string{
			"foo": "foo val",
		},
	}
	s.state.EXPECT().GetStoragePoolByName(gomock.Any(), "ebs-fast").Return(sp, nil)

	got, err := s.service(c).GetStoragePoolByName(c.Context(), "ebs-fast")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(got, tc.DeepEquals, sp)
}
