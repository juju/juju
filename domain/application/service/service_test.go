// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	applicationtesting "github.com/juju/juju/core/application/testing"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/architecture"
	domaincharm "github.com/juju/juju/domain/application/charm"
	domainstorage "github.com/juju/juju/domain/storage"
	domaintesting "github.com/juju/juju/domain/testing"
	"github.com/juju/juju/internal/charm"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type migrationServiceSuite struct {
	baseSuite

	service *MigrationService
}

var _ = gc.Suite(&migrationServiceSuite{})

func (s *migrationServiceSuite) TestImportApplication(c *gc.C) {
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
		Manifest:      s.minimalManifest(c),
		ReferenceName: "ubuntu",
		Source:        domaincharm.CharmHubSource,
		Revision:      42,
		Architecture:  architecture.ARM64,
	}
	platform := application.Platform{
		Channel:      "24.04",
		OSType:       application.Ubuntu,
		Architecture: architecture.ARM64,
	}
	downloadInfo := &domaincharm.DownloadInfo{
		DownloadURL:        "http://example.com",
		DownloadSize:       24,
		CharmhubIdentifier: "foobar",
	}
	app := application.AddApplicationArg{
		Charm:             ch,
		Platform:          platform,
		Scale:             1,
		CharmDownloadInfo: downloadInfo,
	}
	s.state.EXPECT().GetModelType(gomock.Any()).Return("iaas", nil)
	s.state.EXPECT().StorageDefaults(gomock.Any()).Return(domainstorage.StorageDefaults{}, nil)
	s.state.EXPECT().CreateApplication(domaintesting.IsAtomicContextChecker, "ubuntu", app).Return(id, nil)
	s.state.EXPECT().InsertUnit(domaintesting.IsAtomicContextChecker, id, u)
	s.charm.EXPECT().Actions().Return(&charm.Actions{})
	s.charm.EXPECT().Config().Return(&charm.Config{})
	s.charm.EXPECT().Meta().Return(&charm.Meta{
		Name: "ubuntu",
	}).MinTimes(1)
	s.charm.EXPECT().Manifest().Return(&charm.Manifest{
		Bases: []charm.Base{
			{
				Name: "ubuntu",
				Channel: charm.Channel{
					Risk: charm.Stable,
				},
				Architectures: []string{"amd64"},
			},
		},
	}).MinTimes(1)

	err := s.service.ImportApplication(context.Background(), "ubuntu", s.charm, corecharm.Origin{
		Source:   corecharm.CharmHub,
		Platform: corecharm.MustParsePlatform("arm64/ubuntu/24.04"),
		Revision: ptr(42),
	}, AddApplicationArgs{
		ReferenceName: "ubuntu",
		DownloadInfo:  downloadInfo,
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

func (s *migrationServiceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := s.baseSuite.setupMocks(c)

	s.service = NewMigrationService(s.state, s.storageRegistryGetter, s.clock, loggertesting.WrapCheckLog(c))

	return ctrl
}
