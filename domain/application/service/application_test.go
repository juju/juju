// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/caas"
	applicationtesting "github.com/juju/juju/core/application/testing"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/domain/application"
	domaincharm "github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	domainstorage "github.com/juju/juju/domain/storage"
	storageerrors "github.com/juju/juju/domain/storage/errors"
	"github.com/juju/juju/internal/charm"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/storage/provider"
	dummystorage "github.com/juju/juju/internal/storage/provider/dummy"
)

type applicationServiceSuite struct {
	testing.IsolationSuite

	state   *MockApplicationState
	charm   *MockCharm
	service *ApplicationService
	broker  *MockBroker
}

var _ = gc.Suite(&applicationServiceSuite{})

func (s *applicationServiceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.state = NewMockApplicationState(ctrl)
	s.charm = NewMockCharm(ctrl)
	s.broker = NewMockBroker(ctrl)
	registry := storage.ChainedProviderRegistry{
		dummystorage.StorageProviders(),
		provider.CommonStorageProviders(),
	}
	s.service = NewApplicationService(s.state, registry, loggertesting.WrapCheckLog(c))

	return ctrl
}

func ptr[T any](v T) *T {
	return &v
}

func (s *applicationServiceSuite) TestCreateApplication(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := applicationtesting.GenApplicationUUID(c)

	u := application.UpsertUnitArg{
		UnitName: ptr("foo/666"),
	}
	ch := domaincharm.Charm{
		Metadata: domaincharm.Metadata{
			Name:  "foo",
			RunAs: "default",
		},
	}
	platform := application.Platform{
		Channel:        "24.04",
		OSTypeID:       application.Ubuntu,
		ArchitectureID: application.ARM64,
	}
	app := application.AddApplicationArg{
		Charm:    ch,
		Platform: platform,
	}
	s.state.EXPECT().StorageDefaults(gomock.Any()).Return(domainstorage.StorageDefaults{}, nil)
	s.state.EXPECT().CreateApplication(gomock.Any(), "666", app, u).Return(id, nil)
	s.charm.EXPECT().Manifest().Return(&charm.Manifest{})
	s.charm.EXPECT().Actions().Return(&charm.Actions{})
	s.charm.EXPECT().Config().Return(&charm.Config{})
	s.charm.EXPECT().Meta().Return(&charm.Meta{
		Name: "foo",
	}).AnyTimes()

	a := AddUnitArg{
		UnitName: ptr("foo/666"),
	}
	_, err := s.service.CreateApplication(context.Background(), "666", s.charm, corecharm.Origin{
		Platform: corecharm.MustParsePlatform("arm64/ubuntu/24.04"),
	}, AddApplicationArgs{}, a)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *applicationServiceSuite) TestCreateWithStorageBlock(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := applicationtesting.GenApplicationUUID(c)

	u := application.UpsertUnitArg{
		UnitName: ptr("foo/666"),
	}
	ch := domaincharm.Charm{
		Metadata: domaincharm.Metadata{
			Name:  "foo",
			RunAs: "default",
			Storage: map[string]domaincharm.Storage{
				"data": {
					Name:        "data",
					Type:        domaincharm.StorageBlock,
					Shared:      false,
					CountMin:    1,
					CountMax:    2,
					MinimumSize: 10,
				},
			},
		},
	}
	platform := application.Platform{
		Channel:        "24.04",
		OSTypeID:       application.Ubuntu,
		ArchitectureID: application.ARM64,
	}
	app := application.AddApplicationArg{
		Charm:    ch,
		Platform: platform,
	}
	s.state.EXPECT().StorageDefaults(gomock.Any()).Return(domainstorage.StorageDefaults{}, nil)
	s.state.EXPECT().CreateApplication(gomock.Any(), "666", app, u).Return(id, nil)
	s.charm.EXPECT().Manifest().Return(&charm.Manifest{})
	s.charm.EXPECT().Actions().Return(&charm.Actions{})
	s.charm.EXPECT().Config().Return(&charm.Config{})
	s.charm.EXPECT().Meta().Return(&charm.Meta{
		Name: "foo",
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

	a := AddUnitArg{
		UnitName: ptr("foo/666"),
	}
	_, err := s.service.CreateApplication(context.Background(), "666", s.charm, corecharm.Origin{
		Platform: corecharm.MustParsePlatform("arm64/ubuntu/24.04"),
	}, AddApplicationArgs{}, a)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *applicationServiceSuite) TestCreateWithStorageBlockDefaultSource(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := applicationtesting.GenApplicationUUID(c)

	u := application.UpsertUnitArg{
		UnitName: ptr("foo/666"),
	}
	ch := domaincharm.Charm{
		Metadata: domaincharm.Metadata{
			Name:  "foo",
			RunAs: "default",
			Storage: map[string]domaincharm.Storage{
				"data": {
					Name:        "data",
					Type:        domaincharm.StorageBlock,
					Shared:      false,
					CountMin:    1,
					CountMax:    2,
					MinimumSize: 10,
				},
			},
		},
	}
	platform := application.Platform{
		Channel:        "24.04",
		OSTypeID:       application.Ubuntu,
		ArchitectureID: application.ARM64,
	}
	app := application.AddApplicationArg{
		Charm:    ch,
		Platform: platform,
	}
	s.state.EXPECT().StorageDefaults(gomock.Any()).Return(domainstorage.StorageDefaults{DefaultBlockSource: ptr("fast")}, nil)
	s.state.EXPECT().CreateApplication(gomock.Any(), "666", app, u).Return(id, nil)
	s.charm.EXPECT().Manifest().Return(&charm.Manifest{})
	s.charm.EXPECT().Actions().Return(&charm.Actions{})
	s.charm.EXPECT().Config().Return(&charm.Config{})
	s.charm.EXPECT().Meta().Return(&charm.Meta{
		Name: "foo",
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

	a := AddUnitArg{
		UnitName: ptr("foo/666"),
	}
	_, err := s.service.CreateApplication(context.Background(), "666", s.charm, corecharm.Origin{
		Platform: corecharm.MustParsePlatform("arm64/ubuntu/24.04"),
	}, AddApplicationArgs{
		Storage: map[string]storage.Directive{
			"data": {Count: 2},
		},
	}, a)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *applicationServiceSuite) TestCreateWithStorageFilesystem(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := applicationtesting.GenApplicationUUID(c)

	u := application.UpsertUnitArg{
		UnitName: ptr("foo/666"),
	}
	ch := domaincharm.Charm{
		Metadata: domaincharm.Metadata{
			Name:  "foo",
			RunAs: "default",
			Storage: map[string]domaincharm.Storage{
				"data": {
					Name:        "data",
					Type:        domaincharm.StorageFilesystem,
					Shared:      false,
					CountMin:    1,
					CountMax:    2,
					MinimumSize: 10,
				},
			},
		},
	}
	platform := application.Platform{
		Channel:        "24.04",
		OSTypeID:       application.Ubuntu,
		ArchitectureID: application.ARM64,
	}
	app := application.AddApplicationArg{
		Charm:    ch,
		Platform: platform,
	}
	s.state.EXPECT().StorageDefaults(gomock.Any()).Return(domainstorage.StorageDefaults{}, nil)
	s.state.EXPECT().CreateApplication(gomock.Any(), "666", app, u).Return(id, nil)
	s.charm.EXPECT().Manifest().Return(&charm.Manifest{})
	s.charm.EXPECT().Actions().Return(&charm.Actions{})
	s.charm.EXPECT().Config().Return(&charm.Config{})
	s.charm.EXPECT().Meta().Return(&charm.Meta{
		Name: "foo",
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

	a := AddUnitArg{
		UnitName: ptr("foo/666"),
	}
	_, err := s.service.CreateApplication(context.Background(), "666", s.charm, corecharm.Origin{
		Platform: corecharm.MustParsePlatform("arm64/ubuntu/24.04"),
	}, AddApplicationArgs{}, a)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *applicationServiceSuite) TestCreateWithStorageFilesystemDefaultSource(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := applicationtesting.GenApplicationUUID(c)

	u := application.UpsertUnitArg{
		UnitName: ptr("foo/666"),
	}
	ch := domaincharm.Charm{
		Metadata: domaincharm.Metadata{
			Name:  "foo",
			RunAs: "default",
			Storage: map[string]domaincharm.Storage{
				"data": {
					Name:        "data",
					Type:        domaincharm.StorageFilesystem,
					Shared:      false,
					CountMin:    1,
					CountMax:    2,
					MinimumSize: 10,
				},
			},
		},
	}
	platform := application.Platform{
		Channel:        "24.04",
		OSTypeID:       application.Ubuntu,
		ArchitectureID: application.ARM64,
	}
	app := application.AddApplicationArg{
		Charm:    ch,
		Platform: platform,
	}
	s.state.EXPECT().StorageDefaults(gomock.Any()).Return(domainstorage.StorageDefaults{DefaultFilesystemSource: ptr("fast")}, nil)
	s.state.EXPECT().CreateApplication(gomock.Any(), "666", app, u).Return(id, nil)
	s.charm.EXPECT().Manifest().Return(&charm.Manifest{})
	s.charm.EXPECT().Actions().Return(&charm.Actions{})
	s.charm.EXPECT().Config().Return(&charm.Config{})
	s.charm.EXPECT().Meta().Return(&charm.Meta{
		Name: "foo",
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
	pool := domainstorage.StoragePoolDetails{Name: "fast", Provider: "modelscoped"}
	s.state.EXPECT().GetStoragePoolByName(gomock.Any(), "fast").Return(pool, nil)

	a := AddUnitArg{
		UnitName: ptr("foo/666"),
	}
	_, err := s.service.CreateApplication(context.Background(), "666", s.charm, corecharm.Origin{
		Platform: corecharm.MustParsePlatform("arm64/ubuntu/24.04"),
	}, AddApplicationArgs{
		Storage: map[string]storage.Directive{
			"data": {Count: 2},
		},
	}, a)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *applicationServiceSuite) TestCreateWithSharedStorageMissingDirectives(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().StorageDefaults(gomock.Any()).Return(domainstorage.StorageDefaults{}, nil)
	s.charm.EXPECT().Meta().Return(&charm.Meta{
		Name: "foo",
		Storage: map[string]charm.Storage{
			"data": {
				Name:   "data",
				Type:   charm.StorageBlock,
				Shared: true,
			},
		},
	}).AnyTimes()

	a := AddUnitArg{
		UnitName: ptr("foo/666"),
	}
	_, err := s.service.CreateApplication(context.Background(), "666", s.charm, corecharm.Origin{
		Platform: corecharm.MustParsePlatform("arm64/ubuntu/24.04"),
	}, AddApplicationArgs{}, a)
	c.Assert(err, jc.ErrorIs, storageerrors.MissingSharedStorageDirectiveError)
	c.Assert(err, gc.ErrorMatches, `adding default storage directives: no storage directive specified for shared charm storage "data"`)
}

func (s *applicationServiceSuite) TestCreateWithStorageValidates(c *gc.C) {
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

	a := AddUnitArg{
		UnitName: ptr("foo/666"),
	}
	_, err := s.service.CreateApplication(context.Background(), "666", s.charm, corecharm.Origin{
		Platform: corecharm.MustParsePlatform("arm64/ubuntu/24.04"),
	}, AddApplicationArgs{
		Storage: map[string]storage.Directive{
			"logs": {Count: 2},
		},
	}, a)
	c.Assert(err, gc.ErrorMatches, `invalid storage directives: charm "mine" has no store called "logs"`)
}

func (s *applicationServiceSuite) TestCreateApplicationWithNoCharmName(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.charm.EXPECT().Meta().Return(&charm.Meta{}).AnyTimes()

	_, err := s.service.CreateApplication(context.Background(), "666", s.charm, corecharm.Origin{
		Platform: corecharm.MustParsePlatform("arm64/ubuntu/24.04"),
	}, AddApplicationArgs{})
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmNameNotValid)
}

func (s *applicationServiceSuite) TestCreateApplicationWithNoApplicationOrCharmName(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.charm.EXPECT().Meta().Return(&charm.Meta{}).AnyTimes()

	_, err := s.service.CreateApplication(context.Background(), "", s.charm, corecharm.Origin{
		Platform: corecharm.MustParsePlatform("arm64/ubuntu/24.04"),
	}, AddApplicationArgs{})
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationNameNotValid)
}

func (s *applicationServiceSuite) TestCreateApplicationWithNoMeta(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.charm.EXPECT().Meta().Return(nil).AnyTimes()

	_, err := s.service.CreateApplication(context.Background(), "666", s.charm, corecharm.Origin{
		Platform: corecharm.MustParsePlatform("arm64/ubuntu/24.04"),
	}, AddApplicationArgs{})
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmMetadataNotValid)
}

func (s *applicationServiceSuite) TestCreateApplicationWithNoArchitecture(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.charm.EXPECT().Meta().Return(&charm.Meta{Name: "foo"}).AnyTimes()

	_, err := s.service.CreateApplication(context.Background(), "666", s.charm, corecharm.Origin{
		Source:   corecharm.CharmHub,
		Platform: corecharm.Platform{Channel: "24.04", OS: "ubuntu"},
	}, AddApplicationArgs{})
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmOriginNotValid)
}

func (s *applicationServiceSuite) TestCreateApplicationError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := applicationtesting.GenApplicationUUID(c)

	rErr := errors.New("boom")
	s.state.EXPECT().StorageDefaults(gomock.Any()).Return(domainstorage.StorageDefaults{}, nil)
	s.state.EXPECT().CreateApplication(gomock.Any(), "666", gomock.Any()).Return(id, rErr)
	s.charm.EXPECT().Manifest().Return(&charm.Manifest{})
	s.charm.EXPECT().Actions().Return(&charm.Actions{})
	s.charm.EXPECT().Config().Return(&charm.Config{})
	s.charm.EXPECT().Meta().Return(&charm.Meta{
		Name: "foo",
	}).AnyTimes()

	_, err := s.service.CreateApplication(context.Background(), "666", s.charm, corecharm.Origin{
		Platform: corecharm.MustParsePlatform("arm64/ubuntu/24.04"),
	}, AddApplicationArgs{})
	c.Check(err, jc.ErrorIs, rErr)
	c.Assert(err, gc.ErrorMatches, `creating application "666": boom`)
}

func (s *applicationServiceSuite) TestDeleteApplicationSuccess(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().DeleteApplication(gomock.Any(), "666").Return(nil)

	err := s.service.DeleteApplication(context.Background(), "666")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *applicationServiceSuite) TestDeleteApplicationError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	rErr := errors.New("boom")
	s.state.EXPECT().DeleteApplication(gomock.Any(), "666").Return(rErr)

	err := s.service.DeleteApplication(context.Background(), "666")
	c.Check(err, jc.ErrorIs, rErr)
	c.Assert(err, gc.ErrorMatches, `deleting application "666": boom`)
}

func (s *applicationServiceSuite) TestAddUnits(c *gc.C) {
	defer s.setupMocks(c).Finish()

	u := application.UpsertUnitArg{
		UnitName: ptr("foo/666"),
	}
	s.state.EXPECT().AddUnits(gomock.Any(), "666", u).Return(nil)

	a := AddUnitArg{
		UnitName: ptr("foo/666"),
	}
	err := s.service.AddUnits(context.Background(), "666", a)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *applicationServiceSuite) TestRegisterCAASUnit(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().ExecuteTxnOperation(gomock.Any(), "foo", false, gomock.Any()).Return(nil)

	p := RegisterCAASUnitParams{
		UnitName:     "foo/666",
		PasswordHash: ptr("passwordhash"),
		ProviderId:   ptr("provider-id"),
		OrderedScale: true,
		OrderedId:    1,
	}
	err := s.service.RegisterCAASUnit(context.Background(), "foo", p)
	c.Assert(err, jc.ErrorIsNil)
}

var unitParams = RegisterCAASUnitParams{
	UnitName:     "foo/666",
	PasswordHash: ptr("passwordhash"),
	ProviderId:   ptr("provider-id"),
	OrderedScale: true,
	OrderedId:    1,
}

func (s *applicationServiceSuite) TestRegisterCAASUnitMissingUnitName(c *gc.C) {
	defer s.setupMocks(c).Finish()

	p := unitParams
	p.UnitName = ""
	err := s.service.RegisterCAASUnit(context.Background(), "foo", p)
	c.Assert(err, gc.ErrorMatches, "missing unit name not valid")
}

func (s *applicationServiceSuite) TestRegisterCAASUnitMissingOrderedScale(c *gc.C) {
	defer s.setupMocks(c).Finish()

	p := unitParams
	p.OrderedScale = false
	err := s.service.RegisterCAASUnit(context.Background(), "foo", p)
	c.Assert(err, gc.ErrorMatches, "registering CAAS units not supported without ordered unit IDs")
}

func (s *applicationServiceSuite) TestRegisterCAASUnitMissingProviderID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	p := unitParams
	p.ProviderId = nil
	err := s.service.RegisterCAASUnit(context.Background(), "foo", p)
	c.Assert(err, gc.ErrorMatches, "provider id not valid")
}

func (s *applicationServiceSuite) TestRegisterCAASUnitMissingPasswordHash(c *gc.C) {
	defer s.setupMocks(c).Finish()

	p := unitParams
	p.PasswordHash = nil
	err := s.service.RegisterCAASUnit(context.Background(), "foo", p)
	c.Assert(err, gc.ErrorMatches, "password hash not valid")
}

func (s *applicationServiceSuite) TestCAASUnitTerminatingUnitNumLessThanScale(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	app := NewMockApplication(ctrl)
	app.EXPECT().State().Return(caas.ApplicationState{
		DesiredReplicas: 6,
	}, nil)
	s.broker.EXPECT().Application("foo", caas.DeploymentStateful).Return(app)

	s.state.EXPECT().ApplicationScaleState(gomock.Any(), "foo").Return(application.ScaleState{
		Scale: 2,
	}, nil)

	willRestart, err := s.service.CAASUnitTerminating(context.Background(), "foo", 1, s.broker)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(willRestart, jc.IsTrue)
}

func (s *applicationServiceSuite) TestCAASUnitTerminatingUnitNumGreaterThanScale(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	app := NewMockApplication(ctrl)
	app.EXPECT().State().Return(caas.ApplicationState{
		DesiredReplicas: 6,
	}, nil)
	s.broker.EXPECT().Application("foo", caas.DeploymentStateful).Return(app)

	s.state.EXPECT().ApplicationScaleState(gomock.Any(), "foo").Return(application.ScaleState{
		Scale: 2,
	}, nil)

	willRestart, err := s.service.CAASUnitTerminating(context.Background(), "foo", 4, s.broker)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(willRestart, jc.IsFalse)
}

func (s *applicationServiceSuite) TestCAASUnitTerminatingUnitNumLessThanDesired(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	app := NewMockApplication(ctrl)
	app.EXPECT().State().Return(caas.ApplicationState{
		DesiredReplicas: 6,
	}, nil)
	s.broker.EXPECT().Application("foo", caas.DeploymentStateful).Return(app)

	s.state.EXPECT().ApplicationScaleState(gomock.Any(), "foo").Return(application.ScaleState{
		Scale: 6,
	}, nil)

	willRestart, err := s.service.CAASUnitTerminating(context.Background(), "foo", 3, s.broker)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(willRestart, jc.IsTrue)
}

func (s *applicationServiceSuite) TestCAASUnitTerminatingUnitNumGreaterThanDesired(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	app := NewMockApplication(ctrl)
	app.EXPECT().State().Return(caas.ApplicationState{
		DesiredReplicas: 2,
	}, nil)
	s.broker.EXPECT().Application("foo", caas.DeploymentStateful).Return(app)

	s.state.EXPECT().ApplicationScaleState(gomock.Any(), "foo").Return(application.ScaleState{
		Scale: 6,
	}, nil)

	willRestart, err := s.service.CAASUnitTerminating(context.Background(), "foo", 3, s.broker)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(willRestart, jc.IsFalse)
}

func (s *applicationServiceSuite) TestGetScalingState(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().ApplicationScaleState(gomock.Any(), "foo").Return(application.ScaleState{
		Scaling:     true,
		ScaleTarget: 666,
	}, nil)

	got, err := s.service.GetScalingState(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, jc.DeepEquals, ScalingState{
		Scaling:     true,
		ScaleTarget: 666,
	})
}

func (s *applicationServiceSuite) TestGetScale(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().ApplicationScaleState(gomock.Any(), "foo").Return(application.ScaleState{
		Scale: 666,
	}, nil)

	got, err := s.service.GetScale(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, gc.Equals, 666)
}

func (s *applicationServiceSuite) TestSetScaleInvalid(c *gc.C) {
	defer s.setupMocks(c).Finish()

	err := s.service.SetScale(context.Background(), "foo", -1, false)
	c.Assert(err, jc.ErrorIs, applicationerrors.ScaleChangeInvalid)
}
