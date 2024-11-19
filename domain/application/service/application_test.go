// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"math/rand/v2"
	"time"

	"github.com/juju/clock/testclock"
	jujuerrors "github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	coreapplication "github.com/juju/juju/core/application"
	applicationtesting "github.com/juju/juju/core/application/testing"
	"github.com/juju/juju/core/assumes"
	"github.com/juju/juju/core/changestream"
	corecharm "github.com/juju/juju/core/charm"
	charmtesting "github.com/juju/juju/core/charm/testing"
	"github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	corestorage "github.com/juju/juju/core/storage"
	coreunit "github.com/juju/juju/core/unit"
	unittesting "github.com/juju/juju/core/unit/testing"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/application"
	domaincharm "github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/life"
	domainstorage "github.com/juju/juju/domain/storage"
	storageerrors "github.com/juju/juju/domain/storage/errors"
	domaintesting "github.com/juju/juju/domain/testing"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/storage/provider"
	dummystorage "github.com/juju/juju/internal/storage/provider/dummy"
	internaltesting "github.com/juju/juju/internal/testing"
)

type applicationServiceSuite struct {
	testing.IsolationSuite

	service *ApplicationService

	state  *MockApplicationState
	charm  *MockCharm
	secret *MockDeleteSecretState
	clock  *testclock.Clock
}

var _ = gc.Suite(&applicationServiceSuite{})

func (s *applicationServiceSuite) TestImportApplication(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := applicationtesting.GenApplicationUUID(c)

	u := application.InsertUnitArg{
		UnitName:       "ubuntu/666",
		CloudContainer: nil,
		Password: ptr(application.PasswordInfo{
			PasswordHash:  "passwordhash",
			HashAlgorithm: 0,
		}),
		UnitStatusArg: application.UnitStatusArg{
			AgentStatus: application.UnitAgentStatusInfo{
				StatusID: 2,
				StatusInfo: application.StatusInfo{
					Message: "agent status",
					Data:    map[string]string{"foo": "bar"},
					Since:   s.clock.Now(),
				},
			},
			WorkloadStatus: application.UnitWorkloadStatusInfo{
				StatusID: 3,
				StatusInfo: application.StatusInfo{
					Message: "workload status",
					Data:    map[string]string{"foo": "bar"},
					Since:   s.clock.Now(),
				},
			},
		},
	}
	ch := domaincharm.Charm{
		Metadata: domaincharm.Metadata{
			Name:  "ubuntu",
			RunAs: "default",
		},
	}
	platform := application.Platform{
		Channel:      "24.04",
		OSType:       domaincharm.Ubuntu,
		Architecture: domaincharm.ARM64,
	}
	app := application.AddApplicationArg{
		Charm:    ch,
		Platform: platform,
		Origin: domaincharm.CharmOrigin{
			ReferenceName: "ubuntu",
			Source:        domaincharm.CharmHubSource,
			Revision:      42,
		},
		Scale: 1,
	}
	s.state.EXPECT().GetModelType(gomock.Any()).Return("iaas", nil)
	s.state.EXPECT().StorageDefaults(gomock.Any()).Return(domainstorage.StorageDefaults{}, nil)
	s.state.EXPECT().CreateApplication(domaintesting.IsAtomicContextChecker, "ubuntu", app).Return(id, nil)
	s.state.EXPECT().InsertUnit(domaintesting.IsAtomicContextChecker, id, u)
	s.charm.EXPECT().Manifest().Return(&charm.Manifest{})
	s.charm.EXPECT().Actions().Return(&charm.Actions{})
	s.charm.EXPECT().Config().Return(&charm.Config{})
	s.charm.EXPECT().Meta().Return(&charm.Meta{
		Name: "ubuntu",
	}).AnyTimes()

	err := s.service.ImportApplication(context.Background(), "ubuntu", s.charm, corecharm.Origin{
		Source:   corecharm.CharmHub,
		Platform: corecharm.MustParsePlatform("arm64/ubuntu/24.04"),
		Revision: ptr(42),
	}, AddApplicationArgs{
		ReferenceName: "ubuntu",
	}, ImportUnitArg{
		UnitName:     "ubuntu/666",
		PasswordHash: ptr("passwordhash"),
		AgentStatus: StatusParams{
			Status:  "idle",
			Message: "agent status",
			Data:    map[string]any{"foo": "bar"},
			Since:   ptr(s.clock.Now()),
		},
		WorkloadStatus: StatusParams{
			Status:  "waiting",
			Message: "workload status",
			Data:    map[string]any{"foo": "bar"},
			Since:   ptr(s.clock.Now()),
		},
		CloudContainer: nil,
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *applicationServiceSuite) TestCreateApplication(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := applicationtesting.GenApplicationUUID(c)

	u := application.AddUnitArg{
		UnitName: "ubuntu/666",
		UnitStatusArg: application.UnitStatusArg{
			AgentStatus: application.UnitAgentStatusInfo{
				StatusID: application.UnitAgentStatusAllocating,
				StatusInfo: application.StatusInfo{
					Since: s.clock.Now(),
				},
			},
			WorkloadStatus: application.UnitWorkloadStatusInfo{
				StatusID: application.UnitWorkloadStatusWaiting,
				StatusInfo: application.StatusInfo{
					Message: "installing agent",
					Since:   s.clock.Now(),
				},
			},
		},
	}
	ch := domaincharm.Charm{
		Metadata: domaincharm.Metadata{
			Name:  "ubuntu",
			RunAs: "default",
		},
	}
	platform := application.Platform{
		Channel:      "24.04",
		OSType:       domaincharm.Ubuntu,
		Architecture: domaincharm.ARM64,
	}
	app := application.AddApplicationArg{
		Charm:    ch,
		Platform: platform,
		Scale:    1,
		Origin: domaincharm.CharmOrigin{
			ReferenceName: "ubuntu",
			Source:        domaincharm.CharmHubSource,
			Revision:      42,
		},
	}
	s.state.EXPECT().GetModelType(gomock.Any()).Return("caas", nil)
	s.state.EXPECT().StorageDefaults(gomock.Any()).Return(domainstorage.StorageDefaults{}, nil)
	s.state.EXPECT().CreateApplication(domaintesting.IsAtomicContextChecker, "ubuntu", app).Return(id, nil)
	s.state.EXPECT().AddUnits(domaintesting.IsAtomicContextChecker, id, u)
	s.charm.EXPECT().Manifest().Return(&charm.Manifest{})
	s.charm.EXPECT().Actions().Return(&charm.Actions{})
	s.charm.EXPECT().Config().Return(&charm.Config{})
	s.charm.EXPECT().Meta().Return(&charm.Meta{
		Name: "ubuntu",
	}).AnyTimes()

	a := AddUnitArg{
		UnitName: "ubuntu/666",
	}
	_, err := s.service.CreateApplication(context.Background(), "ubuntu", s.charm, corecharm.Origin{
		Source:   corecharm.CharmHub,
		Platform: corecharm.MustParsePlatform("arm64/ubuntu/24.04"),
		Revision: ptr(42),
	}, AddApplicationArgs{
		ReferenceName: "ubuntu",
	}, a)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *applicationServiceSuite) TestCreateApplicationWithInvalidApplicationName(c *gc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.service.CreateApplication(context.Background(), "666", s.charm, corecharm.Origin{
		Source:   corecharm.CharmHub,
		Platform: corecharm.MustParsePlatform("arm64/ubuntu/24.04"),
		Revision: ptr(42),
	}, AddApplicationArgs{
		ReferenceName: "ubuntu",
	})
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationNameNotValid)
}

func (s *applicationServiceSuite) TestCreateApplicationWithInvalidCharmName(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.charm.EXPECT().Meta().Return(&charm.Meta{
		Name: "666",
	}).AnyTimes()

	_, err := s.service.CreateApplication(context.Background(), "ubuntu", s.charm, corecharm.Origin{
		Source:   corecharm.CharmHub,
		Platform: corecharm.MustParsePlatform("arm64/ubuntu/24.04"),
		Revision: ptr(42),
	}, AddApplicationArgs{
		ReferenceName: "ubuntu",
	})
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmNameNotValid)
}

func (s *applicationServiceSuite) TestCreateApplicationWithInvalidReferenceName(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.charm.EXPECT().Meta().Return(&charm.Meta{
		Name: "ubuntu",
	}).AnyTimes()

	_, err := s.service.CreateApplication(context.Background(), "ubuntu", s.charm, corecharm.Origin{
		Source:   corecharm.CharmHub,
		Platform: corecharm.MustParsePlatform("arm64/ubuntu/24.04"),
		Revision: ptr(42),
	}, AddApplicationArgs{
		ReferenceName: "666",
	})
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmNameNotValid)
}

func (s *applicationServiceSuite) TestCreateApplicationWithNoCharmName(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.charm.EXPECT().Meta().Return(&charm.Meta{}).AnyTimes()

	_, err := s.service.CreateApplication(context.Background(), "foo", s.charm, corecharm.Origin{
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

	_, err := s.service.CreateApplication(context.Background(), "foo", s.charm, corecharm.Origin{
		Platform: corecharm.MustParsePlatform("arm64/ubuntu/24.04"),
	}, AddApplicationArgs{})
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmMetadataNotValid)
}

func (s *applicationServiceSuite) TestCreateApplicationWithNoArchitecture(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.charm.EXPECT().Meta().Return(&charm.Meta{Name: "foo"}).AnyTimes()

	_, err := s.service.CreateApplication(context.Background(), "foo", s.charm, corecharm.Origin{
		Source:   corecharm.CharmHub,
		Platform: corecharm.Platform{Channel: "24.04", OS: "ubuntu"},
	}, AddApplicationArgs{
		ReferenceName: "foo",
	})
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmOriginNotValid)
}

func (s *applicationServiceSuite) TestCreateApplicationError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := applicationtesting.GenApplicationUUID(c)

	rErr := errors.New("boom")
	s.state.EXPECT().GetModelType(gomock.Any()).Return("caas", nil)
	s.state.EXPECT().StorageDefaults(gomock.Any()).Return(domainstorage.StorageDefaults{}, nil)
	s.state.EXPECT().CreateApplication(domaintesting.IsAtomicContextChecker, "foo", gomock.Any()).Return(id, rErr)
	s.charm.EXPECT().Manifest().Return(&charm.Manifest{})
	s.charm.EXPECT().Actions().Return(&charm.Actions{})
	s.charm.EXPECT().Config().Return(&charm.Config{})
	s.charm.EXPECT().Meta().Return(&charm.Meta{
		Name: "foo",
	}).AnyTimes()

	_, err := s.service.CreateApplication(context.Background(), "foo", s.charm, corecharm.Origin{
		Source:   corecharm.CharmHub,
		Platform: corecharm.MustParsePlatform("arm64/ubuntu/24.04"),
	}, AddApplicationArgs{
		ReferenceName: "foo",
	})
	c.Check(err, jc.ErrorIs, rErr)
	c.Assert(err, gc.ErrorMatches, `creating application "foo": boom`)
}

func (s *applicationServiceSuite) TestCreateWithStorageBlock(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := applicationtesting.GenApplicationUUID(c)

	u := application.AddUnitArg{
		UnitName: "ubuntu/666",
		UnitStatusArg: application.UnitStatusArg{
			AgentStatus: application.UnitAgentStatusInfo{
				StatusID: application.UnitAgentStatusAllocating,
				StatusInfo: application.StatusInfo{
					Since: s.clock.Now(),
				},
			},
			WorkloadStatus: application.UnitWorkloadStatusInfo{
				StatusID: application.UnitWorkloadStatusWaiting,
				StatusInfo: application.StatusInfo{
					Message: "waiting for machine",
					Since:   s.clock.Now(),
				},
			},
		},
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
		Channel:      "24.04",
		OSType:       domaincharm.Ubuntu,
		Architecture: domaincharm.ARM64,
	}
	app := application.AddApplicationArg{
		Charm:    ch,
		Platform: platform,
		Origin: domaincharm.CharmOrigin{
			ReferenceName: "foo",
			Source:        domaincharm.LocalSource,
			Revision:      42,
		},
		Scale: 1,
	}
	s.state.EXPECT().GetModelType(gomock.Any()).Return("iaas", nil)
	s.state.EXPECT().StorageDefaults(gomock.Any()).Return(domainstorage.StorageDefaults{}, nil)
	s.state.EXPECT().CreateApplication(domaintesting.IsAtomicContextChecker, "foo", app).Return(id, nil)
	s.state.EXPECT().AddUnits(domaintesting.IsAtomicContextChecker, id, u)
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
		UnitName: "ubuntu/666",
	}
	_, err := s.service.CreateApplication(context.Background(), "foo", s.charm, corecharm.Origin{
		Source:   corecharm.Local,
		Platform: corecharm.MustParsePlatform("arm64/ubuntu/24.04"),
		Revision: ptr(42),
	}, AddApplicationArgs{
		ReferenceName: "foo",
	}, a)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *applicationServiceSuite) TestCreateWithStorageBlockDefaultSource(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := applicationtesting.GenApplicationUUID(c)

	u := application.AddUnitArg{
		UnitName: "ubuntu/666",
		UnitStatusArg: application.UnitStatusArg{
			AgentStatus: application.UnitAgentStatusInfo{
				StatusID: application.UnitAgentStatusAllocating,
				StatusInfo: application.StatusInfo{
					Since: s.clock.Now(),
				},
			},
			WorkloadStatus: application.UnitWorkloadStatusInfo{
				StatusID: application.UnitWorkloadStatusWaiting,
				StatusInfo: application.StatusInfo{
					Message: "waiting for machine",
					Since:   s.clock.Now(),
				},
			},
		},
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
		Channel:      "24.04",
		OSType:       domaincharm.Ubuntu,
		Architecture: domaincharm.ARM64,
	}
	app := application.AddApplicationArg{
		Charm:    ch,
		Platform: platform,
		Origin: domaincharm.CharmOrigin{
			ReferenceName: "foo",
			Source:        domaincharm.CharmHubSource,
			Revision:      42,
		},
		Scale: 1,
	}
	s.state.EXPECT().GetModelType(gomock.Any()).Return("iaas", nil)
	s.state.EXPECT().StorageDefaults(gomock.Any()).Return(domainstorage.StorageDefaults{DefaultBlockSource: ptr("fast")}, nil)
	s.state.EXPECT().CreateApplication(domaintesting.IsAtomicContextChecker, "foo", app).Return(id, nil)
	s.state.EXPECT().AddUnits(domaintesting.IsAtomicContextChecker, id, u)
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
		UnitName: "ubuntu/666",
	}
	_, err := s.service.CreateApplication(context.Background(), "foo", s.charm, corecharm.Origin{
		Source:   corecharm.CharmHub,
		Platform: corecharm.MustParsePlatform("arm64/ubuntu/24.04"),
		Revision: ptr(42),
	}, AddApplicationArgs{
		ReferenceName: "foo",
		Storage: map[string]storage.Directive{
			"data": {Count: 2},
		},
	}, a)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *applicationServiceSuite) TestCreateWithStorageFilesystem(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := applicationtesting.GenApplicationUUID(c)

	u := application.AddUnitArg{
		UnitName: "ubuntu/666",
		UnitStatusArg: application.UnitStatusArg{
			AgentStatus: application.UnitAgentStatusInfo{
				StatusID: application.UnitAgentStatusAllocating,
				StatusInfo: application.StatusInfo{
					Since: s.clock.Now(),
				},
			},
			WorkloadStatus: application.UnitWorkloadStatusInfo{
				StatusID: application.UnitWorkloadStatusWaiting,
				StatusInfo: application.StatusInfo{
					Message: "waiting for machine",
					Since:   s.clock.Now(),
				},
			},
		},
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
		Channel:      "24.04",
		OSType:       domaincharm.Ubuntu,
		Architecture: domaincharm.ARM64,
	}
	app := application.AddApplicationArg{
		Charm:    ch,
		Platform: platform,
		Origin: domaincharm.CharmOrigin{
			ReferenceName: "foo",
			Source:        domaincharm.CharmHubSource,
			Revision:      42,
		},
		Scale: 1,
	}
	s.state.EXPECT().GetModelType(gomock.Any()).Return("iaas", nil)
	s.state.EXPECT().StorageDefaults(gomock.Any()).Return(domainstorage.StorageDefaults{}, nil)
	s.state.EXPECT().CreateApplication(domaintesting.IsAtomicContextChecker, "foo", app).Return(id, nil)
	s.state.EXPECT().AddUnits(domaintesting.IsAtomicContextChecker, id, u)
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
		UnitName: "ubuntu/666",
	}
	_, err := s.service.CreateApplication(context.Background(), "foo", s.charm, corecharm.Origin{
		Source:   corecharm.CharmHub,
		Platform: corecharm.MustParsePlatform("arm64/ubuntu/24.04"),
		Revision: ptr(42),
	}, AddApplicationArgs{
		ReferenceName: "foo",
	}, a)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *applicationServiceSuite) TestCreateWithStorageFilesystemDefaultSource(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := applicationtesting.GenApplicationUUID(c)

	u := application.AddUnitArg{
		UnitName: "ubuntu/666",
		UnitStatusArg: application.UnitStatusArg{
			AgentStatus: application.UnitAgentStatusInfo{
				StatusID: application.UnitAgentStatusAllocating,
				StatusInfo: application.StatusInfo{
					Since: s.clock.Now(),
				},
			},
			WorkloadStatus: application.UnitWorkloadStatusInfo{
				StatusID: application.UnitWorkloadStatusWaiting,
				StatusInfo: application.StatusInfo{
					Message: "waiting for machine",
					Since:   s.clock.Now(),
				},
			},
		},
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
		Channel:      "24.04",
		OSType:       domaincharm.Ubuntu,
		Architecture: domaincharm.ARM64,
	}
	app := application.AddApplicationArg{
		Charm:    ch,
		Platform: platform,
		Origin: domaincharm.CharmOrigin{
			ReferenceName: "foo",
			Source:        domaincharm.CharmHubSource,
			Revision:      42,
		},
		Scale: 1,
	}
	s.state.EXPECT().GetModelType(gomock.Any()).Return("iaas", nil)
	s.state.EXPECT().StorageDefaults(gomock.Any()).Return(domainstorage.StorageDefaults{DefaultFilesystemSource: ptr("fast")}, nil)
	s.state.EXPECT().CreateApplication(domaintesting.IsAtomicContextChecker, "foo", app).Return(id, nil)
	s.state.EXPECT().AddUnits(domaintesting.IsAtomicContextChecker, id, u)
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
		UnitName: "ubuntu/666",
	}
	_, err := s.service.CreateApplication(context.Background(), "foo", s.charm, corecharm.Origin{
		Source:   corecharm.CharmHub,
		Platform: corecharm.MustParsePlatform("arm64/ubuntu/24.04"),
		Revision: ptr(42),
	}, AddApplicationArgs{
		ReferenceName: "foo",
		Storage: map[string]storage.Directive{
			"data": {Count: 2},
		},
	}, a)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *applicationServiceSuite) TestCreateWithSharedStorageMissingDirectives(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetModelType(gomock.Any()).Return("iaas", nil)
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
		UnitName: "ubuntu/666",
	}
	_, err := s.service.CreateApplication(context.Background(), "foo", s.charm, corecharm.Origin{
		Platform: corecharm.MustParsePlatform("arm64/ubuntu/24.04"),
	}, AddApplicationArgs{
		ReferenceName: "foo",
	}, a)
	c.Assert(err, jc.ErrorIs, storageerrors.MissingSharedStorageDirectiveError)
	c.Assert(err, gc.ErrorMatches, `.*adding default storage directives: no storage directive specified for shared charm storage "data"`)
}

func (s *applicationServiceSuite) TestCreateWithStorageValidates(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetModelType(gomock.Any()).Return("iaas", nil)
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
		UnitName: "ubuntu/666",
	}
	_, err := s.service.CreateApplication(context.Background(), "foo", s.charm, corecharm.Origin{
		Platform: corecharm.MustParsePlatform("arm64/ubuntu/24.04"),
	}, AddApplicationArgs{
		ReferenceName: "foo",
		Storage: map[string]storage.Directive{
			"logs": {Count: 2},
		},
	}, a)
	c.Assert(err, gc.ErrorMatches, `.*invalid storage directives: charm "mine" has no store called "logs"`)
}

func (s *applicationServiceSuite) TestAddUnits(c *gc.C) {
	defer s.setupMocks(c).Finish()

	u := application.AddUnitArg{
		UnitName: "ubuntu/666",
		UnitStatusArg: application.UnitStatusArg{
			AgentStatus: application.UnitAgentStatusInfo{
				StatusID: application.UnitAgentStatusAllocating,
				StatusInfo: application.StatusInfo{
					Since: s.clock.Now(),
				},
			},
			WorkloadStatus: application.UnitWorkloadStatusInfo{
				StatusID: application.UnitWorkloadStatusWaiting,
				StatusInfo: application.StatusInfo{
					Message: "installing agent",
					Since:   s.clock.Now(),
				},
			},
		},
	}
	appID := applicationtesting.GenApplicationUUID(c)
	s.state.EXPECT().GetModelType(gomock.Any()).Return("caas", nil)
	s.state.EXPECT().GetApplicationID(domaintesting.IsAtomicContextChecker, "666").Return(appID, nil)
	s.state.EXPECT().AddUnits(domaintesting.IsAtomicContextChecker, appID, u).Return(nil)

	a := AddUnitArg{
		UnitName: "ubuntu/666",
	}
	err := s.service.AddUnits(context.Background(), "666", a)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *applicationServiceSuite) TestGetUnitUUIDs(c *gc.C) {
	defer s.setupMocks(c).Finish()

	names := []coreunit.Name{coreunit.Name("foo/666"), coreunit.Name("foo/667")}
	uuids := []coreunit.UUID{unittesting.GenUnitUUID(c), unittesting.GenUnitUUID(c)}
	s.state.EXPECT().GetUnitUUIDs(gomock.Any(), names).Return(uuids, nil)

	us, err := s.service.GetUnitUUIDs(context.Background(), names)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(us, gc.DeepEquals, uuids)
}

func (s *applicationServiceSuite) TestGetUnitUUIDsErrors(c *gc.C) {
	defer s.setupMocks(c).Finish()

	names := []coreunit.Name{coreunit.Name("foo/666"), coreunit.Name("foo/667")}
	s.state.EXPECT().GetUnitUUIDs(gomock.Any(), names).Return(nil, applicationerrors.UnitNotFound)

	_, err := s.service.GetUnitUUIDs(context.Background(), names)
	c.Assert(err, jc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *applicationServiceSuite) TestGetUnitUUID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	uuid := unittesting.GenUnitUUID(c)
	unitName := coreunit.Name("foo/666")
	s.state.EXPECT().GetUnitUUIDs(gomock.Any(), []coreunit.Name{unitName}).Return([]coreunit.UUID{uuid}, nil)

	u, err := s.service.GetUnitUUID(context.Background(), unitName)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(u, gc.Equals, uuid)
}

func (s *applicationServiceSuite) TestGetUnitUUIDErrors(c *gc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("foo/666")
	s.state.EXPECT().GetUnitUUIDs(gomock.Any(), []coreunit.Name{unitName}).Return(nil, applicationerrors.UnitNotFound)

	_, err := s.service.GetUnitUUID(context.Background(), unitName)
	c.Assert(err, jc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *applicationServiceSuite) TestGetUnitNames(c *gc.C) {
	defer s.setupMocks(c).Finish()

	names := []coreunit.Name{coreunit.Name("foo/666"), coreunit.Name("foo/667")}
	uuids := []coreunit.UUID{unittesting.GenUnitUUID(c), unittesting.GenUnitUUID(c)}
	s.state.EXPECT().GetUnitNames(gomock.Any(), uuids).Return(names, nil)

	u, err := s.service.GetUnitNames(context.Background(), uuids)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(u, gc.DeepEquals, names)
}

func (s *applicationServiceSuite) TestGetUnitNamesErrors(c *gc.C) {
	defer s.setupMocks(c).Finish()

	uuids := []coreunit.UUID{unittesting.GenUnitUUID(c), unittesting.GenUnitUUID(c)}
	s.state.EXPECT().GetUnitNames(gomock.Any(), uuids).Return(nil, applicationerrors.UnitNotFound)

	_, err := s.service.GetUnitNames(context.Background(), uuids)
	c.Assert(err, jc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *applicationServiceSuite) TestRegisterCAASUnit(c *gc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("foo/666")

	s.state.EXPECT().GetApplicationID(domaintesting.IsAtomicContextChecker, "foo").Return("app-id", nil)
	s.state.EXPECT().GetUnitLife(domaintesting.IsAtomicContextChecker, unitName).Return(life.Alive, nil)
	s.state.EXPECT().UpdateUnitContainer(domaintesting.IsAtomicContextChecker, unitName, &application.CloudContainer{
		ProviderId: "provider-id",
		Address: &application.ContainerAddress{
			Device: application.ContainerDevice{
				Name:              `placeholder for "foo/666" cloud container`,
				DeviceTypeID:      0,
				VirtualPortTypeID: 0,
			},
			Value:       "10.6.6.6",
			AddressType: 0,
			Scope:       3,
			Origin:      1,
			ConfigType:  1,
		},
		Ports: ptr([]string{"666"}),
	})
	unitUUID := unittesting.GenUnitUUID(c)
	s.state.EXPECT().GetUnitUUID(domaintesting.IsAtomicContextChecker, unitName).Return(unitUUID, nil)
	s.state.EXPECT().SetUnitPassword(domaintesting.IsAtomicContextChecker, unitUUID, application.PasswordInfo{
		PasswordHash:  "passwordhash",
		HashAlgorithm: 0,
	})

	p := RegisterCAASUnitParams{
		UnitName:     unitName,
		PasswordHash: "passwordhash",
		ProviderId:   "provider-id",
		Address:      ptr("10.6.6.6"),
		Ports:        ptr([]string{"666"}),
		OrderedScale: true,
		OrderedId:    1,
	}
	err := s.service.RegisterCAASUnit(context.Background(), "foo", p)
	c.Assert(err, jc.ErrorIsNil)
}

var unitParams = RegisterCAASUnitParams{
	UnitName:     coreunit.Name("foo/666"),
	PasswordHash: "passwordhash",
	ProviderId:   "provider-id",
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
	p.ProviderId = ""
	err := s.service.RegisterCAASUnit(context.Background(), "foo", p)
	c.Assert(err, gc.ErrorMatches, "provider id not valid")
}

func (s *applicationServiceSuite) TestRegisterCAASUnitMissingPasswordHash(c *gc.C) {
	defer s.setupMocks(c).Finish()

	p := unitParams
	p.PasswordHash = ""
	err := s.service.RegisterCAASUnit(context.Background(), "foo", p)
	c.Assert(err, gc.ErrorMatches, "password hash not valid")
}

func (s *applicationServiceSuite) TestUpdateCAASUnit(c *gc.C) {
	defer s.setupMocks(c).Finish()

	appID := applicationtesting.GenApplicationUUID(c)
	unitUUID := unittesting.GenUnitUUID(c)
	unitName := coreunit.Name("foo/666")
	s.state.EXPECT().GetApplicationLife(domaintesting.IsAtomicContextChecker, "foo").Return(appID, life.Alive, nil)
	s.state.EXPECT().UpdateUnitContainer(domaintesting.IsAtomicContextChecker, unitName, &application.CloudContainer{
		ProviderId: "provider-id",
		Address: &application.ContainerAddress{
			Device: application.ContainerDevice{
				Name:              `placeholder for "foo/666" cloud container`,
				DeviceTypeID:      0,
				VirtualPortTypeID: 0,
			},
			Value:       "10.6.6.6",
			AddressType: 0,
			Scope:       3,
			Origin:      1,
			ConfigType:  1,
		},
		Ports: ptr([]string{"666"}),
	})
	s.state.EXPECT().GetUnitUUID(domaintesting.IsAtomicContextChecker, unitName).Return(unitUUID, nil)

	now := time.Now()
	s.state.EXPECT().SetUnitAgentStatus(domaintesting.IsAtomicContextChecker, unitUUID, application.UnitAgentStatusInfo{
		StatusID: application.UnitAgentStatusIdle,
		StatusInfo: application.StatusInfo{
			Message: "agent status",
			Data:    map[string]string{"foo": "bar"},
			Since:   now,
		},
	})
	s.state.EXPECT().SetUnitWorkloadStatus(domaintesting.IsAtomicContextChecker, unitUUID, application.UnitWorkloadStatusInfo{
		StatusID: application.UnitWorkloadStatusWaiting,
		StatusInfo: application.StatusInfo{
			Message: "workload status",
			Data:    map[string]string{"foo": "bar"},
			Since:   now,
		},
	})
	s.state.EXPECT().SetCloudContainerStatus(domaintesting.IsAtomicContextChecker, unitUUID, application.CloudContainerStatusStatusInfo{
		StatusID: application.CloudContainerStatusRunning,
		StatusInfo: application.StatusInfo{
			Message: "container status",
			Data:    map[string]string{"foo": "bar"},
			Since:   now,
		},
	})

	err := s.service.UpdateCAASUnit(context.Background(), unitName, UpdateCAASUnitParams{
		ProviderId: ptr("provider-id"),
		Address:    ptr("10.6.6.6"),
		Ports:      ptr([]string{"666"}),
		AgentStatus: ptr(StatusParams{
			Status:  "idle",
			Message: "agent status",
			Data:    map[string]any{"foo": "bar"},
			Since:   ptr(now),
		}),
		WorkloadStatus: ptr(StatusParams{
			Status:  "waiting",
			Message: "workload status",
			Data:    map[string]any{"foo": "bar"},
			Since:   ptr(now),
		}),
		CloudContainerStatus: ptr(StatusParams{
			Status:  "running",
			Message: "container status",
			Data:    map[string]any{"foo": "bar"},
			Since:   ptr(now),
		}),
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *applicationServiceSuite) TestUpdateCAASUnitNotAlive(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := applicationtesting.GenApplicationUUID(c)
	s.state.EXPECT().GetApplicationLife(domaintesting.IsAtomicContextChecker, "foo").Return(id, life.Dying, nil)

	err := s.service.UpdateCAASUnit(context.Background(), coreunit.Name("foo/666"), UpdateCAASUnitParams{})
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationNotAlive)
}

func (s *applicationServiceSuite) TestSetUnitPassword(c *gc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID := unittesting.GenUnitUUID(c)
	s.state.EXPECT().GetUnitUUID(domaintesting.IsAtomicContextChecker, coreunit.Name("foo/666")).Return(unitUUID, nil)
	s.state.EXPECT().SetUnitPassword(domaintesting.IsAtomicContextChecker, unitUUID, application.PasswordInfo{
		PasswordHash:  "password",
		HashAlgorithm: 0,
	})

	err := s.service.SetUnitPassword(context.Background(), coreunit.Name("foo/666"), "password")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *applicationServiceSuite) TestGetCharmByApplicationName(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := applicationtesting.GenApplicationUUID(c)

	s.state.EXPECT().GetCharmByApplicationID(gomock.Any(), id).Return(domaincharm.Charm{
		Metadata: domaincharm.Metadata{
			Name: "foo",

			// RunAs becomes mandatory when being persisted. Empty string is not
			// allowed.
			RunAs: "default",
		},
	}, domaincharm.CharmOrigin{
		ReferenceName: "bar",
		Revision:      42,
	}, application.Platform{
		OSType:       domaincharm.Ubuntu,
		Architecture: domaincharm.AMD64,
	}, nil)

	metadata, origin, platform, err := s.service.GetCharmByApplicationID(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(metadata.Meta(), gc.DeepEquals, &charm.Meta{
		Name: "foo",

		// Notice that the RunAs field becomes empty string when being returned.
	})
	c.Check(origin, gc.DeepEquals, domaincharm.CharmOrigin{
		ReferenceName: "bar",
		Revision:      42,
	})
	c.Check(platform, gc.DeepEquals, application.Platform{
		OSType:       domaincharm.Ubuntu,
		Architecture: domaincharm.AMD64,
	})
}

func (s *applicationServiceSuite) TestGetCharmIDByApplicationName(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := charmtesting.GenCharmID(c)

	s.state.EXPECT().GetCharmIDByApplicationName(gomock.Any(), "foo").Return(id, nil)

	charmID, err := s.service.GetCharmIDByApplicationName(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(charmID, gc.DeepEquals, id)
}

func (s *applicationServiceSuite) TestGetApplicationIDByUnitName(c *gc.C) {
	defer s.setupMocks(c).Finish()

	expectedAppID := applicationtesting.GenApplicationUUID(c)
	unitName := coreunit.Name("foo")
	s.state.EXPECT().GetApplicationIDByUnitName(gomock.Any(), unitName).Return(expectedAppID, nil)

	obtainedAppID, err := s.service.GetApplicationIDByUnitName(context.Background(), unitName)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(obtainedAppID, gc.DeepEquals, expectedAppID)
}

func (s *applicationServiceSuite) TestGetCharmModifiedVersion(c *gc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := applicationtesting.GenApplicationUUID(c)
	s.state.EXPECT().GetCharmModifiedVersion(gomock.Any(), appUUID).Return(42, nil)

	obtained, err := s.service.GetCharmModifiedVersion(context.Background(), appUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(obtained, gc.DeepEquals, 42)
}

func (s *applicationServiceSuite) TestGetApplicationsWithPendingCharmsFromUUIDs(c *gc.C) {
	defer s.setupMocks(c).Finish()

	uuids := []coreapplication.ID{
		applicationtesting.GenApplicationUUID(c),
		applicationtesting.GenApplicationUUID(c),
	}

	s.state.EXPECT().GetApplicationsWithPendingCharmsFromUUIDs(gomock.Any(), uuids).Return(uuids[0:1], nil)

	received, err := s.service.GetApplicationsWithPendingCharmsFromUUIDs(context.Background(), uuids)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(received, gc.DeepEquals, uuids[0:1])
}

func (s *applicationServiceSuite) TestGetApplicationsWithPendingCharmsFromUUIDsIsInvalidUUID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	uuids := []coreapplication.ID{
		"foo",
	}

	_, err := s.service.GetApplicationsWithPendingCharmsFromUUIDs(context.Background(), uuids)
	c.Assert(err, jc.ErrorIs, jujuerrors.NotValid)
}

func (s *applicationServiceSuite) TestGetApplicationsWithPendingCharmsFromUUIDsWithNoUUIDs(c *gc.C) {
	defer s.setupMocks(c).Finish()

	uuids := []coreapplication.ID{}

	_, err := s.service.GetApplicationsWithPendingCharmsFromUUIDs(context.Background(), uuids)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *applicationServiceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.state = NewMockApplicationState(ctrl)
	s.charm = NewMockCharm(ctrl)
	s.secret = NewMockDeleteSecretState(ctrl)
	registry := corestorage.ConstModelStorageRegistry(func() storage.ProviderRegistry {
		return storage.ChainedProviderRegistry{
			dummystorage.StorageProviders(),
			provider.CommonStorageProviders(),
		}
	})

	s.clock = testclock.NewClock(time.Time{})
	s.service = NewApplicationService(s.state, s.secret, registry, loggertesting.WrapCheckLog(c))
	s.service.clock = s.clock

	s.state.EXPECT().RunAtomic(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, fn func(ctx domain.AtomicContext) error) error {
		return fn(domaintesting.NewAtomicContext(ctx))
	}).AnyTimes()

	return ctrl
}

type applicationWatcherServiceSuite struct {
	testing.IsolationSuite

	service *WatchableApplicationService

	state          *MockApplicationState
	charm          *MockCharm
	secret         *MockDeleteSecretState
	clock          *testclock.Clock
	watcherFactory *MockWatcherFactory
}

var _ = gc.Suite(&applicationWatcherServiceSuite{})

func (s *applicationWatcherServiceSuite) TestWatchApplicationsWithPendingCharmMapper(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// There is an integration test to ensure correct wire up. This test ensures
	// that the mapper correctly orders the results based on changes emitted by
	// the watcher.

	appID := applicationtesting.GenApplicationUUID(c)

	s.state.EXPECT().GetApplicationsWithPendingCharmsFromUUIDs(gomock.Any(), []coreapplication.ID{appID}).Return([]coreapplication.ID{
		appID,
	}, nil)

	changes := []changestream.ChangeEvent{&changeEvent{
		typ:       changestream.All,
		namespace: "application",
		changed:   appID.String(),
	}}

	result, err := s.service.watchApplicationsWithPendingCharmsMapper(context.Background(), changes)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, gc.DeepEquals, changes)
}

func (s *applicationWatcherServiceSuite) TestWatchApplicationsWithPendingCharmMapperInvalidID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// There is an integration test to ensure correct wire up. This test ensures
	// that the mapper correctly orders the results based on changes emitted by
	// the watcher.

	changes := []changestream.ChangeEvent{&changeEvent{
		typ:       changestream.All,
		namespace: "application",
		changed:   "foo",
	}}

	_, err := s.service.watchApplicationsWithPendingCharmsMapper(context.Background(), changes)
	c.Assert(err, jc.ErrorIs, jujuerrors.NotValid)
}

func (s *applicationWatcherServiceSuite) TestWatchApplicationsWithPendingCharmMapperOrder(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// There is an integration test to ensure correct wire up. This test ensures
	// that the mapper correctly orders the results based on changes emitted by
	// the watcher.

	appIDs := make([]coreapplication.ID, 4)
	for i := range appIDs {
		appIDs[i] = applicationtesting.GenApplicationUUID(c)
	}

	changes := make([]changestream.ChangeEvent, len(appIDs))
	for i, appID := range appIDs {
		changes[i] = &changeEvent{
			typ:       changestream.All,
			namespace: "application",
			changed:   appID.String(),
		}
	}

	// Ensure order is persvered if the state returns the uuids in an unexpected
	// order. This is because we can't guarantee the order if there are holes in
	// the pending sequence.

	shuffle := make([]coreapplication.ID, len(appIDs))
	copy(shuffle, appIDs)
	rand.Shuffle(len(shuffle), func(i, j int) {
		shuffle[i], shuffle[j] = shuffle[j], shuffle[i]
	})

	s.state.EXPECT().GetApplicationsWithPendingCharmsFromUUIDs(gomock.Any(), appIDs).Return(shuffle, nil)

	result, err := s.service.watchApplicationsWithPendingCharmsMapper(context.Background(), changes)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, gc.DeepEquals, changes)
}

func (s *applicationWatcherServiceSuite) TestWatchApplicationsWithPendingCharmMapperDropped(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// There is an integration test to ensure correct wire up. This test ensures
	// that the mapper correctly orders the results based on changes emitted by
	// the watcher.

	appIDs := make([]coreapplication.ID, 10)
	for i := range appIDs {
		appIDs[i] = applicationtesting.GenApplicationUUID(c)
	}

	changes := make([]changestream.ChangeEvent, len(appIDs))
	for i, appID := range appIDs {
		changes[i] = &changeEvent{
			typ:       changestream.All,
			namespace: "application",
			changed:   appID.String(),
		}
	}

	// Ensure order is persvered if the state returns the uuids in an unexpected
	// order. This is because we can't guarantee the order if there are holes in
	// the pending sequence.

	var dropped []coreapplication.ID
	var expected []changestream.ChangeEvent
	for i, appID := range appIDs {
		if rand.IntN(2) == 0 {
			continue
		}
		dropped = append(dropped, appID)
		expected = append(expected, changes[i])
	}

	s.state.EXPECT().GetApplicationsWithPendingCharmsFromUUIDs(gomock.Any(), appIDs).Return(dropped, nil)

	result, err := s.service.watchApplicationsWithPendingCharmsMapper(context.Background(), changes)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, gc.DeepEquals, expected)
}

func (s *applicationWatcherServiceSuite) TestWatchApplicationsWithPendingCharmMapperOrderDropped(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// There is an integration test to ensure correct wire up. This test ensures
	// that the mapper correctly orders the results based on changes emitted by
	// the watcher.

	appIDs := make([]coreapplication.ID, 10)
	for i := range appIDs {
		appIDs[i] = applicationtesting.GenApplicationUUID(c)
	}

	changes := make([]changestream.ChangeEvent, len(appIDs))
	for i, appID := range appIDs {
		changes[i] = &changeEvent{
			typ:       changestream.All,
			namespace: "application",
			changed:   appID.String(),
		}
	}

	// Ensure order is persvered if the state returns the uuids in an unexpected
	// order. This is because we can't guarantee the order if there are holes in
	// the pending sequence.

	var dropped []coreapplication.ID
	var expected []changestream.ChangeEvent
	for i, appID := range appIDs {
		if rand.IntN(2) == 0 {
			continue
		}
		dropped = append(dropped, appID)
		expected = append(expected, changes[i])
	}

	// Shuffle them to replicate out of order return.

	shuffle := make([]coreapplication.ID, len(dropped))
	copy(shuffle, dropped)
	rand.Shuffle(len(shuffle), func(i, j int) {
		shuffle[i], shuffle[j] = shuffle[j], shuffle[i]
	})

	s.state.EXPECT().GetApplicationsWithPendingCharmsFromUUIDs(gomock.Any(), appIDs).Return(shuffle, nil)

	result, err := s.service.watchApplicationsWithPendingCharmsMapper(context.Background(), changes)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, gc.DeepEquals, expected)
}

func (s *applicationWatcherServiceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockApplicationState(ctrl)
	s.charm = NewMockCharm(ctrl)
	s.secret = NewMockDeleteSecretState(ctrl)
	s.watcherFactory = NewMockWatcherFactory(ctrl)

	registry := corestorage.ConstModelStorageRegistry(func() storage.ProviderRegistry {
		return storage.ChainedProviderRegistry{
			dummystorage.StorageProviders(),
			provider.CommonStorageProviders(),
		}
	})

	modelUUID := modeltesting.GenModelUUID(c)

	s.clock = testclock.NewClock(time.Time{})
	s.service = NewWatchableApplicationService(
		s.state,
		s.secret,
		s.watcherFactory,
		modelUUID,
		nil,
		nil,
		registry,
		loggertesting.WrapCheckLog(c),
	)
	s.service.clock = s.clock

	s.state.EXPECT().RunAtomic(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, fn func(ctx domain.AtomicContext) error) error {
		return fn(domaintesting.NewAtomicContext(ctx))
	}).AnyTimes()

	return ctrl
}

type providerApplicationServiceSuite struct {
	testing.IsolationSuite

	service *ProviderApplicationService

	modelID model.UUID

	agentVersionGetter *MockAgentVersionGetter
	provider           *MockProvider
}

var _ = gc.Suite(&providerApplicationServiceSuite{})

func (s *providerApplicationServiceSuite) TestGetSupportedFeatures(c *gc.C) {
	ctrl := s.setupMocks(c, func(ctx context.Context) (Provider, error) {
		return s.provider, nil
	})
	defer ctrl.Finish()

	agentVersion := version.MustParse("4.0.0")
	s.agentVersionGetter.EXPECT().GetModelTargetAgentVersion(gomock.Any(), s.modelID).Return(agentVersion, nil)

	s.provider.EXPECT().SupportedFeatures().Return(assumes.FeatureSet{}, nil)

	features, err := s.service.GetSupportedFeatures(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	var fs assumes.FeatureSet
	fs.Add(assumes.Feature{
		Name:        "juju",
		Description: assumes.UserFriendlyFeatureDescriptions["juju"],
		Version:     &agentVersion,
	})
	c.Check(features, jc.DeepEquals, fs)
}

func (s *providerApplicationServiceSuite) TestGetSupportedFeaturesNotSupported(c *gc.C) {
	ctrl := s.setupMocks(c, func(ctx context.Context) (Provider, error) {
		return s.provider, jujuerrors.NotSupported
	})
	defer ctrl.Finish()

	agentVersion := version.MustParse("4.0.0")
	s.agentVersionGetter.EXPECT().GetModelTargetAgentVersion(gomock.Any(), s.modelID).Return(agentVersion, nil)

	features, err := s.service.GetSupportedFeatures(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	var fs assumes.FeatureSet
	fs.Add(assumes.Feature{
		Name:        "juju",
		Description: assumes.UserFriendlyFeatureDescriptions["juju"],
		Version:     &agentVersion,
	})
	c.Check(features, jc.DeepEquals, fs)
}

func (s *providerApplicationServiceSuite) setupMocks(c *gc.C, fn func(ctx context.Context) (Provider, error)) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.modelID = model.UUID(internaltesting.ModelTag.Id())

	s.agentVersionGetter = NewMockAgentVersionGetter(ctrl)
	s.provider = NewMockProvider(ctrl)

	s.service = &ProviderApplicationService{
		modelID:            s.modelID,
		agentVersionGetter: s.agentVersionGetter,
		provider:           fn,
	}

	return ctrl
}
