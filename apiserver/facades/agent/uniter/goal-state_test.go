// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"context"
	"testing"
	"time"

	"github.com/juju/names/v6"
	"github.com/juju/tc"
	gomock "go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/common"
	applicationtesting "github.com/juju/juju/core/application/testing"
	"github.com/juju/juju/core/life"
	corerelation "github.com/juju/juju/core/relation"
	corestatus "github.com/juju/juju/core/status"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/relation"
	"github.com/juju/juju/internal/charm"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/rpc/params"
)

// uniterSuite implements common testing suite for all API
// versions. It's not intended to be used directly or registered as a
// suite, but embedded.
type uniterGoalStateSuite struct {
	testhelpers.IsolationSuite

	applicationService *MockApplicationService
	relationService    *MockRelationService
	statusService      *MockStatusService

	uniter *UniterAPI
}

func TestUniterGoalStateSuite(t *testing.T) {
	tc.Run(t, &uniterGoalStateSuite{})
}

func (s *uniterGoalStateSuite) TestStub(c *tc.C) {
	c.Skip(`
Given the initial state where:
- 3 machines exist
- A wordpress charm is deployed with a single unit to machine 0, with an unset status
- A mysql charm is deployed with a single unit to machine 1, with an unset status
- A logging charm is deployed with a single unit to machine 2, with an unset status
- An authoriser is congured to mock a logged in mysql unit

This suite is missing tests for the following scenarios:
- TestGoalStatesCrossModelRelation: when a relation is added between the mysql unit and a cross-model
  relation is established, but relations are included in the GoalStates result with the URL as the key
  for the cmr relation (where previously the application name was used)
`)
}

func (s *uniterGoalStateSuite) TestGoalStatesNoRelation(c *tc.C) {
	defer s.setupMocks(c).Finish()
	now := time.Now()

	// arrange: an applications with a principal units in 'waiting' workload status
	unitName := coreunit.Name("wordpress/0")
	appID := applicationtesting.GenApplicationUUID(c)

	s.applicationService.EXPECT().GetApplicationIDByUnitName(gomock.Any(), unitName).Return(appID, nil)
	s.statusService.EXPECT().GetUnitWorkloadStatusesForApplication(gomock.Any(), appID).
		Return(map[coreunit.Name]corestatus.StatusInfo{
			unitName: {
				Status: corestatus.Waiting,
				Since:  &now,
			},
		}, nil)

	s.applicationService.EXPECT().GetUnitNamesForApplication(gomock.Any(), "wordpress").
		Return([]coreunit.Name{unitName}, nil)
	s.applicationService.EXPECT().GetUnitPrincipal(gomock.Any(), unitName).Return("", false, nil)
	s.applicationService.EXPECT().GetUnitLife(gomock.Any(), unitName).Return(life.Alive, nil)

	s.relationService.EXPECT().GetGoalStateRelationDataForApplication(gomock.Any(), appID).
		Return([]relation.GoalStateRelationData{}, nil)

	// act:
	result, err := s.uniter.GoalStates(c.Context(), params.Entities{
		Entities: []params.Entity{{
			Tag: names.NewUnitTag(unitName.String()).String(),
		}},
	})

	// assert:
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.GoalStateResults{
		Results: []params.GoalStateResult{{
			Result: &params.GoalState{
				Units: params.UnitsGoalState{
					unitName.String(): params.GoalStateStatus{
						Status: corestatus.Waiting.String(),
						Since:  &now,
					},
				},
				Relations: map[string]params.UnitsGoalState{},
			},
		}},
	})
}

func (s *uniterGoalStateSuite) TestGoalStatesPeerUnitsNotRelation(c *tc.C) {
	defer s.setupMocks(c).Finish()
	now := time.Now()

	// arrange: an application with two principal units in 'waiting' workload status
	// with a peer relation between them
	unitName := coreunit.Name("wordpress/0")
	otherUnitName := coreunit.Name("wordpress/1")
	appID := applicationtesting.GenApplicationUUID(c)

	s.applicationService.EXPECT().GetApplicationIDByUnitName(gomock.Any(), unitName).Return(appID, nil)
	s.statusService.EXPECT().GetUnitWorkloadStatusesForApplication(gomock.Any(), appID).
		Return(map[coreunit.Name]corestatus.StatusInfo{
			unitName: {
				Status: corestatus.Waiting,
				Since:  &now,
			},
			otherUnitName: {
				Status: corestatus.Waiting,
				Since:  &now,
			},
		}, nil)

	s.applicationService.EXPECT().GetUnitNamesForApplication(gomock.Any(), "wordpress").
		Return([]coreunit.Name{unitName, otherUnitName}, nil)
	s.applicationService.EXPECT().GetUnitPrincipal(gomock.Any(), unitName).Return("", false, nil)
	s.applicationService.EXPECT().GetUnitPrincipal(gomock.Any(), otherUnitName).Return("", false, nil)
	s.applicationService.EXPECT().GetUnitLife(gomock.Any(), unitName).Return(life.Alive, nil)
	s.applicationService.EXPECT().GetUnitLife(gomock.Any(), otherUnitName).Return(life.Alive, nil)

	// arrange the peer relation
	s.relationService.EXPECT().GetGoalStateRelationDataForApplication(gomock.Any(), appID).
		Return([]relation.GoalStateRelationData{{
			EndpointIdentifiers: []corerelation.EndpointIdentifier{
				{
					ApplicationName: "wordpress",
					EndpointName:    "wordpress-peer",
					Role:            charm.RolePeer,
				},
			},
			Status: corestatus.Joining,
			Since:  &now,
		}}, nil)

	// act:
	result, err := s.uniter.GoalStates(c.Context(), params.Entities{
		Entities: []params.Entity{{
			Tag: names.NewUnitTag(unitName.String()).String(),
		}},
	})

	// assert:
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.GoalStateResults{
		Results: []params.GoalStateResult{{
			Result: &params.GoalState{
				Units: params.UnitsGoalState{
					unitName.String(): params.GoalStateStatus{
						Status: corestatus.Waiting.String(),
						Since:  &now,
					},
					otherUnitName.String(): params.GoalStateStatus{
						Status: corestatus.Waiting.String(),
						Since:  &now,
					},
				},
				Relations: map[string]params.UnitsGoalState{},
			},
		}},
	})
}

func (s *uniterGoalStateSuite) TestGoalStatesSingleRelation(c *tc.C) {
	defer s.setupMocks(c).Finish()
	now := time.Now()

	// arrange: an application with a single principal unit in 'waiting' workload status
	// with a 'joining' relation to another unit in 'waiting' workload status

	unitName := coreunit.Name("wordpress/0")
	otherUnitName := coreunit.Name("mysql/0")
	appID := applicationtesting.GenApplicationUUID(c)
	otherAppID := applicationtesting.GenApplicationUUID(c)

	s.applicationService.EXPECT().GetApplicationIDByUnitName(gomock.Any(), unitName).Return(appID, nil)
	s.applicationService.EXPECT().GetApplicationIDByName(gomock.Any(), "wordpress").Return(otherAppID, nil)
	s.applicationService.EXPECT().GetApplicationIDByName(gomock.Any(), "mysql").Return(otherAppID, nil)
	s.statusService.EXPECT().GetUnitWorkloadStatusesForApplication(gomock.Any(), appID).
		Return(map[coreunit.Name]corestatus.StatusInfo{
			unitName: {
				Status: corestatus.Waiting,
				Since:  &now,
			},
		}, nil)
	s.statusService.EXPECT().GetUnitWorkloadStatusesForApplication(gomock.Any(), otherAppID).
		Return(map[coreunit.Name]corestatus.StatusInfo{
			otherUnitName: {
				Status: corestatus.Waiting,
				Since:  &now,
			},
		}, nil)

	s.applicationService.EXPECT().GetUnitNamesForApplication(gomock.Any(), "wordpress").
		Return([]coreunit.Name{unitName}, nil)
	s.applicationService.EXPECT().GetUnitNamesForApplication(gomock.Any(), "mysql").
		Return([]coreunit.Name{otherUnitName}, nil)
	s.applicationService.EXPECT().GetUnitPrincipal(gomock.Any(), unitName).Return("", false, nil)
	s.applicationService.EXPECT().GetUnitPrincipal(gomock.Any(), otherUnitName).Return("", false, nil)
	s.applicationService.EXPECT().GetUnitLife(gomock.Any(), unitName).Return(life.Alive, nil)
	s.applicationService.EXPECT().GetUnitLife(gomock.Any(), otherUnitName).Return(life.Alive, nil)

	// arrange the relation
	s.relationService.EXPECT().GetGoalStateRelationDataForApplication(gomock.Any(), appID).
		Return([]relation.GoalStateRelationData{{
			EndpointIdentifiers: []corerelation.EndpointIdentifier{
				{
					ApplicationName: "wordpress",
					EndpointName:    "db",
					Role:            charm.RoleRequirer,
				},
				{
					ApplicationName: "mysql",
					EndpointName:    "db",
					Role:            charm.RoleProvider,
				},
			},
			Status: corestatus.Joining,
			Since:  &now,
		}}, nil)

	// act:
	result, err := s.uniter.GoalStates(c.Context(), params.Entities{
		Entities: []params.Entity{{
			Tag: names.NewUnitTag(unitName.String()).String(),
		}},
	})

	// assert:
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.GoalStateResults{
		Results: []params.GoalStateResult{{
			Result: &params.GoalState{
				Units: params.UnitsGoalState{
					unitName.String(): params.GoalStateStatus{
						Status: corestatus.Waiting.String(),
						Since:  &now,
					},
				},
				Relations: map[string]params.UnitsGoalState{
					"db": {
						"mysql": {
							Status: corestatus.Joining.String(),
							Since:  &now,
						},
						"mysql/0": {
							Status: corestatus.Waiting.String(),
							Since:  &now,
						},
					},
				},
			},
		}},
	})
}

func (s *uniterGoalStateSuite) TestGoalStatesDeadUnitsExcluded(c *tc.C) {
	defer s.setupMocks(c).Finish()
	now := time.Now()

	// arrange: an application with a principal unit in 'waiting' workload status
	// and a dead unit

	unitName := coreunit.Name("wordpress/0")
	deadUnitName := coreunit.Name("wordpress/1")
	appID := applicationtesting.GenApplicationUUID(c)

	s.applicationService.EXPECT().GetApplicationIDByUnitName(gomock.Any(), unitName).Return(appID, nil)
	s.statusService.EXPECT().GetUnitWorkloadStatusesForApplication(gomock.Any(), appID).
		Return(map[coreunit.Name]corestatus.StatusInfo{
			unitName: {
				Status: corestatus.Waiting,
				Since:  &now,
			},
			deadUnitName: {
				Status: corestatus.Unknown,
				Since:  &now,
			},
		}, nil)

	s.applicationService.EXPECT().GetUnitNamesForApplication(gomock.Any(), "wordpress").
		Return([]coreunit.Name{unitName, deadUnitName}, nil)
	s.applicationService.EXPECT().GetUnitPrincipal(gomock.Any(), unitName).Return("", false, nil)
	s.applicationService.EXPECT().GetUnitPrincipal(gomock.Any(), deadUnitName).Return("", false, nil)
	s.applicationService.EXPECT().GetUnitLife(gomock.Any(), unitName).Return(life.Alive, nil)
	s.applicationService.EXPECT().GetUnitLife(gomock.Any(), deadUnitName).Return(life.Dead, nil)

	// arrange the peer relation
	s.relationService.EXPECT().GetGoalStateRelationDataForApplication(gomock.Any(), appID).
		Return([]relation.GoalStateRelationData{{
			EndpointIdentifiers: []corerelation.EndpointIdentifier{
				{
					ApplicationName: "wordpress",
					EndpointName:    "wordpress-peer",
					Role:            charm.RolePeer,
				},
			},
			Status: corestatus.Waiting,
			Since:  &now,
		}}, nil)

	// act:
	result, err := s.uniter.GoalStates(c.Context(), params.Entities{
		Entities: []params.Entity{{
			Tag: names.NewUnitTag(unitName.String()).String(),
		}},
	})

	// assert:
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.GoalStateResults{
		Results: []params.GoalStateResult{{
			Result: &params.GoalState{
				Units: params.UnitsGoalState{
					unitName.String(): params.GoalStateStatus{
						Status: corestatus.Waiting.String(),
						Since:  &now,
					},
				},
				Relations: map[string]params.UnitsGoalState{},
			},
		}},
	})
}

func (s *uniterGoalStateSuite) TestGoalStatesSingleRelationDyingUnits(c *tc.C) {
	defer s.setupMocks(c).Finish()
	now := time.Now()

	// arrange: an application with a principal unit in 'waiting' workload status
	// and a dying unit

	unitName := coreunit.Name("wordpress/0")
	dyingUnitName := coreunit.Name("wordpress/1")
	appID := applicationtesting.GenApplicationUUID(c)

	s.applicationService.EXPECT().GetApplicationIDByUnitName(gomock.Any(), unitName).Return(appID, nil)
	s.statusService.EXPECT().GetUnitWorkloadStatusesForApplication(gomock.Any(), appID).
		Return(map[coreunit.Name]corestatus.StatusInfo{
			unitName: {
				Status: corestatus.Waiting,
				Since:  &now,
			},
			dyingUnitName: {
				Status: corestatus.Unknown,
				Since:  &now,
			},
		}, nil)

	s.applicationService.EXPECT().GetUnitNamesForApplication(gomock.Any(), "wordpress").
		Return([]coreunit.Name{unitName, dyingUnitName}, nil)
	s.applicationService.EXPECT().GetUnitPrincipal(gomock.Any(), unitName).Return("", false, nil)
	s.applicationService.EXPECT().GetUnitPrincipal(gomock.Any(), dyingUnitName).Return("", false, nil)
	s.applicationService.EXPECT().GetUnitLife(gomock.Any(), unitName).Return(life.Alive, nil)
	s.applicationService.EXPECT().GetUnitLife(gomock.Any(), dyingUnitName).Return(life.Dying, nil)

	// arrange the peer relation
	s.relationService.EXPECT().GetGoalStateRelationDataForApplication(gomock.Any(), appID).
		Return([]relation.GoalStateRelationData{{
			EndpointIdentifiers: []corerelation.EndpointIdentifier{
				{
					ApplicationName: "wordpress",
					EndpointName:    "wordpress-peer",
					Role:            charm.RolePeer,
				},
			},
			Status: corestatus.Waiting,
			Since:  &now,
		}}, nil)

	// act:
	result, err := s.uniter.GoalStates(c.Context(), params.Entities{
		Entities: []params.Entity{{
			Tag: names.NewUnitTag(unitName.String()).String(),
		}},
	})

	// assert:
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.GoalStateResults{
		Results: []params.GoalStateResult{{
			Result: &params.GoalState{
				Units: params.UnitsGoalState{
					unitName.String(): params.GoalStateStatus{
						Status: corestatus.Waiting.String(),
						Since:  &now,
					},
					dyingUnitName.String(): params.GoalStateStatus{
						Status: "dying",
						Since:  &now,
					},
				},
				Relations: map[string]params.UnitsGoalState{},
			},
		}},
	})
}

func (s *uniterGoalStateSuite) TestGoalStatesMultipleRelations(c *tc.C) {
	defer s.setupMocks(c).Finish()
	now := time.Now()

	// arrange:
	// - A 'wordpress' applications with two units
	//   - So the wordpress application has a peer relation
	// - Two 'mysql' applications, one with two units, and one with one unit
	// - A 'logging' application with a single unit
	// - Our wordpress application is related to both mysql applications
	// - Our wordpress application is related to the logging application

	wpUnit := coreunit.Name("wordpress/0")
	otherWpUnit := coreunit.Name("wordpress/1")
	wpAppID := applicationtesting.GenApplicationUUID(c)
	s.applicationService.EXPECT().GetApplicationIDByUnitName(gomock.Any(), wpUnit).Return(wpAppID, nil)
	s.applicationService.EXPECT().GetApplicationIDByName(gomock.Any(), "wordpress").Return(wpAppID, nil).MinTimes(1)
	s.applicationService.EXPECT().GetUnitNamesForApplication(gomock.Any(), "wordpress").
		Return([]coreunit.Name{wpUnit, otherWpUnit}, nil)
	s.applicationService.EXPECT().GetUnitPrincipal(gomock.Any(), wpUnit).Return("", false, nil)
	s.applicationService.EXPECT().GetUnitPrincipal(gomock.Any(), otherWpUnit).Return("", false, nil)
	s.applicationService.EXPECT().GetUnitLife(gomock.Any(), wpUnit).Return(life.Alive, nil)
	s.applicationService.EXPECT().GetUnitLife(gomock.Any(), otherWpUnit).Return(life.Alive, nil)

	mysqlUnit := coreunit.Name("mysql/0")
	otherMysqlUnit := coreunit.Name("mysql/1")
	mysqlAppID := applicationtesting.GenApplicationUUID(c)
	otherAppMysqlUnit := coreunit.Name("other-mysql/0")
	otherMysqlAppID := applicationtesting.GenApplicationUUID(c)
	s.applicationService.EXPECT().GetApplicationIDByName(gomock.Any(), "mysql").Return(mysqlAppID, nil)
	s.applicationService.EXPECT().GetApplicationIDByName(gomock.Any(), "other-mysql").Return(otherMysqlAppID, nil)
	s.applicationService.EXPECT().GetUnitNamesForApplication(gomock.Any(), "mysql").
		Return([]coreunit.Name{mysqlUnit, otherMysqlUnit}, nil)
	s.applicationService.EXPECT().GetUnitNamesForApplication(gomock.Any(), "other-mysql").
		Return([]coreunit.Name{otherAppMysqlUnit}, nil)
	s.applicationService.EXPECT().GetUnitPrincipal(gomock.Any(), mysqlUnit).Return("", false, nil)
	s.applicationService.EXPECT().GetUnitPrincipal(gomock.Any(), otherMysqlUnit).Return("", false, nil)
	s.applicationService.EXPECT().GetUnitPrincipal(gomock.Any(), otherAppMysqlUnit).Return("", false, nil)
	s.applicationService.EXPECT().GetUnitLife(gomock.Any(), mysqlUnit).Return(life.Alive, nil)
	s.applicationService.EXPECT().GetUnitLife(gomock.Any(), otherMysqlUnit).Return(life.Alive, nil)
	s.applicationService.EXPECT().GetUnitLife(gomock.Any(), otherAppMysqlUnit).Return(life.Alive, nil)

	loggingUnit := coreunit.Name("logging/0")
	loggingAppID := applicationtesting.GenApplicationUUID(c)
	s.applicationService.EXPECT().GetApplicationIDByName(gomock.Any(), "logging").Return(loggingAppID, nil)
	s.applicationService.EXPECT().GetUnitNamesForApplication(gomock.Any(), "logging").
		Return([]coreunit.Name{loggingUnit}, nil)
	s.applicationService.EXPECT().GetUnitPrincipal(gomock.Any(), loggingUnit).Return("", false, nil)
	s.applicationService.EXPECT().GetUnitLife(gomock.Any(), loggingUnit).Return(life.Alive, nil)

	s.statusService.EXPECT().GetUnitWorkloadStatusesForApplication(gomock.Any(), wpAppID).
		Return(map[coreunit.Name]corestatus.StatusInfo{
			wpUnit: {
				Status: corestatus.Waiting,
				Since:  &now,
			},
			otherWpUnit: {
				Status: corestatus.Waiting,
				Since:  &now,
			},
		}, nil)
	s.statusService.EXPECT().GetUnitWorkloadStatusesForApplication(gomock.Any(), mysqlAppID).
		Return(map[coreunit.Name]corestatus.StatusInfo{
			mysqlUnit: {
				Status: corestatus.Waiting,
				Since:  &now,
			},
			otherMysqlUnit: {
				Status: corestatus.Waiting,
				Since:  &now,
			},
		}, nil)
	s.statusService.EXPECT().GetUnitWorkloadStatusesForApplication(gomock.Any(), otherMysqlAppID).
		Return(map[coreunit.Name]corestatus.StatusInfo{
			otherAppMysqlUnit: {
				Status: corestatus.Waiting,
				Since:  &now,
			},
		}, nil)
	s.statusService.EXPECT().GetUnitWorkloadStatusesForApplication(gomock.Any(), loggingAppID).
		Return(map[coreunit.Name]corestatus.StatusInfo{
			loggingUnit: {
				Status: corestatus.Waiting,
				Since:  &now,
			},
		}, nil)

	// Arrange the relations
	s.relationService.EXPECT().GetGoalStateRelationDataForApplication(gomock.Any(), wpAppID).
		Return([]relation.GoalStateRelationData{
			{
				EndpointIdentifiers: []corerelation.EndpointIdentifier{
					{
						ApplicationName: "wordpress",
						EndpointName:    "wordpress-peer",
						Role:            charm.RolePeer,
					},
				},
				Status: corestatus.Joining,
				Since:  &now,
			},
			{
				EndpointIdentifiers: []corerelation.EndpointIdentifier{
					{
						ApplicationName: "wordpress",
						EndpointName:    "db",
						Role:            charm.RoleRequirer,
					},
					{
						ApplicationName: "mysql",
						EndpointName:    "db",
						Role:            charm.RoleProvider,
					},
				},
				Status: corestatus.Joining,
				Since:  &now,
			}, {
				EndpointIdentifiers: []corerelation.EndpointIdentifier{
					{
						ApplicationName: "wordpress",
						EndpointName:    "db",
						Role:            charm.RoleRequirer,
					}, {
						ApplicationName: "other-mysql",
						EndpointName:    "db",
						Role:            charm.RoleProvider,
					},
				},
				Status: corestatus.Joining,
				Since:  &now,
			}, {
				EndpointIdentifiers: []corerelation.EndpointIdentifier{
					{
						ApplicationName: "wordpress",
						EndpointName:    "logging",
						Role:            charm.RoleRequirer,
					}, {
						ApplicationName: "logging",
						EndpointName:    "logging",
						Role:            charm.RoleProvider,
					},
				},
				Status: corestatus.Joining,
				Since:  &now,
			},
		}, nil)

	// Act:
	result, err := s.uniter.GoalStates(c.Context(), params.Entities{
		Entities: []params.Entity{{
			Tag: names.NewUnitTag(wpUnit.String()).String(),
		}},
	})

	// Assert:
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.GoalStateResults{
		Results: []params.GoalStateResult{{
			Result: &params.GoalState{
				Units: params.UnitsGoalState{
					wpUnit.String(): params.GoalStateStatus{
						Status: corestatus.Waiting.String(),
						Since:  &now,
					},
					otherWpUnit.String(): params.GoalStateStatus{
						Status: corestatus.Waiting.String(),
						Since:  &now,
					},
				},
				Relations: map[string]params.UnitsGoalState{
					"db": {
						"mysql": params.GoalStateStatus{
							Status: corestatus.Joining.String(),
							Since:  &now,
						},
						"mysql/0": params.GoalStateStatus{
							Status: corestatus.Waiting.String(),
							Since:  &now,
						},
						"mysql/1": params.GoalStateStatus{
							Status: corestatus.Waiting.String(),
							Since:  &now,
						},
						"other-mysql": params.GoalStateStatus{
							Status: corestatus.Joining.String(),
							Since:  &now,
						},
						"other-mysql/0": params.GoalStateStatus{
							Status: corestatus.Waiting.String(),
							Since:  &now,
						},
					},
					"logging": {
						"logging": params.GoalStateStatus{
							Status: corestatus.Joining.String(),
							Since:  &now,
						},
						"logging/0": params.GoalStateStatus{
							Status: corestatus.Waiting.String(),
							Since:  &now,
						},
					},
				},
			},
		}},
	})
}

func (s *uniterGoalStateSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.applicationService = NewMockApplicationService(ctrl)
	s.relationService = NewMockRelationService(ctrl)
	s.statusService = NewMockStatusService(ctrl)

	authFunc := func(ctx context.Context) (common.AuthFunc, error) {
		return func(tag names.Tag) bool {
			return true
		}, nil
	}

	s.uniter = &UniterAPI{
		applicationService: s.applicationService,
		relationService:    s.relationService,
		statusService:      s.statusService,
		accessApplication:  authFunc,
		accessUnit:         authFunc,
		logger:             loggertesting.WrapCheckLog(c),
	}

	c.Cleanup(func() {
		s.applicationService = nil
		s.relationService = nil
		s.statusService = nil
		s.uniter = nil
	})

	return ctrl
}
