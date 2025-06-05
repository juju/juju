// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"
	"time"

	"github.com/juju/clock"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	modeltesting "github.com/juju/juju/core/model/testing"
	corestatus "github.com/juju/juju/core/status"
	storagetesting "github.com/juju/juju/core/storage/testing"
	"github.com/juju/juju/domain/status"
	storageerrors "github.com/juju/juju/domain/storage/errors"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/statushistory"
)

type storageServiceSuite struct {
	state         *MockState
	statusHistory *statusHistoryRecorder

	service *Service
}

func TestStorageServiceSuite(t *testing.T) {
	tc.Run(t, &storageServiceSuite{})
}

func (s *storageServiceSuite) TestSetFilesystemStatus(c *tc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now()

	filesystemUUID := storagetesting.GenFilesystemUUID(c)
	s.state.EXPECT().GetFilesystemUUIDByID(gomock.Any(), "666").Return(filesystemUUID, nil)
	s.state.EXPECT().SetFilesystemStatus(gomock.Any(), filesystemUUID, status.StatusInfo[status.StorageFilesystemStatusType]{
		Status:  status.StorageFilesystemStatusTypeAttached,
		Message: "doink",
		Data:    []byte(`{"foo":"bar"}`),
		Since:   &now,
	})

	err := s.service.SetFilesystemStatus(c.Context(), "666", corestatus.StatusInfo{
		Status:  corestatus.Attached,
		Message: "doink",
		Data:    map[string]any{"foo": "bar"},
		Since:   &now,
	})
	c.Assert(err, tc.ErrorIsNil)

	c.Check(s.statusHistory.records, tc.DeepEquals, []statusHistoryRecord{{
		ns: statushistory.Namespace{Kind: corestatus.KindFilesystem, ID: filesystemUUID.String()},
		s: corestatus.StatusInfo{
			Status:  corestatus.Attached,
			Message: "doink",
			Data:    map[string]any{"foo": "bar"},
			Since:   &now,
		},
	}})
}

func (s *storageServiceSuite) TestSetFilesystemStatusNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetFilesystemUUIDByID(gomock.Any(), "666").Return("", storageerrors.FilesystemNotFound)

	err := s.service.SetFilesystemStatus(c.Context(), "666", corestatus.StatusInfo{
		Status: corestatus.Attached,
	})
	c.Assert(err, tc.ErrorIs, storageerrors.FilesystemNotFound)
}

func (s *storageServiceSuite) TestSetFilesystemStatusInvalidStatus(c *tc.C) {
	defer s.setupMocks(c).Finish()

	err := s.service.SetFilesystemStatus(c.Context(), "666", corestatus.StatusInfo{
		Status: "invalid",
	})
	c.Assert(err, tc.ErrorMatches, `.*unknown filesystem status "invalid"`)

	err = s.service.SetFilesystemStatus(c.Context(), "666", corestatus.StatusInfo{
		Status: corestatus.Allocating,
	})
	c.Assert(err, tc.ErrorMatches, `.*unknown filesystem status "allocating"`)
}

func (s *storageServiceSuite) TestSetVolumeStatus(c *tc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now()

	volumeUUID := storagetesting.GenVolumeUUID(c)
	s.state.EXPECT().GetVolumeUUIDByID(gomock.Any(), "666").Return(volumeUUID, nil)
	s.state.EXPECT().SetVolumeStatus(gomock.Any(), volumeUUID, status.StatusInfo[status.StorageVolumeStatusType]{
		Status:  status.StorageVolumeStatusTypeAttached,
		Message: "doink",
		Data:    []byte(`{"foo":"bar"}`),
		Since:   &now,
	})

	err := s.service.SetVolumeStatus(c.Context(), "666", corestatus.StatusInfo{
		Status:  corestatus.Attached,
		Message: "doink",
		Data:    map[string]any{"foo": "bar"},
		Since:   &now,
	})
	c.Assert(err, tc.ErrorIsNil)

	c.Check(s.statusHistory.records, tc.DeepEquals, []statusHistoryRecord{{
		ns: statushistory.Namespace{Kind: corestatus.KindVolume, ID: volumeUUID.String()},
		s: corestatus.StatusInfo{
			Status:  corestatus.Attached,
			Message: "doink",
			Data:    map[string]any{"foo": "bar"},
			Since:   &now,
		},
	}})
}

func (s *storageServiceSuite) TestSetVolumeStatusNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetVolumeUUIDByID(gomock.Any(), "666").Return("", storageerrors.VolumeNotFound)

	err := s.service.SetVolumeStatus(c.Context(), "666", corestatus.StatusInfo{
		Status: corestatus.Attached,
	})
	c.Assert(err, tc.ErrorIs, storageerrors.VolumeNotFound)
}

func (s *storageServiceSuite) TestSetVolumeStatusInvalidStatus(c *tc.C) {
	defer s.setupMocks(c).Finish()

	err := s.service.SetVolumeStatus(c.Context(), "666", corestatus.StatusInfo{
		Status: "invalid",
	})
	c.Assert(err, tc.ErrorMatches, `.*unknown volume status "invalid"`)

	err = s.service.SetVolumeStatus(c.Context(), "666", corestatus.StatusInfo{
		Status: corestatus.Allocating,
	})
	c.Assert(err, tc.ErrorMatches, `.*unknown volume status "allocating"`)
}

func (s *storageServiceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockState(ctrl)
	s.statusHistory = &statusHistoryRecorder{}

	s.service = NewService(
		s.state,
		modeltesting.GenModelUUID(c),
		s.statusHistory,
		func() (StatusHistoryReader, error) {
			return nil, errors.Errorf("status history reader not available")
		},
		clock.WallClock,
		loggertesting.WrapCheckLog(c),
	)

	return ctrl
}
