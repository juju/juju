// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"math/rand/v2"
	"time"

	"github.com/juju/clock"
	"github.com/juju/clock/testclock"
	jujuerrors "github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	coreapplication "github.com/juju/juju/core/application"
	applicationtesting "github.com/juju/juju/core/application/testing"
	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/changestream"
	corecharm "github.com/juju/juju/core/charm"
	charmtesting "github.com/juju/juju/core/charm/testing"
	"github.com/juju/juju/core/config"
	modeltesting "github.com/juju/juju/core/model/testing"
	objectstoretesting "github.com/juju/juju/core/objectstore/testing"
	corestatus "github.com/juju/juju/core/status"
	corestorage "github.com/juju/juju/core/storage"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/architecture"
	applicationcharm "github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/domain/application/charm/store"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	domaintesting "github.com/juju/juju/domain/testing"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/statushistory"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/storage/provider"
	dummystorage "github.com/juju/juju/internal/storage/provider/dummy"
	"github.com/juju/juju/testcharms"
)

type applicationServiceSuite struct {
	baseSuite
}

var _ = gc.Suite(&applicationServiceSuite{})

func (s *applicationServiceSuite) TestGetApplicationStatus(c *gc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now()

	applicationUUID := applicationtesting.GenApplicationUUID(c)
	s.state.EXPECT().GetApplicationStatus(gomock.Any(), applicationUUID).Return(
		&application.StatusInfo[application.WorkloadStatusType]{
			Status:  application.WorkloadStatusActive,
			Message: "doink",
			Data:    []byte(`{"foo":"bar"}`),
			Since:   &now,
		}, nil)

	obtained, err := s.service.GetApplicationStatus(context.Background(), applicationUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(obtained, jc.DeepEquals, &corestatus.StatusInfo{
		Status:  corestatus.Active,
		Message: "doink",
		Data:    map[string]interface{}{"foo": "bar"},
		Since:   &now,
	})
}

func (s *applicationServiceSuite) TestGetApplicationStatusFallbackToUnits(c *gc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now()

	applicationUUID := applicationtesting.GenApplicationUUID(c)
	s.state.EXPECT().GetApplicationStatus(gomock.Any(), applicationUUID).Return(
		&application.StatusInfo[application.WorkloadStatusType]{
			Status: application.WorkloadStatusUnset,
		}, nil)

	s.state.EXPECT().GetUnitWorkloadStatusesForApplication(gomock.Any(), applicationUUID).Return(
		application.UnitWorkloadStatuses{
			"unit-1": {
				StatusInfo: application.StatusInfo[application.WorkloadStatusType]{
					Status:  application.WorkloadStatusActive,
					Message: "doink-1",
					Data:    []byte(`{"foo":"bar-1"}`),
					Since:   &now,
				},
				Present: true,
			},
			"unit-2": {
				StatusInfo: application.StatusInfo[application.WorkloadStatusType]{
					Status:  application.WorkloadStatusBlocked,
					Message: "doink-2",
					Data:    []byte(`{"foo":"bar-2"}`),
					Since:   &now,
				},
				Present: true,
			},
			"unit-3": {
				StatusInfo: application.StatusInfo[application.WorkloadStatusType]{
					Status:  application.WorkloadStatusMaintenance,
					Message: "doink-3",
					Data:    []byte(`{"foo":"bar-3"}`),
					Since:   &now,
				},
				Present: true,
			},
		}, nil)

	obtained, err := s.service.GetApplicationStatus(context.Background(), applicationUUID)
	c.Assert(err, jc.ErrorIsNil)
	// We pick the status from the units with the highest 'severity'
	c.Check(obtained, jc.DeepEquals, &corestatus.StatusInfo{
		Status:  corestatus.Blocked,
		Message: "doink-2",
		Data:    map[string]interface{}{"foo": "bar-2"},
		Since:   &now,
	})
}

func (s *applicationServiceSuite) TestGetApplicationStatusFallbackToUnitsNoUnits(c *gc.C) {
	defer s.setupMocks(c).Finish()

	applicationUUID := applicationtesting.GenApplicationUUID(c)
	s.state.EXPECT().GetApplicationStatus(gomock.Any(), applicationUUID).Return(
		&application.StatusInfo[application.WorkloadStatusType]{
			Status: application.WorkloadStatusUnset,
		}, nil)

	s.state.EXPECT().GetUnitWorkloadStatusesForApplication(gomock.Any(), applicationUUID).Return(
		application.UnitWorkloadStatuses{}, nil,
	)

	obtained, err := s.service.GetApplicationStatus(context.Background(), applicationUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(obtained, jc.DeepEquals, &corestatus.StatusInfo{
		Status: corestatus.Unknown,
	})
}

func (s *applicationServiceSuite) TestSetApplicationStatus(c *gc.C) {
	history := &statusHistoryRecorder{}

	defer s.setupMocksWithStatusHistory(c, history).Finish()

	now := time.Now()

	applicationUUID := applicationtesting.GenApplicationUUID(c)
	s.state.EXPECT().SetApplicationStatus(gomock.Any(), applicationUUID, &application.StatusInfo[application.WorkloadStatusType]{
		Status:  application.WorkloadStatusActive,
		Message: "doink",
		Data:    []byte(`{"foo":"bar"}`),
		Since:   &now,
	})

	err := s.service.SetApplicationStatus(context.Background(), applicationUUID, &corestatus.StatusInfo{
		Status:  corestatus.Active,
		Message: "doink",
		Data:    map[string]interface{}{"foo": "bar"},
		Since:   &now,
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Check(history.records, jc.DeepEquals, []statusHistoryRecord{{
		ns: statushistory.Namespace{Name: "application", ID: applicationUUID.String()},
		s: corestatus.StatusInfo{
			Status:  corestatus.Active,
			Message: "doink",
			Data:    map[string]interface{}{"foo": "bar"},
			Since:   &now,
		},
	}})
}

func (s *applicationServiceSuite) TestSetApplicationStatusInvalidStatus(c *gc.C) {
	defer s.setupMocks(c).Finish()

	applicationUUID := applicationtesting.GenApplicationUUID(c)
	err := s.service.SetApplicationStatus(context.Background(), applicationUUID, &corestatus.StatusInfo{
		Status: corestatus.Status("invalid"),
	})
	c.Assert(err, gc.ErrorMatches, `.*unknown workload status "invalid"`)

	err = s.service.SetApplicationStatus(context.Background(), applicationUUID, &corestatus.StatusInfo{
		Status: corestatus.Allocating,
	})
	c.Assert(err, gc.ErrorMatches, `.*unknown workload status "allocating"`)
}

func (s *applicationServiceSuite) TestSetApplicationStatusForUnitLeader(c *gc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now()

	applicationUUID := applicationtesting.GenApplicationUUID(c)
	unitName := coreunit.Name("foo/666")

	s.leadership.EXPECT().WithLeader(gomock.Any(), "foo", unitName.String(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, _, _ string, fn func(context.Context) error) error {
			return fn(ctx)
		})

	s.state.EXPECT().GetApplicationIDAndNameByUnitName(gomock.Any(), unitName).Return(applicationUUID, "foo", nil)
	s.state.EXPECT().SetApplicationStatus(gomock.Any(), applicationUUID, &application.StatusInfo[application.WorkloadStatusType]{
		Status:  application.WorkloadStatusActive,
		Message: "doink",
		Data:    []byte(`{"foo":"bar"}`),
		Since:   &now,
	})

	err := s.service.SetApplicationStatusForUnitLeader(context.Background(), unitName, &corestatus.StatusInfo{
		Status:  corestatus.Active,
		Message: "doink",
		Data:    map[string]interface{}{"foo": "bar"},
		Since:   &now,
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *applicationServiceSuite) TestSetApplicationStatusForUnitLeaderNotLeader(c *gc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now()

	applicationUUID := applicationtesting.GenApplicationUUID(c)
	unitName := coreunit.Name("foo/666")

	s.leadership.EXPECT().WithLeader(gomock.Any(), "foo", unitName.String(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, _, _ string, fn func(context.Context) error) error {
			return jujuerrors.NotValidf("not leader")
		})

	s.state.EXPECT().GetApplicationIDAndNameByUnitName(gomock.Any(), unitName).Return(applicationUUID, "foo", nil)

	err := s.service.SetApplicationStatusForUnitLeader(context.Background(), unitName, &corestatus.StatusInfo{
		Status:  corestatus.Active,
		Message: "doink",
		Data:    map[string]interface{}{"foo": "bar"},
		Since:   &now,
	})
	c.Assert(err, jc.ErrorIs, jujuerrors.NotValid)
}

func (s *applicationServiceSuite) TestSetApplicationStatusForUnitLeaderInvalidUnitName(c *gc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now()

	unitName := coreunit.Name("!!!!")
	err := s.service.SetApplicationStatusForUnitLeader(context.Background(), unitName, &corestatus.StatusInfo{
		Status:  corestatus.Active,
		Message: "doink",
		Data:    map[string]interface{}{"foo": "bar"},
		Since:   &now,
	})
	c.Assert(err, jc.ErrorIs, coreunit.InvalidUnitName)
}

func (s *applicationServiceSuite) TestSetApplicationStatusForUnitLeaderNoUnitFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now()

	applicationUUID := applicationtesting.GenApplicationUUID(c)
	unitName := coreunit.Name("foo/666")

	s.state.EXPECT().GetApplicationIDAndNameByUnitName(gomock.Any(), unitName).
		Return(applicationUUID, "foo", applicationerrors.UnitNotFound)

	err := s.service.SetApplicationStatusForUnitLeader(context.Background(), unitName, &corestatus.StatusInfo{
		Status:  corestatus.Active,
		Message: "doink",
		Data:    map[string]interface{}{"foo": "bar"},
		Since:   &now,
	})
	c.Assert(err, jc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *applicationServiceSuite) TestGetApplicationDisplayStatusApplicationStatusSet(c *gc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now()

	applicationUUID := applicationtesting.GenApplicationUUID(c)
	s.state.EXPECT().GetApplicationStatus(gomock.Any(), applicationUUID).Return(
		&application.StatusInfo[application.WorkloadStatusType]{
			Status:  application.WorkloadStatusActive,
			Message: "doink",
			Data:    []byte(`{"foo":"bar"}`),
			Since:   &now,
		}, nil)

	obtained, err := s.service.GetApplicationDisplayStatus(context.Background(), applicationUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(obtained, jc.DeepEquals, &corestatus.StatusInfo{
		Status:  corestatus.Active,
		Message: "doink",
		Data:    map[string]interface{}{"foo": "bar"},
		Since:   &now,
	})
}

func (s *applicationServiceSuite) TestGetApplicationDisplayStatusFallbackToUnitsNoUnits(c *gc.C) {
	defer s.setupMocks(c).Finish()

	applicationUUID := applicationtesting.GenApplicationUUID(c)
	s.state.EXPECT().GetApplicationStatus(gomock.Any(), applicationUUID).Return(
		&application.StatusInfo[application.WorkloadStatusType]{
			Status: application.WorkloadStatusUnset,
		}, nil)

	s.state.EXPECT().GetUnitWorkloadAndCloudContainerStatusesForApplication(gomock.Any(), applicationUUID).Return(nil, nil, nil)

	obtained, err := s.service.GetApplicationDisplayStatus(context.Background(), applicationUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(obtained, jc.DeepEquals, &corestatus.StatusInfo{
		Status: corestatus.Unknown,
	})
}

func (s *applicationServiceSuite) TestGetApplicationDisplayStatusFallbackToUnitsNoContainers(c *gc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now()

	applicationUUID := applicationtesting.GenApplicationUUID(c)
	s.state.EXPECT().GetApplicationStatus(gomock.Any(), applicationUUID).Return(
		&application.StatusInfo[application.WorkloadStatusType]{
			Status: application.WorkloadStatusUnset,
		}, nil)

	s.state.EXPECT().GetUnitWorkloadAndCloudContainerStatusesForApplication(gomock.Any(), applicationUUID).Return(
		application.UnitWorkloadStatuses{
			"unit-1": {
				StatusInfo: application.StatusInfo[application.WorkloadStatusType]{
					Status:  application.WorkloadStatusActive,
					Message: "doink",
					Data:    []byte(`{"foo":"bar"}`),
					Since:   &now,
				},
				Present: true,
			},
			"unit-2": {
				StatusInfo: application.StatusInfo[application.WorkloadStatusType]{
					Status:  application.WorkloadStatusActive,
					Message: "doink",
					Data:    []byte(`{"foo":"bar"}`),
					Since:   &now,
				},
				Present: true,
			},
		},
		application.UnitCloudContainerStatuses{}, nil)

	obtained, err := s.service.GetApplicationDisplayStatus(context.Background(), applicationUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(obtained, jc.DeepEquals, &corestatus.StatusInfo{
		Status:  corestatus.Active,
		Message: "doink",
		Data:    map[string]interface{}{"foo": "bar"},
		Since:   &now,
	})
}

func (s *applicationServiceSuite) TestGetApplicationAndUnitStatusesForUnitWithLeaderNotLeader(c *gc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("foo/0")
	applicationUUID := applicationtesting.GenApplicationUUID(c)

	s.state.EXPECT().GetApplicationIDByName(gomock.Any(), "foo").Return(applicationUUID, nil)

	s.leadership.EXPECT().WithLeader(gomock.Any(), "foo", unitName.String(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, _, _ string, fn func(context.Context) error) error {
			return jujuerrors.NotValidf("not leader")
		})

	_, _, err := s.service.GetApplicationAndUnitStatusesForUnitWithLeader(context.Background(), unitName)
	c.Assert(err, jc.ErrorIs, jujuerrors.NotValid)
}

func (s *applicationServiceSuite) TestGetApplicationAndUnitStatusesForUnitWithLeaderInvalidUnitName(c *gc.C) {
	defer s.setupMocks(c).Finish()

	_, _, err := s.service.GetApplicationAndUnitStatusesForUnitWithLeader(context.Background(), coreunit.Name("!!!"))
	c.Assert(err, jc.ErrorIs, coreunit.InvalidUnitName)
}

func (s *applicationServiceSuite) TestGetApplicationAndUnitStatusesForUnitWithLeaderNoApplication(c *gc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("foo/0")

	s.state.EXPECT().GetApplicationIDByName(gomock.Any(), "foo").Return("", applicationerrors.ApplicationNotFound)
	_, _, err := s.service.GetApplicationAndUnitStatusesForUnitWithLeader(context.Background(), unitName)
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *applicationServiceSuite) TestGetApplicationAndUnitStatusesForUnitWithLeaderApplicationStatusSet(c *gc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("foo/0")
	now := time.Now()
	applicationUUID := applicationtesting.GenApplicationUUID(c)

	s.leadership.EXPECT().WithLeader(gomock.Any(), "foo", unitName.String(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, _, _ string, fn func(context.Context) error) error {
			return fn(ctx)
		})

	s.state.EXPECT().GetApplicationIDByName(gomock.Any(), "foo").Return(applicationUUID, nil)

	s.state.EXPECT().GetApplicationStatus(gomock.Any(), applicationUUID).Return(
		&application.StatusInfo[application.WorkloadStatusType]{
			Status:  application.WorkloadStatusActive,
			Message: "doink",
			Data:    []byte(`{"foo":"bar"}`),
			Since:   &now,
		}, nil)

	s.state.EXPECT().GetUnitWorkloadAndCloudContainerStatusesForApplication(gomock.Any(), applicationUUID).Return(
		application.UnitWorkloadStatuses{
			"foo/0": {
				StatusInfo: application.StatusInfo[application.WorkloadStatusType]{
					Status:  application.WorkloadStatusActive,
					Message: "boink",
					Data:    []byte(`{"foo":"baz"}`),
					Since:   &now,
				},
				Present: true,
			},
			"foo/1": {
				StatusInfo: application.StatusInfo[application.WorkloadStatusType]{
					Status:  application.WorkloadStatusBlocked,
					Message: "poink",
					Data:    []byte(`{"foo":"bat"}`),
					Since:   &now,
				},
				Present: true,
			},
		}, nil, nil)

	applicationStatus, unitWorkloadStatuses, err := s.service.GetApplicationAndUnitStatusesForUnitWithLeader(context.Background(), unitName)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(applicationStatus, jc.DeepEquals, &corestatus.StatusInfo{
		Status:  corestatus.Active,
		Message: "doink",
		Data:    map[string]interface{}{"foo": "bar"},
		Since:   &now,
	})
	c.Check(unitWorkloadStatuses, jc.DeepEquals, map[coreunit.Name]corestatus.StatusInfo{
		"foo/0": {
			Status:  corestatus.Active,
			Message: "boink",
			Data:    map[string]interface{}{"foo": "baz"},
			Since:   &now,
		},
		"foo/1": {
			Status:  corestatus.Blocked,
			Message: "poink",
			Data:    map[string]interface{}{"foo": "bat"},
			Since:   &now,
		},
	})
}

func (s *applicationServiceSuite) TestGetApplicationAndUnitStatusesForUnitWithLeaderApplicationStatusUnset(c *gc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("foo/0")
	now := time.Now()
	applicationUUID := applicationtesting.GenApplicationUUID(c)

	s.leadership.EXPECT().WithLeader(gomock.Any(), "foo", unitName.String(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, _, _ string, fn func(context.Context) error) error {
			return fn(ctx)
		})

	s.state.EXPECT().GetApplicationIDByName(gomock.Any(), "foo").Return(applicationUUID, nil)

	s.state.EXPECT().GetApplicationStatus(gomock.Any(), applicationUUID).Return(
		&application.StatusInfo[application.WorkloadStatusType]{
			Status: application.WorkloadStatusUnset,
		}, nil)

	s.state.EXPECT().GetUnitWorkloadAndCloudContainerStatusesForApplication(gomock.Any(), applicationUUID).Return(
		application.UnitWorkloadStatuses{
			"foo/0": {
				StatusInfo: application.StatusInfo[application.WorkloadStatusType]{
					Status:  application.WorkloadStatusActive,
					Message: "boink",
					Data:    []byte(`{"foo":"baz"}`),
					Since:   &now,
				},
				Present: true,
			},
			"foo/1": {
				StatusInfo: application.StatusInfo[application.WorkloadStatusType]{
					Status:  application.WorkloadStatusActive,
					Message: "poink",
					Data:    []byte(`{"foo":"bat"}`),
					Since:   &now,
				},
				Present: true,
			},
		}, application.UnitCloudContainerStatuses{
			"foo/0": {
				Status:  application.CloudContainerStatusBlocked,
				Message: "zoink",
				Data:    []byte(`{"foo":"baz"}`),
				Since:   &now,
			},
			"foo/1": {
				Status:  application.CloudContainerStatusRunning,
				Message: "yoink",
				Data:    []byte(`{"foo":"bat"}`),
				Since:   &now,
			},
		}, nil)

	applicationStatus, unitWorkloadStatuses, err := s.service.GetApplicationAndUnitStatusesForUnitWithLeader(context.Background(), unitName)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(applicationStatus, jc.DeepEquals, &corestatus.StatusInfo{
		Status:  corestatus.Blocked,
		Message: "zoink",
		Data:    map[string]interface{}{"foo": "baz"},
		Since:   &now,
	})
	c.Check(unitWorkloadStatuses, jc.DeepEquals, map[coreunit.Name]corestatus.StatusInfo{
		"foo/0": {
			Status:  corestatus.Active,
			Message: "boink",
			Data:    map[string]interface{}{"foo": "baz"},
			Since:   &now,
		},
		"foo/1": {
			Status:  corestatus.Active,
			Message: "poink",
			Data:    map[string]interface{}{"foo": "bat"},
			Since:   &now,
		},
	})
}

func (s *applicationServiceSuite) TestGetCharmByApplicationName(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := applicationtesting.GenApplicationUUID(c)

	s.state.EXPECT().GetCharmByApplicationID(gomock.Any(), id).Return(applicationcharm.Charm{
		Metadata: applicationcharm.Metadata{
			Name: "foo",

			// RunAs becomes mandatory when being persisted. Empty string is not
			// allowed.
			RunAs: "default",
		},
		ReferenceName: "bar",
		Revision:      42,
		Source:        applicationcharm.LocalSource,
		Architecture:  architecture.AMD64,
	}, nil)

	ch, locator, err := s.service.GetCharmByApplicationID(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(ch.Meta(), gc.DeepEquals, &charm.Meta{
		Name: "foo",

		// Notice that the RunAs field becomes empty string when being returned.
	})
	c.Check(locator, gc.DeepEquals, applicationcharm.CharmLocator{
		Name:         "bar",
		Revision:     42,
		Source:       applicationcharm.LocalSource,
		Architecture: architecture.AMD64,
	})
}

func (s *applicationServiceSuite) TestGetCharmLocatorByApplicationName(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := charmtesting.GenCharmID(c)

	s.state.EXPECT().GetCharmIDByApplicationName(gomock.Any(), "foo").Return(id, nil)
	s.state.EXPECT().GetCharmLocatorByCharmID(gomock.Any(), id).Return(applicationcharm.CharmLocator{
		Name:         "bar",
		Revision:     42,
		Source:       applicationcharm.LocalSource,
		Architecture: architecture.AMD64,
	}, nil)

	expectedLocator := applicationcharm.CharmLocator{
		Name:         "bar",
		Revision:     42,
		Source:       applicationcharm.LocalSource,
		Architecture: architecture.AMD64,
	}
	locator, err := s.service.GetCharmLocatorByApplicationName(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(locator, gc.DeepEquals, expectedLocator)
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

func (s *applicationServiceSuite) TestGetAsyncCharmDownloadInfo(c *gc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := applicationtesting.GenApplicationUUID(c)
	charmUUID := charmtesting.GenCharmID(c)

	info := application.CharmDownloadInfo{
		CharmUUID: charmUUID,
		Name:      "foo",
		SHA256:    "hash",
		DownloadInfo: applicationcharm.DownloadInfo{
			Provenance:         applicationcharm.ProvenanceDownload,
			CharmhubIdentifier: "foo",
			DownloadURL:        "https://example.com/foo",
			DownloadSize:       42,
		},
	}

	s.state.EXPECT().GetAsyncCharmDownloadInfo(gomock.Any(), appUUID).Return(info, nil)

	obtained, err := s.service.GetAsyncCharmDownloadInfo(context.Background(), appUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(obtained, gc.DeepEquals, info)
}

func (s *applicationServiceSuite) TestResolveCharmDownload(c *gc.C) {
	defer s.setupMocks(c).Finish()

	objectStoreUUID := objectstoretesting.GenObjectStoreUUID(c)

	dst := c.MkDir()
	path := testcharms.Repo.CharmArchivePath(dst, "dummy")

	// This will be removed once we get the information from charmhub store.
	// For now, just brute force our way through to get the actions.
	ch := testcharms.Repo.CharmDir("dummy")
	actions, err := encodeActions(ch.Actions())
	c.Assert(err, jc.ErrorIsNil)

	appUUID := applicationtesting.GenApplicationUUID(c)
	charmUUID := charmtesting.GenCharmID(c)

	info := application.CharmDownloadInfo{
		CharmUUID: charmUUID,
		Name:      "foo",
		SHA256:    "hash-256",
		DownloadInfo: applicationcharm.DownloadInfo{
			Provenance:         applicationcharm.ProvenanceDownload,
			CharmhubIdentifier: "foo",
			DownloadURL:        "https://example.com/foo",
			DownloadSize:       42,
		},
	}

	s.state.EXPECT().GetAsyncCharmDownloadInfo(gomock.Any(), appUUID).Return(info, nil)
	s.charmStore.EXPECT().Store(gomock.Any(), path, int64(42), "hash-384").Return(store.StoreResult{
		UniqueName:      "somepath",
		ObjectStoreUUID: objectStoreUUID,
	}, nil)
	s.state.EXPECT().ResolveCharmDownload(gomock.Any(), charmUUID, application.ResolvedCharmDownload{
		Actions:         actions,
		ObjectStoreUUID: objectStoreUUID,
		ArchivePath:     "somepath",
	})

	err = s.service.ResolveCharmDownload(context.Background(), appUUID, application.ResolveCharmDownload{
		CharmUUID: charmUUID,
		SHA256:    "hash-256",
		SHA384:    "hash-384",
		Path:      path,
		Size:      42,
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *applicationServiceSuite) TestResolveCharmDownloadInvalidApplicationID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	err := s.service.ResolveCharmDownload(context.Background(), "!!!!", application.ResolveCharmDownload{})
	c.Assert(err, jc.ErrorIs, jujuerrors.NotValid)
}

func (s *applicationServiceSuite) TestResolveCharmDownloadAlreadyAvailable(c *gc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := applicationtesting.GenApplicationUUID(c)
	charmUUID := charmtesting.GenCharmID(c)

	info := application.CharmDownloadInfo{
		CharmUUID: charmUUID,
		Name:      "foo",
		SHA256:    "hash",
		DownloadInfo: applicationcharm.DownloadInfo{
			Provenance:         applicationcharm.ProvenanceDownload,
			CharmhubIdentifier: "foo",
			DownloadURL:        "https://example.com/foo",
			DownloadSize:       42,
		},
	}

	s.state.EXPECT().GetAsyncCharmDownloadInfo(gomock.Any(), appUUID).Return(info, applicationerrors.CharmAlreadyAvailable)

	err := s.service.ResolveCharmDownload(context.Background(), appUUID, application.ResolveCharmDownload{
		CharmUUID: charmUUID,
		Path:      "foo",
		Size:      42,
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *applicationServiceSuite) TestResolveCharmDownloadAlreadyResolved(c *gc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := applicationtesting.GenApplicationUUID(c)
	charmUUID := charmtesting.GenCharmID(c)

	info := application.CharmDownloadInfo{
		CharmUUID: charmUUID,
		Name:      "foo",
		SHA256:    "hash",
		DownloadInfo: applicationcharm.DownloadInfo{
			Provenance:         applicationcharm.ProvenanceDownload,
			CharmhubIdentifier: "foo",
			DownloadURL:        "https://example.com/foo",
			DownloadSize:       42,
		},
	}

	s.state.EXPECT().GetAsyncCharmDownloadInfo(gomock.Any(), appUUID).Return(info, applicationerrors.CharmAlreadyResolved)

	err := s.service.ResolveCharmDownload(context.Background(), appUUID, application.ResolveCharmDownload{
		CharmUUID: charmUUID,
		Path:      "foo",
		Size:      42,
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *applicationServiceSuite) TestResolveCharmDownloadCharmUUIDMismatch(c *gc.C) {
	defer s.setupMocks(c).Finish()

	dst := c.MkDir()
	path := testcharms.Repo.CharmArchivePath(dst, "dummy")

	appUUID := applicationtesting.GenApplicationUUID(c)
	charmUUID := charmtesting.GenCharmID(c)

	info := application.CharmDownloadInfo{
		CharmUUID: "blah",
		Name:      "foo",
		SHA256:    "hash",
		DownloadInfo: applicationcharm.DownloadInfo{
			Provenance:         applicationcharm.ProvenanceDownload,
			CharmhubIdentifier: "foo",
			DownloadURL:        "https://example.com/foo",
			DownloadSize:       42,
		},
	}

	s.state.EXPECT().GetAsyncCharmDownloadInfo(gomock.Any(), appUUID).Return(info, nil)

	err := s.service.ResolveCharmDownload(context.Background(), appUUID, application.ResolveCharmDownload{
		CharmUUID: charmUUID,
		Path:      path,
		Size:      42,
	})
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmNotResolved)
}

func (s *applicationServiceSuite) TestResolveCharmDownloadNotStored(c *gc.C) {
	defer s.setupMocks(c).Finish()

	dst := c.MkDir()
	path := testcharms.Repo.CharmArchivePath(dst, "dummy")

	appUUID := applicationtesting.GenApplicationUUID(c)
	charmUUID := charmtesting.GenCharmID(c)

	info := application.CharmDownloadInfo{
		CharmUUID: charmUUID,
		Name:      "foo",
		SHA256:    "hash-256",
		DownloadInfo: applicationcharm.DownloadInfo{
			Provenance:         applicationcharm.ProvenanceDownload,
			CharmhubIdentifier: "foo",
			DownloadURL:        "https://example.com/foo",
			DownloadSize:       42,
		},
	}

	s.state.EXPECT().GetAsyncCharmDownloadInfo(gomock.Any(), appUUID).Return(info, nil)
	s.charmStore.EXPECT().Store(gomock.Any(), path, int64(42), "hash-384").Return(store.StoreResult{}, jujuerrors.NotFoundf("not found"))

	err := s.service.ResolveCharmDownload(context.Background(), appUUID, application.ResolveCharmDownload{
		CharmUUID: charmUUID,
		SHA256:    "hash-256",
		SHA384:    "hash-384",
		Path:      path,
		Size:      42,
	})
	c.Assert(err, jc.ErrorIs, jujuerrors.NotFound)
}

func (s *applicationServiceSuite) TestResolveCharmDownloadAlreadyStored(c *gc.C) {
	defer s.setupMocks(c).Finish()

	objectStoreUUID := objectstoretesting.GenObjectStoreUUID(c)

	dst := c.MkDir()
	path := testcharms.Repo.CharmArchivePath(dst, "dummy")

	// This will be removed once we get the information from charmhub store.
	// For now, just brute force our way through to get the actions.
	ch := testcharms.Repo.CharmDir("dummy")
	actions, err := encodeActions(ch.Actions())
	c.Assert(err, jc.ErrorIsNil)

	appUUID := applicationtesting.GenApplicationUUID(c)
	charmUUID := charmtesting.GenCharmID(c)

	info := application.CharmDownloadInfo{
		CharmUUID: charmUUID,
		Name:      "foo",
		SHA256:    "hash-256",
		DownloadInfo: applicationcharm.DownloadInfo{
			Provenance:         applicationcharm.ProvenanceDownload,
			CharmhubIdentifier: "foo",
			DownloadURL:        "https://example.com/foo",
			DownloadSize:       42,
		},
	}

	s.state.EXPECT().GetAsyncCharmDownloadInfo(gomock.Any(), appUUID).Return(info, nil)
	s.charmStore.EXPECT().Store(gomock.Any(), path, int64(42), "hash-384").Return(store.StoreResult{
		UniqueName:      "somepath",
		ObjectStoreUUID: objectStoreUUID,
	}, nil)
	s.state.EXPECT().ResolveCharmDownload(gomock.Any(), charmUUID, application.ResolvedCharmDownload{
		Actions:         actions,
		ObjectStoreUUID: objectStoreUUID,
		ArchivePath:     "somepath",
	})

	err = s.service.ResolveCharmDownload(context.Background(), appUUID, application.ResolveCharmDownload{
		CharmUUID: charmUUID,
		SHA256:    "hash-256",
		SHA384:    "hash-384",
		Path:      path,
		Size:      42,
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *applicationServiceSuite) TestGetApplicationsForRevisionUpdater(c *gc.C) {
	defer s.setupMocks(c).Finish()

	apps := []application.RevisionUpdaterApplication{
		{
			Name: "foo",
		},
	}

	s.state.EXPECT().GetApplicationsForRevisionUpdater(gomock.Any()).Return(apps, nil)

	results, err := s.service.GetApplicationsForRevisionUpdater(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(results, gc.DeepEquals, apps)
}

func (s *applicationServiceSuite) TestGetApplicationConfig(c *gc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := applicationtesting.GenApplicationUUID(c)

	s.state.EXPECT().GetApplicationConfigAndSettings(gomock.Any(), appUUID).Return(map[string]application.ApplicationConfig{
		"foo": {
			Type:  applicationcharm.OptionString,
			Value: "bar",
		},
	}, application.ApplicationSettings{
		Trust: true,
	}, nil)

	results, err := s.service.GetApplicationConfig(context.Background(), appUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(results, gc.DeepEquals, config.ConfigAttributes{
		"foo":   "bar",
		"trust": true,
	})
}

func (s *applicationServiceSuite) TestGetApplicationConfigWithError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := applicationtesting.GenApplicationUUID(c)

	s.state.EXPECT().GetApplicationConfigAndSettings(gomock.Any(), appUUID).Return(map[string]application.ApplicationConfig{
		"foo": {
			Type:  applicationcharm.OptionString,
			Value: "bar",
		},
	}, application.ApplicationSettings{
		Trust: true,
	}, errors.Errorf("boom"))

	_, err := s.service.GetApplicationConfig(context.Background(), appUUID)
	c.Assert(err, gc.ErrorMatches, "boom")
}

func (s *applicationServiceSuite) TestGetApplicationConfigNoConfig(c *gc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := applicationtesting.GenApplicationUUID(c)

	s.state.EXPECT().GetApplicationConfigAndSettings(gomock.Any(), appUUID).
		Return(map[string]application.ApplicationConfig{}, application.ApplicationSettings{}, nil)

	results, err := s.service.GetApplicationConfig(context.Background(), appUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(results, gc.DeepEquals, config.ConfigAttributes{
		"trust": false,
	})
}

func (s *applicationServiceSuite) TestGetApplicationConfigNoConfigWithTrust(c *gc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := applicationtesting.GenApplicationUUID(c)

	s.state.EXPECT().GetApplicationConfigAndSettings(gomock.Any(), appUUID).
		Return(map[string]application.ApplicationConfig{}, application.ApplicationSettings{
			Trust: true,
		}, nil)

	results, err := s.service.GetApplicationConfig(context.Background(), appUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(results, gc.DeepEquals, config.ConfigAttributes{
		"trust": true,
	})
}

func (s *applicationServiceSuite) TestGetApplicationConfigInvalidApplicationID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.service.GetApplicationConfig(context.Background(), "!!!")
	c.Assert(err, jc.ErrorIs, jujuerrors.NotValid)
}

func (s *applicationServiceSuite) TestGetApplicationTrustSetting(c *gc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := applicationtesting.GenApplicationUUID(c)

	s.state.EXPECT().GetApplicationTrustSetting(gomock.Any(), appUUID).Return(true, nil)

	results, err := s.service.GetApplicationTrustSetting(context.Background(), appUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(results, jc.IsTrue)
}

func (s *applicationServiceSuite) TestGetApplicationTrustSettingInvalidApplicationID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.service.GetApplicationTrustSetting(context.Background(), "!!!")
	c.Assert(err, jc.ErrorIs, jujuerrors.NotValid)
}

func (s *applicationServiceSuite) TestUnsetApplicationConfigKeys(c *gc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := applicationtesting.GenApplicationUUID(c)

	s.state.EXPECT().UnsetApplicationConfigKeys(gomock.Any(), appUUID, []string{"a", "b"}).Return(nil)

	err := s.service.UnsetApplicationConfigKeys(context.Background(), appUUID, []string{"a", "b"})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *applicationServiceSuite) TestUnsetApplicationConfigKeysNoValues(c *gc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := applicationtesting.GenApplicationUUID(c)

	err := s.service.UnsetApplicationConfigKeys(context.Background(), appUUID, []string{})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *applicationServiceSuite) TestUnsetApplicationConfigKeysInvalidApplicationID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	err := s.service.UnsetApplicationConfigKeys(context.Background(), "!!!", []string{"a", "b"})
	c.Assert(err, jc.ErrorIs, jujuerrors.NotValid)
}

func (s *applicationServiceSuite) TestSetApplicationConfig(c *gc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := applicationtesting.GenApplicationUUID(c)
	charmUUID := charmtesting.GenCharmID(c)

	s.state.EXPECT().GetCharmConfigByApplicationID(gomock.Any(), appUUID).Return(charmUUID, applicationcharm.Config{
		Options: map[string]applicationcharm.Option{
			"foo": {
				Type:    applicationcharm.OptionString,
				Default: "baz",
			},
		},
	}, nil)
	s.state.EXPECT().SetApplicationConfigAndSettings(gomock.Any(), appUUID, charmUUID, map[string]application.ApplicationConfig{
		"foo": {
			Type:  applicationcharm.OptionString,
			Value: "bar",
		},
	}, application.ApplicationSettings{
		Trust: true,
	}).Return(nil)

	err := s.service.SetApplicationConfig(context.Background(), appUUID, map[string]string{
		"trust": "true",
		"foo":   "bar",
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *applicationServiceSuite) TestSetApplicationConfigNoCharmConfig(c *gc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := applicationtesting.GenApplicationUUID(c)
	charmUUID := charmtesting.GenCharmID(c)

	s.state.EXPECT().GetCharmConfigByApplicationID(gomock.Any(), appUUID).Return(
		charmUUID,
		applicationcharm.Config{},
		applicationerrors.CharmNotFound,
	)

	err := s.service.SetApplicationConfig(context.Background(), appUUID, map[string]string{
		"trust": "true",
		"foo":   "bar",
	})
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmNotFound)
}

func (s *applicationServiceSuite) TestSetApplicationConfigWithNoCharmConfig(c *gc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := applicationtesting.GenApplicationUUID(c)
	charmUUID := charmtesting.GenCharmID(c)

	s.state.EXPECT().GetCharmConfigByApplicationID(gomock.Any(), appUUID).Return(charmUUID, applicationcharm.Config{
		Options: map[string]applicationcharm.Option{},
	}, nil)

	err := s.service.SetApplicationConfig(context.Background(), appUUID, map[string]string{
		"trust": "true",
		"foo":   "bar",
	})
	c.Assert(err, jc.ErrorIs, applicationerrors.InvalidCharmConfig)
}

func (s *applicationServiceSuite) TestSetApplicationConfigInvalidOptionType(c *gc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := applicationtesting.GenApplicationUUID(c)
	charmUUID := charmtesting.GenCharmID(c)

	s.state.EXPECT().GetCharmConfigByApplicationID(gomock.Any(), appUUID).Return(charmUUID, applicationcharm.Config{
		Options: map[string]applicationcharm.Option{
			"foo": {
				Type:    "blah",
				Default: "baz",
			},
		},
	}, nil)

	err := s.service.SetApplicationConfig(context.Background(), appUUID, map[string]string{
		"trust": "true",
		"foo":   "bar",
	})
	c.Assert(err, gc.ErrorMatches, `.*unknown option type "blah"`)
}

func (s *applicationServiceSuite) TestSetApplicationConfigInvalidTrustType(c *gc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := applicationtesting.GenApplicationUUID(c)
	charmUUID := charmtesting.GenCharmID(c)

	s.state.EXPECT().GetCharmConfigByApplicationID(gomock.Any(), appUUID).Return(charmUUID, applicationcharm.Config{
		Options: map[string]applicationcharm.Option{
			"foo": {
				Type:    "string",
				Default: "baz",
			},
		},
	}, nil)

	err := s.service.SetApplicationConfig(context.Background(), appUUID, map[string]string{
		"trust": "FOO",
		"foo":   "bar",
	})
	c.Assert(err, gc.ErrorMatches, `.*parsing trust setting.*`)
}

func (s *applicationServiceSuite) TestSetApplicationConfigNoConfig(c *gc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := applicationtesting.GenApplicationUUID(c)
	charmUUID := charmtesting.GenCharmID(c)

	s.state.EXPECT().GetCharmConfigByApplicationID(gomock.Any(), appUUID).Return(charmUUID, applicationcharm.Config{}, nil)
	s.state.EXPECT().SetApplicationConfigAndSettings(
		gomock.Any(), appUUID, charmUUID,
		map[string]application.ApplicationConfig{},
		application.ApplicationSettings{},
	).Return(nil)

	err := s.service.SetApplicationConfig(context.Background(), appUUID, map[string]string{})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *applicationServiceSuite) TestSetApplicationConfigInvalidApplicationID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	err := s.service.SetApplicationConfig(context.Background(), "!!!", nil)
	c.Assert(err, jc.ErrorIs, jujuerrors.NotValid)
}

func (s *applicationServiceSuite) TestGetApplicationAndCharmConfig(c *gc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := applicationtesting.GenApplicationUUID(c)
	charmUUID := charmtesting.GenCharmID(c)

	appConfig := map[string]application.ApplicationConfig{
		"foo": {
			Type:  applicationcharm.OptionString,
			Value: "bar",
		},
	}
	settings := application.ApplicationSettings{
		Trust: true,
	}
	charmConfig := applicationcharm.Config{
		Options: map[string]applicationcharm.Option{
			"foo": {
				Type:    applicationcharm.OptionString,
				Default: "baz",
			},
		},
	}
	charmOrigin := application.CharmOrigin{
		Name:   "foo",
		Source: applicationcharm.CharmHubSource,
		Platform: application.Platform{
			Architecture: architecture.AMD64,
			Channel:      "stable",
			OSType:       application.Ubuntu,
		},
		Channel: &application.Channel{
			Risk: application.RiskStable,
		},
	}

	s.state.EXPECT().GetApplicationConfigAndSettings(gomock.Any(), appUUID).Return(appConfig, settings, nil)
	s.state.EXPECT().GetCharmConfigByApplicationID(gomock.Any(), appUUID).Return(charmUUID, charmConfig, nil)
	s.state.EXPECT().IsSubordinateCharm(gomock.Any(), charmUUID).Return(false, nil)
	s.state.EXPECT().GetApplicationCharmOrigin(gomock.Any(), appUUID).Return(charmOrigin, nil)

	results, err := s.service.GetApplicationAndCharmConfig(context.Background(), appUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(results, gc.DeepEquals, ApplicationConfig{
		ApplicationConfig: config.ConfigAttributes{
			"foo": "bar",
		},
		CharmConfig: charm.Config{
			Options: map[string]charm.Option{
				"foo": {
					Type:    "string",
					Default: "baz",
				},
			},
		},
		Trust:     true,
		Principal: true,
		CharmName: "foo",
		CharmOrigin: corecharm.Origin{
			Source: corecharm.CharmHub,
			Platform: corecharm.Platform{
				Architecture: arch.AMD64,
				Channel:      "stable",
				OS:           "Ubuntu",
			},
			Channel: &charm.Channel{
				Risk: charm.Stable,
			},
		},
	})
}

func (s *applicationServiceSuite) TestGetApplicationAndCharmConfigInvalidID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.service.GetApplicationAndCharmConfig(context.Background(), "!!!")
	c.Assert(err, jc.ErrorIs, jujuerrors.NotValid)
}

func (s *applicationServiceSuite) TestGetApplicationAndCharmConfigNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := applicationtesting.GenApplicationUUID(c)

	appConfig := map[string]application.ApplicationConfig{
		"foo": {
			Type:  applicationcharm.OptionString,
			Value: "bar",
		},
	}
	settings := application.ApplicationSettings{
		Trust: true,
	}

	s.state.EXPECT().GetApplicationConfigAndSettings(gomock.Any(), appUUID).Return(appConfig, settings, applicationerrors.ApplicationNotFound)

	_, err := s.service.GetApplicationAndCharmConfig(context.Background(), appUUID)
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationNotFound)
}

type applicationWatcherServiceSuite struct {
	testing.IsolationSuite

	service *WatchableService

	state          *MockState
	charm          *MockCharm
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

	// Ensure order is preserved if the state returns the uuids in an unexpected
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

	// Ensure order is preserved if the state returns the uuids in an unexpected
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

	// Ensure order is preserved if the state returns the uuids in an unexpected
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

	s.state = NewMockState(ctrl)
	s.charm = NewMockCharm(ctrl)
	s.watcherFactory = NewMockWatcherFactory(ctrl)

	registry := corestorage.ConstModelStorageRegistry(func() storage.ProviderRegistry {
		return storage.ChainedProviderRegistry{
			dummystorage.StorageProviders(),
			provider.CommonStorageProviders(),
		}
	})

	modelUUID := modeltesting.GenModelUUID(c)

	s.clock = testclock.NewClock(time.Time{})
	s.service = NewWatchableService(
		s.state,
		domaintesting.NoopLeaderEnsurer(),
		registry,
		modelUUID,
		s.watcherFactory,
		nil,
		nil,
		nil,
		nil,
		domain.NewStatusHistory(loggertesting.WrapCheckLog(c), clock.WallClock),
		s.clock,
		loggertesting.WrapCheckLog(c),
	)
	s.service.clock = s.clock

	return ctrl
}
