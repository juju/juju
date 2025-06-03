// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	corecharm "github.com/juju/juju/core/charm"
	coremachine "github.com/juju/juju/core/machine"
	corestatus "github.com/juju/juju/core/status"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/architecture"
	"github.com/juju/juju/domain/deployment"
	"github.com/juju/juju/domain/status"
	internalcharm "github.com/juju/juju/internal/charm"
)

type serviceSuite struct {
	baseSuite
}

func TestServiceSuite(t *testing.T) {
	tc.Run(t, &serviceSuite{})
}

func (s *serviceSuite) TestEncodeChannelAndPlatform(c *tc.C) {
	ch, pl, err := encodeChannelAndPlatform(corecharm.Origin{
		Channel: ptr(internalcharm.MakePermissiveChannel("track", "stable", "branch")),
		Platform: corecharm.Platform{
			Architecture: "amd64",
			OS:           "ubuntu",
			Channel:      "24.04",
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(ch, tc.DeepEquals, &deployment.Channel{
		Track:  "track",
		Risk:   deployment.RiskStable,
		Branch: "branch",
	})
	c.Check(pl, tc.DeepEquals, deployment.Platform{
		Architecture: architecture.AMD64,
		OSType:       deployment.Ubuntu,
		Channel:      "24.04",
	})
}

func (s *serviceSuite) TestEncodeChannelAndPlatformInvalidArch(c *tc.C) {
	ch, pl, err := encodeChannelAndPlatform(corecharm.Origin{
		Channel: ptr(internalcharm.MakePermissiveChannel("track", "stable", "branch")),
		Platform: corecharm.Platform{
			Architecture: "armhf",
			OS:           "ubuntu",
			Channel:      "24.04",
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(ch, tc.DeepEquals, &deployment.Channel{
		Track:  "track",
		Risk:   deployment.RiskStable,
		Branch: "branch",
	})
	c.Check(pl, tc.DeepEquals, deployment.Platform{
		Architecture: architecture.Unknown,
		OSType:       deployment.Ubuntu,
		Channel:      "24.04",
	})
}

func (s *serviceSuite) TestEncodeChannelAndPlatformInvalidRisk(c *tc.C) {
	_, _, err := encodeChannelAndPlatform(corecharm.Origin{
		Channel: ptr(internalcharm.MakePermissiveChannel("track", "blah", "branch")),
		Platform: corecharm.Platform{
			Architecture: "armhf",
			OS:           "ubuntu",
			Channel:      "24.04",
		},
	})
	c.Assert(err, tc.ErrorMatches, `unknown risk.*`)
}

func (s *serviceSuite) TestEncodeChannelAndPlatformInvalidOSType(c *tc.C) {
	_, _, err := encodeChannelAndPlatform(corecharm.Origin{
		Channel: ptr(internalcharm.MakePermissiveChannel("track", "stable", "branch")),
		Platform: corecharm.Platform{
			Architecture: "armhf",
			OS:           "windows",
			Channel:      "24.04",
		},
	})
	c.Assert(err, tc.ErrorMatches, `unknown os type.*`)
}

func (s *serviceSuite) TestRecordUnitStatusHistory(c *tc.C) {
	var statusHistory *MockStatusHistory
	defer s.setupMocksWithStatusHistory(c, func(c *gomock.Controller) StatusHistory {
		statusHistory = NewMockStatusHistory(c)
		return statusHistory
	}).Finish()

	statusHistory.EXPECT().RecordStatus(c.Context(), status.UnitAgentNamespace.WithID("foo/0"), corestatus.StatusInfo{
		Status: corestatus.Allocating,
	})
	statusHistory.EXPECT().RecordStatus(c.Context(), status.UnitWorkloadNamespace.WithID("foo/0"), corestatus.StatusInfo{
		Status:  corestatus.Active,
		Message: "message",
	})

	err := s.service.recordUnitStatusHistory(c.Context(), unit.Name("foo/0"), application.UnitStatusArg{
		AgentStatus: &status.StatusInfo[status.UnitAgentStatusType]{
			Status: status.UnitAgentStatusAllocating,
		},
		WorkloadStatus: &status.StatusInfo[status.WorkloadStatusType]{
			Status:  status.WorkloadStatusActive,
			Message: "message",
		},
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestRecordUnitStatusHistoryEmptyAgentStatus(c *tc.C) {
	var statusHistory *MockStatusHistory
	defer s.setupMocksWithStatusHistory(c, func(c *gomock.Controller) StatusHistory {
		statusHistory = NewMockStatusHistory(c)
		return statusHistory
	}).Finish()

	err := s.service.recordUnitStatusHistory(c.Context(), unit.Name("foo/0"), application.UnitStatusArg{
		AgentStatus: &status.StatusInfo[status.UnitAgentStatusType]{
			Status: status.UnitAgentStatusAllocating,
		},
	})
	c.Assert(err, tc.NotNil)
}

func (s *serviceSuite) TestRecordUnitStatusHistoryEmptyWorkloadStatus(c *tc.C) {
	var statusHistory *MockStatusHistory
	defer s.setupMocksWithStatusHistory(c, func(c *gomock.Controller) StatusHistory {
		statusHistory = NewMockStatusHistory(c)
		return statusHistory
	}).Finish()

	err := s.service.recordUnitStatusHistory(c.Context(), unit.Name("foo/0"), application.UnitStatusArg{
		WorkloadStatus: &status.StatusInfo[status.WorkloadStatusType]{
			Status:  status.WorkloadStatusActive,
			Message: "message",
		},
	})
	c.Assert(err, tc.NotNil)
}

func (s *serviceSuite) TestRecordMachinesStatusHistory(c *tc.C) {
	var statusHistory *MockStatusHistory
	defer s.setupMocksWithStatusHistory(c, func(c *gomock.Controller) StatusHistory {
		statusHistory = NewMockStatusHistory(c)
		return statusHistory
	}).Finish()

	now := s.clock.Now().UTC()
	statusHistory.EXPECT().RecordStatus(c.Context(), status.MachineNamespace.WithID("0"), corestatus.StatusInfo{
		Status: corestatus.Pending,
		Since:  ptr(now),
	})
	statusHistory.EXPECT().RecordStatus(c.Context(), status.MachineInstanceNamespace.WithID("0"), corestatus.StatusInfo{
		Status: corestatus.Pending,
		Since:  ptr(now),
	})
	statusHistory.EXPECT().RecordStatus(c.Context(), status.MachineNamespace.WithID("0/lxd/0"), corestatus.StatusInfo{
		Status: corestatus.Pending,
		Since:  ptr(now),
	})
	statusHistory.EXPECT().RecordStatus(c.Context(), status.MachineInstanceNamespace.WithID("0/lxd/0"), corestatus.StatusInfo{
		Status: corestatus.Pending,
		Since:  ptr(now),
	})

	s.service.recordInitMachinesStatusHistory(c.Context(), []coremachine.Name{
		coremachine.Name("0"),
		coremachine.Name("0/lxd/0"),
	})
}
