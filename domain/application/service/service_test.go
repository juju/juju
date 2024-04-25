// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/charm/v13"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/domain/application"
	domainstorage "github.com/juju/juju/domain/storage"
	storageerrors "github.com/juju/juju/domain/storage/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/storage/provider"
	dummystorage "github.com/juju/juju/internal/storage/provider/dummy"
)

type serviceSuite struct {
	testing.IsolationSuite

	state   *MockState
	charm   *MockCharm
	service *Service
}

var _ = gc.Suite(&serviceSuite{})

func (s *serviceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.state = NewMockState(ctrl)
	s.charm = NewMockCharm(ctrl)
	registry := storage.ChainedProviderRegistry{
		dummystorage.StorageProviders(),
		provider.CommonStorageProviders(),
	}
	s.service = NewService(s.state, loggertesting.WrapCheckLog(c), registry)

	return ctrl
}

func ptr[T any](v T) *T {
	return &v
}

func (s *serviceSuite) TestCreateApplication(c *gc.C) {
	defer s.setupMocks(c).Finish()

	u := application.AddUnitParams{
		UnitName: ptr("foo/666"),
	}
	s.state.EXPECT().StorageDefaults(gomock.Any()).Return(domainstorage.StorageDefaults{}, nil)
	s.state.EXPECT().UpsertApplication(gomock.Any(), "666", u).Return(nil)
	s.charm.EXPECT().Meta().Return(&charm.Meta{}).AnyTimes()

	a := AddUnitParams{
		UnitName: ptr("foo/666"),
	}
	err := s.service.CreateApplication(context.Background(), "666", AddApplicationParams{
		Charm: s.charm,
	}, a)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestCreateWithStorageBlock(c *gc.C) {
	defer s.setupMocks(c).Finish()

	u := application.AddUnitParams{
		UnitName: ptr("foo/666"),
	}
	s.state.EXPECT().StorageDefaults(gomock.Any()).Return(domainstorage.StorageDefaults{}, nil)
	s.state.EXPECT().UpsertApplication(gomock.Any(), "666", u).Return(nil)
	s.charm.EXPECT().Meta().Return(&charm.Meta{
		Storage: map[string]charm.Storage{
			"data": {
				Name:        "data",
				Type:        charm.StorageBlock,
				Shared:      false,
				CountMin:    1,
				CountMax:    2,
				MinimumSize: 10,
			},
		},
	}).AnyTimes()
	pool := domainstorage.StoragePoolDetails{Name: "loop", Provider: "loop"}
	s.state.EXPECT().GetStoragePoolByName(gomock.Any(), "loop").Return(pool, nil)

	a := AddUnitParams{
		UnitName: ptr("foo/666"),
	}
	err := s.service.CreateApplication(context.Background(), "666", AddApplicationParams{
		Charm: s.charm,
	}, a)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestCreateWithStorageBlockDefaultSource(c *gc.C) {
	defer s.setupMocks(c).Finish()

	u := application.AddUnitParams{
		UnitName: ptr("foo/666"),
	}
	s.state.EXPECT().StorageDefaults(gomock.Any()).Return(domainstorage.StorageDefaults{DefaultBlockSource: ptr("fast")}, nil)
	s.state.EXPECT().UpsertApplication(gomock.Any(), "666", u).Return(nil)
	s.charm.EXPECT().Meta().Return(&charm.Meta{
		Storage: map[string]charm.Storage{
			"data": {
				Name:        "data",
				Type:        charm.StorageBlock,
				Shared:      false,
				CountMin:    1,
				CountMax:    2,
				MinimumSize: 10,
			},
		},
	}).AnyTimes()
	pool := domainstorage.StoragePoolDetails{Name: "fast", Provider: "modelscoped-block"}
	s.state.EXPECT().GetStoragePoolByName(gomock.Any(), "fast").Return(pool, nil)

	a := AddUnitParams{
		UnitName: ptr("foo/666"),
	}
	err := s.service.CreateApplication(context.Background(), "666", AddApplicationParams{
		Charm: s.charm,
		Storage: map[string]storage.Directive{
			"data": {Count: 2},
		},
	}, a)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestCreateWithStorageFilesystem(c *gc.C) {
	defer s.setupMocks(c).Finish()

	u := application.AddUnitParams{
		UnitName: ptr("foo/666"),
	}
	s.state.EXPECT().StorageDefaults(gomock.Any()).Return(domainstorage.StorageDefaults{}, nil)
	s.state.EXPECT().UpsertApplication(gomock.Any(), "666", u).Return(nil)
	s.charm.EXPECT().Meta().Return(&charm.Meta{
		Storage: map[string]charm.Storage{
			"data": {
				Name:        "data",
				Type:        charm.StorageFilesystem,
				Shared:      false,
				CountMin:    1,
				CountMax:    2,
				MinimumSize: 10,
			},
		},
	}).AnyTimes()
	pool := domainstorage.StoragePoolDetails{Name: "rootfs", Provider: "rootfs"}
	s.state.EXPECT().GetStoragePoolByName(gomock.Any(), "rootfs").Return(pool, nil)

	a := AddUnitParams{
		UnitName: ptr("foo/666"),
	}
	err := s.service.CreateApplication(context.Background(), "666", AddApplicationParams{
		Charm: s.charm,
	}, a)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestCreateWithStorageFilesystemDefaultSource(c *gc.C) {
	defer s.setupMocks(c).Finish()

	u := application.AddUnitParams{
		UnitName: ptr("foo/666"),
	}
	s.state.EXPECT().StorageDefaults(gomock.Any()).Return(domainstorage.StorageDefaults{DefaultFilesystemSource: ptr("fast")}, nil)
	s.state.EXPECT().UpsertApplication(gomock.Any(), "666", u).Return(nil)
	s.charm.EXPECT().Meta().Return(&charm.Meta{
		Storage: map[string]charm.Storage{
			"data": {
				Name:        "data",
				Type:        charm.StorageFilesystem,
				CountMin:    1,
				CountMax:    2,
				MinimumSize: 10,
			},
		},
	}).AnyTimes()
	pool := domainstorage.StoragePoolDetails{Name: "fast", Provider: "modelscoped"}
	s.state.EXPECT().GetStoragePoolByName(gomock.Any(), "fast").Return(pool, nil)

	a := AddUnitParams{
		UnitName: ptr("foo/666"),
	}
	err := s.service.CreateApplication(context.Background(), "666", AddApplicationParams{
		Charm: s.charm,
		Storage: map[string]storage.Directive{
			"data": {Count: 2},
		},
	}, a)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestCreateWithSharedStorageMissingDirectives(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().StorageDefaults(gomock.Any()).Return(domainstorage.StorageDefaults{}, nil)
	s.charm.EXPECT().Meta().Return(&charm.Meta{
		Storage: map[string]charm.Storage{
			"data": {
				Name:   "data",
				Type:   charm.StorageBlock,
				Shared: true,
			},
		},
	}).AnyTimes()

	a := AddUnitParams{
		UnitName: ptr("foo/666"),
	}
	err := s.service.CreateApplication(context.Background(), "666", AddApplicationParams{
		Charm: s.charm,
	}, a)
	c.Assert(err, jc.ErrorIs, storageerrors.MissingSharedStorageDirectiveError)
	c.Assert(err, gc.ErrorMatches, `adding default storage directives: no storage directive specified for shared charm storage "data"`)
}

func (s *serviceSuite) TestCreateWithStorageValidates(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().StorageDefaults(gomock.Any()).Return(domainstorage.StorageDefaults{}, nil)
	s.state.EXPECT().GetStoragePoolByName(gomock.Any(), "loop").
		Return(domainstorage.StoragePoolDetails{}, storageerrors.PoolNotFoundError).MaxTimes(1)
	s.charm.EXPECT().Meta().Return(&charm.Meta{
		Name: "mine",
		Storage: map[string]charm.Storage{
			"data": {
				Name: "data",
				Type: charm.StorageBlock,
			},
		},
	}).AnyTimes()

	a := AddUnitParams{
		UnitName: ptr("foo/666"),
	}
	err := s.service.CreateApplication(context.Background(), "666", AddApplicationParams{
		Charm: s.charm,
		Storage: map[string]storage.Directive{
			"logs": {Count: 2},
		},
	}, a)
	c.Assert(err, gc.ErrorMatches, `invalid storage directives: charm "mine" has no store called "logs"`)
}

func (s *serviceSuite) TestCreateApplicationError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	rErr := errors.New("boom")
	s.state.EXPECT().StorageDefaults(gomock.Any()).Return(domainstorage.StorageDefaults{}, nil)
	s.state.EXPECT().UpsertApplication(gomock.Any(), "666").Return(rErr)
	s.charm.EXPECT().Meta().Return(&charm.Meta{}).AnyTimes()

	err := s.service.CreateApplication(context.Background(), "666", AddApplicationParams{
		Charm: s.charm,
	})
	c.Check(err, jc.ErrorIs, rErr)
	c.Assert(err, gc.ErrorMatches, `saving application "666": boom`)
}

func (s *serviceSuite) TestDeleteApplicationSuccess(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().DeleteApplication(gomock.Any(), "666").Return(nil)

	err := s.service.DeleteApplication(context.Background(), "666")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestDeleteApplicationError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	rErr := errors.New("boom")
	s.state.EXPECT().DeleteApplication(gomock.Any(), "666").Return(rErr)

	err := s.service.DeleteApplication(context.Background(), "666")
	c.Check(err, jc.ErrorIs, rErr)
	c.Assert(err, gc.ErrorMatches, `deleting application "666": boom`)
}

func (s *serviceSuite) TestAddUnits(c *gc.C) {
	defer s.setupMocks(c).Finish()

	u := application.AddUnitParams{
		UnitName: ptr("foo/666"),
	}
	s.state.EXPECT().AddUnits(gomock.Any(), "666", u).Return(nil)

	a := AddUnitParams{
		UnitName: ptr("foo/666"),
	}
	err := s.service.AddUnits(context.Background(), "666", a)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestAddUpsertCAASUnit(c *gc.C) {
	defer s.setupMocks(c).Finish()

	u := application.AddUnitParams{
		UnitName: ptr("foo/666"),
	}
	s.state.EXPECT().UpsertApplication(gomock.Any(), "foo", u).Return(nil)

	p := UpsertCAASUnitParams{
		UnitName: ptr("foo/666"),
	}
	err := s.service.UpsertCAASUnit(context.Background(), "foo", p)
	c.Assert(err, jc.ErrorIsNil)
}
