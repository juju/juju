// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	domainstorage "github.com/juju/juju/domain/storage"
	storageerrors "github.com/juju/juju/domain/storage/errors"
	"github.com/juju/juju/internal/storage"
	dummystorage "github.com/juju/juju/internal/storage/provider/dummy"
)

type serviceSuite struct {
	testing.IsolationSuite

	state    *MockState
	registry storage.ProviderRegistry
}

var _ = gc.Suite(&serviceSuite{})

const validationError = errors.ConstError("missing attribute foo")

func (s *serviceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockState(ctrl)

	s.registry = storage.StaticProviderRegistry{
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
	}

	return ctrl
}

func (s *serviceSuite) service() *Service {
	return NewService(s.state, loggo.GetLogger("test"), s.registry)
}

func (s *serviceSuite) TestCreateStoragePool(c *gc.C) {
	defer s.setupMocks(c).Finish()

	sp := domainstorage.StoragePoolDetails{
		Name:     "ebs-fast",
		Provider: "ebs",
		Attrs: map[string]string{
			"foo": "foo val",
		},
	}
	s.state.EXPECT().CreateStoragePool(gomock.Any(), sp).Return(nil)

	err := s.service().CreateStoragePool(context.Background(), "ebs-fast", "ebs", PoolAttrs{"foo": "foo val"})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestCreateStoragePoolInvalidName(c *gc.C) {
	defer s.setupMocks(c).Finish()

	err := s.service().CreateStoragePool(context.Background(), "66invalid", "ebs", PoolAttrs{"foo": "foo val"})
	c.Assert(err, jc.ErrorIs, storageerrors.InvalidPoolNameError)
}

func (s *serviceSuite) TestCreateStoragePoolMissingName(c *gc.C) {
	defer s.setupMocks(c).Finish()

	err := s.service().CreateStoragePool(context.Background(), "", "ebs", PoolAttrs{"foo": "foo val"})
	c.Assert(err, jc.ErrorIs, storageerrors.MissingPoolNameError)
}

func (s *serviceSuite) TestCreateStoragePoolMissingType(c *gc.C) {
	defer s.setupMocks(c).Finish()

	err := s.service().CreateStoragePool(context.Background(), "ebs-fast", "", PoolAttrs{"foo": "foo val"})
	c.Assert(err, jc.ErrorIs, storageerrors.MissingPoolTypeError)
}

func (s *serviceSuite) TestCreateStoragePoolValidates(c *gc.C) {
	defer s.setupMocks(c).Finish()

	err := s.service().CreateStoragePool(context.Background(), "ebs-fast", "ebs", PoolAttrs{"bar": "bar val"})
	c.Assert(err, jc.ErrorIs, validationError)
	c.Assert(err, gc.ErrorMatches, `.* missing attribute foo`)
}

func (s *serviceSuite) TestDeleteStoragePool(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().DeleteStoragePool(gomock.Any(), "ebs-fast").Return(nil)

	err := s.service().DeleteStoragePool(context.Background(), "ebs-fast")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestReplaceStoragePool(c *gc.C) {
	defer s.setupMocks(c).Finish()

	sp := domainstorage.StoragePoolDetails{
		Name:     "ebs-fast",
		Provider: "ebs",
		Attrs: map[string]string{
			"foo": "foo val",
		},
	}
	s.state.EXPECT().ReplaceStoragePool(gomock.Any(), sp).Return(nil)

	err := s.service().ReplaceStoragePool(context.Background(), "ebs-fast", "ebs", PoolAttrs{"foo": "foo val"})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestReplaceStoragePoolInvalidName(c *gc.C) {
	defer s.setupMocks(c).Finish()

	err := s.service().ReplaceStoragePool(context.Background(), "66invalid", "ebs", PoolAttrs{"foo": "foo val"})
	c.Assert(err, jc.ErrorIs, storageerrors.InvalidPoolNameError)
}

func (s *serviceSuite) TestReplaceStoragePoolMissingName(c *gc.C) {
	defer s.setupMocks(c).Finish()

	err := s.service().ReplaceStoragePool(context.Background(), "", "ebs", PoolAttrs{"foo": "foo val"})
	c.Assert(err, jc.ErrorIs, storageerrors.MissingPoolNameError)
}

func (s *serviceSuite) TestReplaceStoragePoolExistingProvider(c *gc.C) {
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

	err := s.service().ReplaceStoragePool(context.Background(), "ebs-fast", "", PoolAttrs{"foo": "foo val"})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestReplaceStoragePoolValidates(c *gc.C) {
	defer s.setupMocks(c).Finish()

	err := s.service().ReplaceStoragePool(context.Background(), "ebs-fast", "ebs", PoolAttrs{"bar": "bar val"})
	c.Assert(err, jc.ErrorIs, validationError)
	c.Assert(err, gc.ErrorMatches, `.* missing attribute foo`)
}

func (s *serviceSuite) TestListStoragePoolsNoFilter(c *gc.C) {
	defer s.setupMocks(c).Finish()

	sp := domainstorage.StoragePoolDetails{
		Name:     "ebs-fast",
		Provider: "ebs",
		Attrs: map[string]string{
			"foo": "foo val",
		},
	}
	s.state.EXPECT().ListStoragePools(gomock.Any(), domainstorage.StoragePoolFilter{}).Return([]domainstorage.StoragePoolDetails{sp}, nil)

	got, err := s.service().ListStoragePools(context.Background(), domainstorage.StoragePoolFilter{})
	c.Assert(err, jc.ErrorIsNil)
	expected, err := storage.NewConfig("ebs-fast", "ebs", storage.Attrs{"foo": "foo val"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, jc.DeepEquals, []*storage.Config{expected})
}

func (s *serviceSuite) TestListStoragePoolsValidFilter(c *gc.C) {
	defer s.setupMocks(c).Finish()

	sp := domainstorage.StoragePoolDetails{
		Name:     "ebs-fast",
		Provider: "ebs",
		Attrs: map[string]string{
			"foo": "foo val",
		},
	}
	s.state.EXPECT().ListStoragePools(gomock.Any(), domainstorage.StoragePoolFilter{
		Names:     []string{"ebs-fast"},
		Providers: []string{"ebs"},
	}).Return([]domainstorage.StoragePoolDetails{sp}, nil)

	got, err := s.service().ListStoragePools(context.Background(), domainstorage.StoragePoolFilter{
		Names:     []string{"ebs-fast"},
		Providers: []string{"ebs"},
	})
	c.Assert(err, jc.ErrorIsNil)
	expected, err := storage.NewConfig("ebs-fast", "ebs", storage.Attrs{"foo": "foo val"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, jc.DeepEquals, []*storage.Config{expected})
}

func (s *serviceSuite) TestListStoragePoolsInvalidFilterName(c *gc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.service().ListStoragePools(context.Background(), domainstorage.StoragePoolFilter{
		Names: []string{"666invalid"},
	})
	c.Assert(err, jc.ErrorIs, storageerrors.InvalidPoolNameError)
	c.Assert(err, gc.ErrorMatches, `pool name "666invalid" not valid`)
}

func (s *serviceSuite) TestListStoragePoolsInvalidFilterProvider(c *gc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.service().ListStoragePools(context.Background(), domainstorage.StoragePoolFilter{
		Providers: []string{"loop"},
	})
	c.Assert(err, jc.ErrorIs, errors.NotFound)
	c.Assert(err, gc.ErrorMatches, `storage provider "loop" not found`)
}

func (s *serviceSuite) TestGetStoragePoolByName(c *gc.C) {
	defer s.setupMocks(c).Finish()

	sp := domainstorage.StoragePoolDetails{
		Name:     "ebs-fast",
		Provider: "ebs",
		Attrs: map[string]string{
			"foo": "foo val",
		},
	}
	s.state.EXPECT().GetStoragePoolByName(gomock.Any(), "ebs-fast").Return(sp, nil)

	got, err := s.service().GetStoragePoolByName(context.Background(), "ebs-fast")
	c.Assert(err, jc.ErrorIsNil)
	expected, err := storage.NewConfig("ebs-fast", "ebs", storage.Attrs{"foo": "foo val"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, jc.DeepEquals, expected)
}
