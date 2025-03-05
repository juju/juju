// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"time"

	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	coreapplication "github.com/juju/juju/core/application"
	applicationtesting "github.com/juju/juju/core/application/testing"
	coreconstraints "github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	corestatus "github.com/juju/juju/core/status"
	coreunit "github.com/juju/juju/core/unit"
	unittesting "github.com/juju/juju/core/unit/testing"
	"github.com/juju/juju/domain/application"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/constraints"
	"github.com/juju/juju/domain/life"
	"github.com/juju/juju/internal/statushistory"
)

type unitServiceSuite struct {
	baseSuite
}

var _ = gc.Suite(&unitServiceSuite{})

func (s *unitServiceSuite) TestAddUnitsEmptyConstraints(c *gc.C) {
	ctrl := s.setupMocksWithProvider(c, func(ctx context.Context) (Provider, error) {
		return s.provider, nil
	}, func(ctx context.Context) (SupportedFeatureProvider, error) {
		return s.supportedFeaturesProvider, nil
	})
	defer ctrl.Finish()

	appUUID := applicationtesting.GenApplicationUUID(c)
	unitUUID := unittesting.GenUnitUUID(c)

	now := ptr(s.clock.Now())
	u := []application.AddUnitArg{{
		UnitName: "ubuntu/666",
		UnitStatusArg: application.UnitStatusArg{
			AgentStatus: &application.StatusInfo[application.UnitAgentStatusType]{
				Status: application.UnitAgentStatusAllocating,
				Since:  now,
			},
			WorkloadStatus: &application.StatusInfo[application.WorkloadStatusType]{
				Status:  application.WorkloadStatusWaiting,
				Message: corestatus.MessageInstallingAgent,
				Since:   now,
			},
		},
	}}
	s.state.EXPECT().GetModelType(gomock.Any()).Return("caas", nil)
	s.state.EXPECT().GetApplicationIDByName(gomock.Any(), "ubuntu").Return(appUUID, nil)
	s.expectEmptyUnitConstraints(c, ctrl, unitUUID, appUUID)

	var received []application.AddUnitArg
	s.state.EXPECT().AddUnits(gomock.Any(), appUUID, gomock.Any()).DoAndReturn(func(_ context.Context, _ coreapplication.ID, args []application.AddUnitArg) error {
		received = args
		return nil
	})

	a := AddUnitArg{
		UnitName: "ubuntu/666",
	}
	err := s.service.AddUnits(context.Background(), "ubuntu", a)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(received, jc.DeepEquals, u)
}

func (s *unitServiceSuite) expectEmptyUnitConstraints(c *gc.C, ctrl *gomock.Controller, unitUUID coreunit.UUID, appUUID coreapplication.ID) {
	validator := NewMockValidator(ctrl)
	s.provider.EXPECT().ConstraintsValidator(gomock.Any()).Return(validator, nil)
	appConstraints := constraints.Constraints{}
	modelConstraints := constraints.Constraints{}
	s.state.EXPECT().GetApplicationConstraints(gomock.Any(), appUUID).Return(appConstraints, nil)
	s.state.EXPECT().GetModelConstraints(gomock.Any()).Return(modelConstraints, nil)
	validator.EXPECT().Merge(constraints.EncodeConstraints(appConstraints), constraints.EncodeConstraints(modelConstraints)).Return(coreconstraints.Value{}, nil)
}

func (s *unitServiceSuite) TestAddUnitsAppConstraints(c *gc.C) {
	ctrl := s.setupMocksWithProvider(c, func(ctx context.Context) (Provider, error) {
		return s.provider, nil
	}, func(ctx context.Context) (SupportedFeatureProvider, error) {
		return s.supportedFeaturesProvider, nil
	})
	defer ctrl.Finish()

	appUUID := applicationtesting.GenApplicationUUID(c)
	unitUUID := unittesting.GenUnitUUID(c)

	now := ptr(s.clock.Now())
	u := []application.AddUnitArg{{
		UnitName: "ubuntu/666",
		UnitStatusArg: application.UnitStatusArg{
			AgentStatus: &application.StatusInfo[application.UnitAgentStatusType]{
				Status: application.UnitAgentStatusAllocating,
				Since:  now,
			},
			WorkloadStatus: &application.StatusInfo[application.WorkloadStatusType]{
				Status:  application.WorkloadStatusWaiting,
				Message: corestatus.MessageInstallingAgent,
				Since:   now,
			},
		},
	}}
	s.state.EXPECT().GetModelType(gomock.Any()).Return("caas", nil)
	s.state.EXPECT().GetApplicationIDByName(gomock.Any(), "ubuntu").Return(appUUID, nil)
	s.expectAppConstraints(c, ctrl, unitUUID, appUUID)

	var received []application.AddUnitArg
	s.state.EXPECT().AddUnits(gomock.Any(), appUUID, gomock.Any()).DoAndReturn(func(_ context.Context, _ coreapplication.ID, args []application.AddUnitArg) error {
		received = args
		return nil
	})

	a := AddUnitArg{
		UnitName: "ubuntu/666",
	}
	err := s.service.AddUnits(context.Background(), "ubuntu", a)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(received, jc.DeepEquals, u)
}

func (s *unitServiceSuite) expectAppConstraints(c *gc.C, ctrl *gomock.Controller, unitUUID coreunit.UUID, appUUID coreapplication.ID) {
	validator := NewMockValidator(ctrl)
	s.provider.EXPECT().ConstraintsValidator(gomock.Any()).Return(validator, nil)
	appConstraints := constraints.Constraints{
		Arch:           ptr("amd64"),
		Container:      ptr(instance.LXD),
		CpuCores:       ptr(uint64(4)),
		Mem:            ptr(uint64(1024)),
		RootDisk:       ptr(uint64(1024)),
		RootDiskSource: ptr("root-disk-source"),
		Tags:           ptr([]string{"tag1", "tag2"}),
		InstanceRole:   ptr("instance-role"),
		InstanceType:   ptr("instance-type"),
		Spaces: ptr([]constraints.SpaceConstraint{
			{SpaceName: "space1", Exclude: false},
		}),
		VirtType:         ptr("virt-type"),
		Zones:            ptr([]string{"zone1", "zone2"}),
		AllocatePublicIP: ptr(true),
	}
	modelConstraints := constraints.Constraints{}
	s.state.EXPECT().GetApplicationConstraints(gomock.Any(), appUUID).Return(appConstraints, nil)
	s.state.EXPECT().GetModelConstraints(gomock.Any()).Return(modelConstraints, nil)
	unitConstraints := appConstraints
	validator.EXPECT().Merge(constraints.EncodeConstraints(appConstraints), constraints.EncodeConstraints(modelConstraints)).Return(constraints.EncodeConstraints(unitConstraints), nil)
	s.state.EXPECT().GetUnitUUIDByName(gomock.Any(), coreunit.Name("ubuntu/666")).Return(unitUUID, nil)
	s.state.EXPECT().SetUnitConstraints(gomock.Any(), unitUUID, unitConstraints)
}

func (s *unitServiceSuite) TestAddUnitsModelConstraints(c *gc.C) {
	ctrl := s.setupMocksWithProvider(c, func(ctx context.Context) (Provider, error) {
		return s.provider, nil
	}, func(ctx context.Context) (SupportedFeatureProvider, error) {
		return s.supportedFeaturesProvider, nil
	})
	defer ctrl.Finish()

	appUUID := applicationtesting.GenApplicationUUID(c)
	unitUUID := unittesting.GenUnitUUID(c)

	now := ptr(s.clock.Now())
	u := []application.AddUnitArg{{
		UnitName: "ubuntu/666",
		UnitStatusArg: application.UnitStatusArg{
			AgentStatus: &application.StatusInfo[application.UnitAgentStatusType]{
				Status: application.UnitAgentStatusAllocating,
				Since:  now,
			},
			WorkloadStatus: &application.StatusInfo[application.WorkloadStatusType]{
				Status:  application.WorkloadStatusWaiting,
				Message: corestatus.MessageInstallingAgent,
				Since:   now,
			},
		},
	}}
	s.state.EXPECT().GetModelType(gomock.Any()).Return("caas", nil)
	s.state.EXPECT().GetApplicationIDByName(gomock.Any(), "ubuntu").Return(appUUID, nil)
	s.expectModelConstraints(c, ctrl, unitUUID, appUUID)

	var received []application.AddUnitArg
	s.state.EXPECT().AddUnits(gomock.Any(), appUUID, gomock.Any()).DoAndReturn(func(_ context.Context, _ coreapplication.ID, args []application.AddUnitArg) error {
		received = args
		return nil
	})

	a := AddUnitArg{
		UnitName: "ubuntu/666",
	}
	err := s.service.AddUnits(context.Background(), "ubuntu", a)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(received, jc.DeepEquals, u)
}

func (s *unitServiceSuite) expectModelConstraints(c *gc.C, ctrl *gomock.Controller, unitUUID coreunit.UUID, appUUID coreapplication.ID) {
	validator := NewMockValidator(ctrl)
	s.provider.EXPECT().ConstraintsValidator(gomock.Any()).Return(validator, nil)
	modelConstraints := constraints.Constraints{
		Arch:           ptr("amd64"),
		Container:      ptr(instance.LXD),
		CpuCores:       ptr(uint64(4)),
		Mem:            ptr(uint64(1024)),
		RootDisk:       ptr(uint64(1024)),
		RootDiskSource: ptr("root-disk-source"),
		Tags:           ptr([]string{"tag1", "tag2"}),
		InstanceRole:   ptr("instance-role"),
		InstanceType:   ptr("instance-type"),
		Spaces: ptr([]constraints.SpaceConstraint{
			{SpaceName: "space1", Exclude: false},
		}),
		VirtType:         ptr("virt-type"),
		Zones:            ptr([]string{"zone1", "zone2"}),
		AllocatePublicIP: ptr(true),
	}
	appConstraints := constraints.Constraints{}
	s.state.EXPECT().GetApplicationConstraints(gomock.Any(), appUUID).Return(appConstraints, nil)
	s.state.EXPECT().GetModelConstraints(gomock.Any()).Return(modelConstraints, nil)
	unitConstraints := modelConstraints
	validator.EXPECT().Merge(constraints.EncodeConstraints(appConstraints), constraints.EncodeConstraints(modelConstraints)).Return(constraints.EncodeConstraints(unitConstraints), nil)
	s.state.EXPECT().GetUnitUUIDByName(gomock.Any(), coreunit.Name("ubuntu/666")).Return(unitUUID, nil)
	s.state.EXPECT().SetUnitConstraints(gomock.Any(), unitUUID, unitConstraints)
}

func (s *unitServiceSuite) TestAddUnitsFullConstraints(c *gc.C) {
	ctrl := s.setupMocksWithProvider(c, func(ctx context.Context) (Provider, error) {
		return s.provider, nil
	}, func(ctx context.Context) (SupportedFeatureProvider, error) {
		return s.supportedFeaturesProvider, nil
	})
	defer ctrl.Finish()

	appUUID := applicationtesting.GenApplicationUUID(c)
	unitUUID := unittesting.GenUnitUUID(c)

	now := ptr(s.clock.Now())
	u := []application.AddUnitArg{{
		UnitName: "ubuntu/666",
		UnitStatusArg: application.UnitStatusArg{
			AgentStatus: &application.StatusInfo[application.UnitAgentStatusType]{
				Status: application.UnitAgentStatusAllocating,
				Since:  now,
			},
			WorkloadStatus: &application.StatusInfo[application.WorkloadStatusType]{
				Status:  application.WorkloadStatusWaiting,
				Message: corestatus.MessageInstallingAgent,
				Since:   now,
			},
		},
	}}
	s.state.EXPECT().GetModelType(gomock.Any()).Return("caas", nil)
	s.state.EXPECT().GetApplicationIDByName(gomock.Any(), "ubuntu").Return(appUUID, nil)
	s.expectFullConstraints(c, ctrl, unitUUID, appUUID)

	var received []application.AddUnitArg
	s.state.EXPECT().AddUnits(gomock.Any(), appUUID, gomock.Any()).DoAndReturn(func(_ context.Context, _ coreapplication.ID, args []application.AddUnitArg) error {
		received = args
		return nil
	})

	a := AddUnitArg{
		UnitName: "ubuntu/666",
	}
	err := s.service.AddUnits(context.Background(), "ubuntu", a)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(received, jc.DeepEquals, u)
}

func (s *unitServiceSuite) expectFullConstraints(c *gc.C, ctrl *gomock.Controller, unitUUID coreunit.UUID, appUUID coreapplication.ID) {
	validator := NewMockValidator(ctrl)
	s.provider.EXPECT().ConstraintsValidator(gomock.Any()).Return(validator, nil)
	modelConstraints := constraints.Constraints{
		CpuCores: ptr(uint64(4)),
	}
	appConstraints := constraints.Constraints{
		CpuPower: ptr(uint64(75)),
	}
	s.state.EXPECT().GetApplicationConstraints(gomock.Any(), appUUID).Return(appConstraints, nil)
	s.state.EXPECT().GetModelConstraints(gomock.Any()).Return(modelConstraints, nil)
	unitConstraints := constraints.Constraints{
		CpuCores: ptr(uint64(4)),
		CpuPower: ptr(uint64(75)),
	}
	validator.EXPECT().Merge(constraints.EncodeConstraints(appConstraints), constraints.EncodeConstraints(modelConstraints)).Return(constraints.EncodeConstraints(unitConstraints), nil)
	s.state.EXPECT().GetUnitUUIDByName(gomock.Any(), coreunit.Name("ubuntu/666")).Return(unitUUID, nil)
	s.state.EXPECT().SetUnitConstraints(gomock.Any(), unitUUID, unitConstraints)
}

func (s *unitServiceSuite) TestSetWorkloadUnitStatus(c *gc.C) {
	history := &statusHistoryRecorder{}

	defer s.setupMocksWithStatusHistory(c, history).Finish()

	now := time.Now()

	unitUUID := unittesting.GenUnitUUID(c)
	s.state.EXPECT().GetUnitUUIDByName(gomock.Any(), coreunit.Name("foo/666")).Return(unitUUID, nil)
	s.state.EXPECT().SetUnitWorkloadStatus(gomock.Any(), unitUUID, &application.StatusInfo[application.WorkloadStatusType]{
		Status:  application.WorkloadStatusActive,
		Message: "doink",
		Data:    []byte(`{"foo":"bar"}`),
		Since:   &now,
	})

	err := s.service.SetUnitWorkloadStatus(context.Background(), coreunit.Name("foo/666"), &corestatus.StatusInfo{
		Status:  corestatus.Active,
		Message: "doink",
		Data:    map[string]interface{}{"foo": "bar"},
		Since:   &now,
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Check(history.records, jc.DeepEquals, []statusHistoryRecord{{
		ns: statushistory.Namespace{Name: "unit-workload", ID: "foo/666"},
		s: corestatus.StatusInfo{
			Status:  corestatus.Active,
			Message: "doink",
			Data:    map[string]interface{}{"foo": "bar"},
			Since:   &now,
		},
	}})
}

func (s *unitServiceSuite) TestSetWorkloadUnitStatusInvalidStatus(c *gc.C) {
	defer s.setupMocks(c).Finish()

	err := s.service.SetUnitWorkloadStatus(context.Background(), coreunit.Name("foo/666"), &corestatus.StatusInfo{
		Status: corestatus.Status("invalid"),
	})
	c.Assert(err, gc.ErrorMatches, `.*unknown workload status "invalid"`)

	err = s.service.SetUnitWorkloadStatus(context.Background(), coreunit.Name("foo/666"), &corestatus.StatusInfo{
		Status: corestatus.Allocating,
	})
	c.Assert(err, gc.ErrorMatches, `.*unknown workload status "allocating"`)
}

func (s *unitServiceSuite) TestGetUnitWorkloadStatusesForApplication(c *gc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now()

	appUUID := applicationtesting.GenApplicationUUID(c)
	s.state.EXPECT().GetUnitWorkloadStatusesForApplication(gomock.Any(), appUUID).Return(
		map[coreunit.Name]application.StatusInfo[application.WorkloadStatusType]{
			"unit-1": {
				Status:  application.WorkloadStatusActive,
				Message: "doink",
				Data:    []byte(`{"foo":"bar"}`),
				Since:   &now,
			},
			"unit-2": {
				Status:  application.WorkloadStatusMaintenance,
				Message: "boink",
				Data:    []byte(`{"foo":"baz"}`),
				Since:   &now,
			},
		}, nil,
	)

	obtained, err := s.service.GetUnitWorkloadStatusesForApplication(context.Background(), appUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(obtained, jc.DeepEquals, map[coreunit.Name]corestatus.StatusInfo{
		"unit-1": {
			Status:  corestatus.Active,
			Message: "doink",
			Data:    map[string]interface{}{"foo": "bar"},
			Since:   &now,
		},
		"unit-2": {
			Status:  corestatus.Maintenance,
			Message: "boink",
			Data:    map[string]interface{}{"foo": "baz"},
			Since:   &now,
		},
	})
}

func (s *unitServiceSuite) TestGetUnitDisplayStatusNoContainer(c *gc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now()

	unitUUID := unittesting.GenUnitUUID(c)
	s.state.EXPECT().GetUnitUUIDByName(gomock.Any(), coreunit.Name("foo/666")).Return(unitUUID, nil)
	s.state.EXPECT().GetUnitWorkloadStatus(gomock.Any(), unitUUID).Return(
		&application.StatusInfo[application.WorkloadStatusType]{
			Status:  application.WorkloadStatusActive,
			Message: "doink",
			Data:    []byte(`{"foo":"bar"}`),
			Since:   &now,
		}, nil)

	s.state.EXPECT().GetUnitCloudContainerStatus(gomock.Any(), unitUUID).Return(nil, applicationerrors.UnitStatusNotFound)

	obtained, err := s.service.GetUnitDisplayStatus(context.Background(), coreunit.Name("foo/666"))
	c.Assert(err, jc.ErrorIsNil)
	c.Check(obtained, jc.DeepEquals, &corestatus.StatusInfo{
		Status:  corestatus.Active,
		Message: "doink",
		Data:    map[string]interface{}{"foo": "bar"},
		Since:   &now,
	})
}

func (s *unitServiceSuite) TestGetUnitDisplayStatusWithPrecedentContainer(c *gc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now()

	unitUUID := unittesting.GenUnitUUID(c)
	s.state.EXPECT().GetUnitUUIDByName(gomock.Any(), coreunit.Name("foo/666")).Return(unitUUID, nil)
	s.state.EXPECT().GetUnitWorkloadStatus(gomock.Any(), unitUUID).Return(
		&application.StatusInfo[application.WorkloadStatusType]{
			Status:  application.WorkloadStatusActive,
			Message: "doink",
			Data:    []byte(`{"foo":"bar"}`),
			Since:   &now,
		}, nil)

	s.state.EXPECT().GetUnitCloudContainerStatus(gomock.Any(), unitUUID).Return(
		&application.StatusInfo[application.CloudContainerStatusType]{
			Status:  application.CloudContainerStatusBlocked,
			Message: "boink",
			Data:    []byte(`{"foo":"baz"}`),
			Since:   &now,
		}, nil)

	obtained, err := s.service.GetUnitDisplayStatus(context.Background(), coreunit.Name("foo/666"))
	c.Assert(err, jc.ErrorIsNil)
	c.Check(obtained, jc.DeepEquals, &corestatus.StatusInfo{
		Status:  corestatus.Blocked,
		Message: "boink",
		Data:    map[string]interface{}{"foo": "baz"},
		Since:   &now,
	})
}

func (s *unitServiceSuite) TestGetUnitDisplayStatusWithPrecedentWorkload(c *gc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now()

	unitUUID := unittesting.GenUnitUUID(c)
	s.state.EXPECT().GetUnitUUIDByName(gomock.Any(), coreunit.Name("foo/666")).Return(unitUUID, nil)
	s.state.EXPECT().GetUnitWorkloadStatus(gomock.Any(), unitUUID).Return(
		&application.StatusInfo[application.WorkloadStatusType]{
			Status:  application.WorkloadStatusMaintenance,
			Message: "doink",
			Data:    []byte(`{"foo":"bar"}`),
			Since:   &now,
		}, nil)

	s.state.EXPECT().GetUnitCloudContainerStatus(gomock.Any(), unitUUID).Return(
		&application.StatusInfo[application.CloudContainerStatusType]{
			Status:  application.CloudContainerStatusRunning,
			Message: "boink",
			Data:    []byte(`{"foo":"baz"}`),
			Since:   &now,
		}, nil)

	obtained, err := s.service.GetUnitDisplayStatus(context.Background(), coreunit.Name("foo/666"))
	c.Assert(err, jc.ErrorIsNil)
	c.Check(obtained, jc.DeepEquals, &corestatus.StatusInfo{
		Status:  corestatus.Maintenance,
		Message: "doink",
		Data:    map[string]interface{}{"foo": "bar"},
		Since:   &now,
	})
}

func (s *unitServiceSuite) TestSetUnitPassword(c *gc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID := unittesting.GenUnitUUID(c)
	s.state.EXPECT().GetUnitUUIDByName(gomock.Any(), coreunit.Name("foo/666")).Return(unitUUID, nil)
	s.state.EXPECT().SetUnitPassword(gomock.Any(), unitUUID, application.PasswordInfo{
		PasswordHash:  "password",
		HashAlgorithm: 0,
	})

	err := s.service.SetUnitPassword(context.Background(), coreunit.Name("foo/666"), "password")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *unitServiceSuite) TestGetUnitUUID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	uuid := unittesting.GenUnitUUID(c)
	unitName := coreunit.Name("foo/666")
	s.state.EXPECT().GetUnitUUIDByName(gomock.Any(), unitName).Return(uuid, nil)

	u, err := s.service.GetUnitUUID(context.Background(), unitName)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(u, gc.Equals, uuid)
}

func (s *unitServiceSuite) TestGetUnitUUIDErrors(c *gc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("foo/666")
	s.state.EXPECT().GetUnitUUIDByName(gomock.Any(), unitName).Return("", applicationerrors.UnitNotFound)

	_, err := s.service.GetUnitUUID(context.Background(), unitName)
	c.Assert(err, jc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *unitServiceSuite) TestRegisterCAASUnit(c *gc.C) {
	defer s.setupMocks(c).Finish()

	appUUID := applicationtesting.GenApplicationUUID(c)

	p := application.RegisterCAASUnitArg{
		UnitName:     coreunit.Name("foo/666"),
		PasswordHash: "passwordhash",
		ProviderID:   "provider-id",
		Address:      ptr("10.6.6.6"),
		Ports:        ptr([]string{"666"}),
		OrderedScale: true,
		OrderedId:    1,
	}

	s.state.EXPECT().GetApplicationIDByName(gomock.Any(), "foo").Return(appUUID, nil)
	s.state.EXPECT().InsertCAASUnit(gomock.Any(), appUUID, p)

	err := s.service.RegisterCAASUnit(context.Background(), "foo", p)
	c.Assert(err, jc.ErrorIsNil)
}

var unitParams = application.RegisterCAASUnitArg{
	UnitName:     coreunit.Name("foo/666"),
	PasswordHash: "passwordhash",
	ProviderID:   "provider-id",
	OrderedScale: true,
	OrderedId:    1,
}

func (s *unitServiceSuite) TestRegisterCAASUnitMissingUnitName(c *gc.C) {
	defer s.setupMocks(c).Finish()

	p := unitParams
	p.UnitName = ""
	err := s.service.RegisterCAASUnit(context.Background(), "foo", p)
	c.Assert(err, gc.ErrorMatches, "missing unit name not valid")
}

func (s *unitServiceSuite) TestRegisterCAASUnitMissingOrderedScale(c *gc.C) {
	defer s.setupMocks(c).Finish()

	p := unitParams
	p.OrderedScale = false
	err := s.service.RegisterCAASUnit(context.Background(), "foo", p)
	c.Assert(err, gc.ErrorMatches, "registering CAAS units not supported without ordered unit IDs")
}

func (s *unitServiceSuite) TestRegisterCAASUnitMissingProviderID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	p := unitParams
	p.ProviderID = ""
	err := s.service.RegisterCAASUnit(context.Background(), "foo", p)
	c.Assert(err, gc.ErrorMatches, "provider id not valid")
}

func (s *unitServiceSuite) TestRegisterCAASUnitMissingPasswordHash(c *gc.C) {
	defer s.setupMocks(c).Finish()

	p := unitParams
	p.PasswordHash = ""
	err := s.service.RegisterCAASUnit(context.Background(), "foo", p)
	c.Assert(err, gc.ErrorMatches, "password hash not valid")
}

func (s *unitServiceSuite) TestUpdateCAASUnit(c *gc.C) {
	defer s.setupMocks(c).Finish()

	appID := applicationtesting.GenApplicationUUID(c)
	unitName := coreunit.Name("foo/666")
	now := time.Now()

	expected := application.UpdateCAASUnitParams{
		ProviderID: ptr("provider-id"),
		Address:    ptr("10.6.6.6"),
		Ports:      ptr([]string{"666"}),
		AgentStatus: ptr(application.StatusInfo[application.UnitAgentStatusType]{
			Status:  application.UnitAgentStatusAllocating,
			Message: "agent status",
			Data:    []byte(`{"foo":"bar"}`),
			Since:   ptr(now),
		}),
		WorkloadStatus: ptr(application.StatusInfo[application.WorkloadStatusType]{
			Status:  application.WorkloadStatusWaiting,
			Message: "workload status",
			Data:    []byte(`{"foo":"bar"}`),
			Since:   ptr(now),
		}),
		CloudContainerStatus: ptr(application.StatusInfo[application.CloudContainerStatusType]{
			Status:  application.CloudContainerStatusRunning,
			Message: "container status",
			Data:    []byte(`{"foo":"bar"}`),
			Since:   ptr(now),
		}),
	}

	params := UpdateCAASUnitParams{
		ProviderID: ptr("provider-id"),
		Address:    ptr("10.6.6.6"),
		Ports:      ptr([]string{"666"}),
		AgentStatus: ptr(corestatus.StatusInfo{
			Status:  corestatus.Allocating,
			Message: "agent status",
			Data:    map[string]interface{}{"foo": "bar"},
			Since:   ptr(now),
		}),
		WorkloadStatus: ptr(corestatus.StatusInfo{
			Status:  corestatus.Waiting,
			Message: "workload status",
			Data:    map[string]interface{}{"foo": "bar"},
			Since:   ptr(now),
		}),
		CloudContainerStatus: ptr(corestatus.StatusInfo{
			Status:  corestatus.Running,
			Message: "container status",
			Data:    map[string]interface{}{"foo": "bar"},
			Since:   ptr(now),
		}),
	}

	s.state.EXPECT().GetApplicationLife(gomock.Any(), "foo").Return(appID, life.Alive, nil)

	var unitArgs application.UpdateCAASUnitParams
	s.state.EXPECT().UpdateCAASUnit(gomock.Any(), unitName, gomock.Any()).DoAndReturn(func(_ context.Context, _ coreunit.Name, args application.UpdateCAASUnitParams) error {
		unitArgs = args
		return nil
	})

	err := s.service.UpdateCAASUnit(context.Background(), unitName, params)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(unitArgs, jc.DeepEquals, expected)
}

func (s *unitServiceSuite) TestUpdateCAASUnitNotAlive(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := applicationtesting.GenApplicationUUID(c)
	s.state.EXPECT().GetApplicationLife(gomock.Any(), "foo").Return(id, life.Dying, nil)

	err := s.service.UpdateCAASUnit(context.Background(), coreunit.Name("foo/666"), UpdateCAASUnitParams{})
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationNotAlive)
}

func (s *unitServiceSuite) TestGetWorkloadUnitStatus(c *gc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now()

	unitUUID := unittesting.GenUnitUUID(c)
	s.state.EXPECT().GetUnitUUIDByName(gomock.Any(), coreunit.Name("foo/666")).Return(unitUUID, nil)
	s.state.EXPECT().GetUnitWorkloadStatus(gomock.Any(), unitUUID).Return(
		&application.StatusInfo[application.WorkloadStatusType]{
			Status:  application.WorkloadStatusActive,
			Message: "doink",
			Data:    []byte(`{"foo":"bar"}`),
			Since:   &now,
		}, nil)

	obtained, err := s.service.GetUnitWorkloadStatus(context.Background(), coreunit.Name("foo/666"))
	c.Assert(err, jc.ErrorIsNil)
	c.Check(obtained, jc.DeepEquals, &corestatus.StatusInfo{
		Status:  corestatus.Active,
		Message: "doink",
		Data:    map[string]interface{}{"foo": "bar"},
		Since:   &now,
	})
}
