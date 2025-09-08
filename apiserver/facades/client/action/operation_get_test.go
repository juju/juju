// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/juju/collections/transform"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	gomock "go.uber.org/mock/gomock"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/machine"
	modeltesting "github.com/juju/juju/core/model/testing"
	corestatus "github.com/juju/juju/core/status"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/operation"
	"github.com/juju/juju/rpc/params"
)

type getOperationSuite struct {
	MockBaseSuite
}

func TestGetOperationSuite(t *testing.T) {
	// Keep legacy runner but now we populate with real tests
	tc.Run(t, &getOperationSuite{})
}

// TestListOperations_PermissionDenied verifies ListOperations returns ErrPerm
// and does not call service when read permission is denied.
func (s *getOperationSuite) TestListOperations_PermissionDenied(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	// Authorizer without read permission
	auth := apiservertesting.FakeAuthorizer{Tag: names.NewUserTag("readonly")}
	api, err := NewActionAPI(auth, s.Leadership, s.ApplicationService, s.BlockCommandService, s.ModelInfoService, s.OperationService, modeltesting.GenModelUUID(c))
	c.Assert(err, tc.ErrorIsNil)
	// Ensure List is not called
	s.OperationService.EXPECT().GetOperations(gomock.Any(), gomock.Any()).Times(0)

	// Act
	_, err = api.ListOperations(c.Context(), params.OperationQueryArgs{Applications: []string{"app"}})

	// Assert
	c.Assert(err, tc.ErrorIs, apiservererrors.ErrPerm)
}

// TestListOperations_NoFilters verifies that no filters pass an empty target
// and no other filters, and empty result is returned.
func (s *getOperationSuite) TestListOperations_NoFilters(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	api := s.NewActionAPI(c)
	s.OperationService.EXPECT().GetOperations(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, qp operation.QueryArgs) (operation.QueryResult, error) {
			c.Check(qp.Receivers.Applications, tc.HasLen, 0)
			c.Check(qp.Receivers.Machines, tc.HasLen, 0)
			c.Check(qp.Receivers.Units, tc.HasLen, 0)
			c.Check(qp.ActionNames, tc.IsNil)
			c.Check(qp.Status, tc.IsNil)
			c.Check(qp.Limit, tc.IsNil)
			c.Check(qp.Offset, tc.IsNil)
			return operation.QueryResult{}, nil
		})

	// Act
	_, err := api.ListOperations(c.Context(), params.OperationQueryArgs{})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

// TestListOperations_ApplicationsFilter ensures application names flow into
// Receivers.Applications.
func (s *getOperationSuite) TestListOperations_ApplicationsFilter(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	api := s.NewActionAPI(c)
	apps := []string{"app-a", "app-b"}
	s.OperationService.EXPECT().GetOperations(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, qp operation.QueryArgs) (operation.QueryResult, error) {
			c.Check(qp.Receivers.Applications, tc.DeepEquals, apps)
			return operation.QueryResult{}, nil
		})

	// Act
	_, err := api.ListOperations(c.Context(), params.OperationQueryArgs{Applications: apps})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

// TestListOperations_UnitsFilter verifies string unit names convert to
// []unit.Name in Receivers.Units.
func (s *getOperationSuite) TestListOperations_UnitsFilter(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	api := s.NewActionAPI(c)
	units := []string{"app-a/0", "app-b/3"}
	s.OperationService.EXPECT().GetOperations(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, qp operation.QueryArgs) (operation.QueryResult, error) {
			c.Check(qp.Receivers.Units, tc.DeepEquals, []unit.Name{"app-a/0", "app-b/3"})
			return operation.QueryResult{}, nil
		})
	// Act
	_, err := api.ListOperations(c.Context(), params.OperationQueryArgs{Units: units})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

// TestListOperations_MachinesFilter verifies string machine names convert
// to []machine.Name in Target.Machines.
func (s *getOperationSuite) TestListOperations_MachinesFilter(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	api := s.NewActionAPI(c)
	machines := []string{"0", "42"}
	s.OperationService.EXPECT().GetOperations(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, qp operation.QueryArgs) (operation.QueryResult, error) {
			c.Check(qp.Receivers.Machines, tc.DeepEquals, []machine.Name{"0", "42"})
			return operation.QueryResult{}, nil
		})
	// Act
	_, err := api.ListOperations(c.Context(), params.OperationQueryArgs{Machines: machines})
	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

// TestListOperations_ActionNamesFilter confirms actions filter passes through.
func (s *getOperationSuite) TestListOperations_ActionNamesFilter(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	api := s.NewActionAPI(c)
	names := []string{"backup", "reindex"}
	s.OperationService.EXPECT().GetOperations(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, qp operation.QueryArgs) (operation.QueryResult, error) {
			c.Check(qp.ActionNames, tc.DeepEquals, names)
			return operation.QueryResult{}, nil
		})
	// Act
	_, err := api.ListOperations(c.Context(), params.OperationQueryArgs{ActionNames: names})
	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

// TestListOperations_StatusFilter confirms status filter passes through.
func (s *getOperationSuite) TestListOperations_StatusFilter(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	api := s.NewActionAPI(c)
	status := []corestatus.Status{"running", "completed"}
	s.OperationService.EXPECT().GetOperations(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, qp operation.QueryArgs) (operation.QueryResult, error) {
			c.Check(qp.Status, tc.DeepEquals, status)
			return operation.QueryResult{}, nil
		})
	// Act
	_, err := api.ListOperations(c.Context(), params.OperationQueryArgs{
		Status: transform.Slice(status, corestatus.Status.String)})
	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

// TestListOperations_LimitOffset verifies Limit and Offset pointers pass unchanged.
func (s *getOperationSuite) TestListOperations_LimitOffset(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	api := s.NewActionAPI(c)
	limit := 10
	offset := 20
	s.OperationService.EXPECT().GetOperations(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, qp operation.QueryArgs) (operation.QueryResult, error) {
			c.Check(qp.Limit, tc.DeepEquals, ptr(10))
			c.Check(qp.Offset, tc.DeepEquals, ptr(20))
			return operation.QueryResult{}, nil
		})
	// Act
	_, err := api.ListOperations(c.Context(), params.OperationQueryArgs{Limit: &limit, Offset: &offset})
	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

// TestListOperations_CombinedFilters ensures multiple filters are passed
// together without modification.
func (s *getOperationSuite) TestListOperations_CombinedFilters(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	api := s.NewActionAPI(c)
	apps := []string{"a"}
	units := []string{"a/0"}
	machines := []string{"1"}
	actionNames := []string{"do"}
	status := []corestatus.Status{"running"}
	s.OperationService.EXPECT().GetOperations(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, qp operation.QueryArgs) (operation.QueryResult, error) {
			c.Check(qp.Receivers.Applications, tc.DeepEquals, apps)
			c.Check(qp.ActionNames, tc.DeepEquals, actionNames)
			c.Check(qp.Status, tc.DeepEquals, status)
			c.Check(qp.Receivers.Units, tc.DeepEquals, []unit.Name{"a/0"})
			c.Check(qp.Receivers.Machines, tc.DeepEquals, []machine.Name{"1"})
			return operation.QueryResult{}, nil
		})
	// Act
	_, err := api.ListOperations(c.Context(), params.OperationQueryArgs{
		Applications: apps,
		Units:        units,
		Machines:     machines,
		ActionNames:  actionNames,
		Status:       transform.Slice(status, corestatus.Status.String)})
	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

// TestListOperations_ServiceError ensures service error is propagated.
func (s *getOperationSuite) TestListOperations_ServiceError(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	api := s.NewActionAPI(c)
	s.OperationService.EXPECT().GetOperations(gomock.Any(), gomock.Any()).Return(operation.QueryResult{}, fmt.Errorf("boom"))
	// Act
	_, err := api.ListOperations(c.Context(), params.OperationQueryArgs{})
	// Assert
	c.Assert(err, tc.ErrorMatches, "boom")
}

// TestListOperations_MappingSingleOperation validates mapping of
// OperationInfo with unit and machine actions into params.
func (s *getOperationSuite) TestListOperations_MappingSingleOperation(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	api := s.NewActionAPI(c)
	tiM := operation.TaskInfo{ID: "1", TaskArgs: operation.TaskArgs{ActionName: "m-act"}}
	tiU := operation.TaskInfo{ID: "2", TaskArgs: operation.TaskArgs{ActionName: "u-act"}}
	qr := operation.QueryResult{Operations: []operation.OperationInfo{{
		OperationID: "123",
		Summary:     "s",
		Status:      "completed",
		Machines:    []operation.MachineTaskResult{{ReceiverName: "2", TaskInfo: tiM}},
		Units:       []operation.UnitTaskResult{{ReceiverName: "app/0", TaskInfo: tiU}},
	}}}
	s.OperationService.EXPECT().GetOperations(gomock.Any(), gomock.Any()).Return(qr, nil)
	// Act
	res, err := api.ListOperations(c.Context(), params.OperationQueryArgs{})
	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(res.Results), tc.Equals, 1)
	c.Check(res.Results[0].OperationTag, tc.Equals, "operation-123")
	c.Check(res.Results[0].Summary, tc.Equals, "s")
	c.Check(res.Results[0].Status, tc.Equals, "completed")
	c.Check(len(res.Results[0].Actions), tc.Equals, 2)
	// machine action
	c.Check(res.Results[0].Actions[0].Action.Receiver, tc.Equals, "machine-2")
	c.Check(res.Results[0].Actions[0].Action.Name, tc.Equals, "m-act")
	c.Check(res.Results[0].Actions[0].Action.Tag, tc.Equals, names.NewActionTag("1").String())
	// unit action
	c.Check(res.Results[0].Actions[1].Action.Receiver, tc.Equals, "unit-app-0")
	c.Check(res.Results[0].Actions[1].Action.Name, tc.Equals, "u-act")
	c.Check(res.Results[0].Actions[1].Action.Tag, tc.Equals, names.NewActionTag("2").String())
}

// TestListOperations_TruncatedPassThrough ensures Truncated flag propagates.
func (s *getOperationSuite) TestListOperations_TruncatedPassThrough(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	api := s.NewActionAPI(c)
	qr := operation.QueryResult{Truncated: true}
	s.OperationService.EXPECT().GetOperations(gomock.Any(), gomock.Any()).Return(qr, nil)
	// Act
	res, err := api.ListOperations(c.Context(), params.OperationQueryArgs{})
	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Truncated, tc.Equals, true)
}

// TestListOperations_OperationErrorMapping validates mapping of operation-level error to params.Error.
func (s *getOperationSuite) TestListOperations_OperationErrorMapping(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	api := s.NewActionAPI(c)
	qr := operation.QueryResult{Operations: []operation.OperationInfo{{OperationID: "1", Error: fmt.Errorf("op-fail")}}}
	s.OperationService.EXPECT().GetOperations(gomock.Any(), gomock.Any()).Return(qr, nil)
	// Act
	res, err := api.ListOperations(c.Context(), params.OperationQueryArgs{})
	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Results, tc.HasLen, 1)
	c.Assert(res.Results[0].Error, tc.NotNil)
	c.Assert(res.Results[0].Error.Message, tc.Matches, ".*op-fail.*")
}

// TestListOperations_ActionFieldMapping ensures TaskInfo fields map into
// ActionResult fields.
func (s *getOperationSuite) TestListOperations_ActionFieldMapping(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	api := s.NewActionAPI(c)
	when := time.Date(2025, time.January, 2, 3, 4, 5, 0, time.UTC)
	log := []operation.TaskLog{{Timestamp: when, Message: "log1"}}
	ti := operation.TaskInfo{
		ID:       "1",
		TaskArgs: operation.TaskArgs{ActionName: "run"},
		Status:   "running",
		Message:  "in progress",
		Log:      log,
		Output:   map[string]interface{}{"k": "v"},
		Error:    fmt.Errorf("task-fail")}
	qr := operation.QueryResult{
		Operations: []operation.OperationInfo{{
			OperationID: "1",
			Units: []operation.UnitTaskResult{{
				ReceiverName: "app/1",
				TaskInfo:     ti,
			}}}}}
	s.OperationService.EXPECT().GetOperations(gomock.Any(), gomock.Any()).Return(qr, nil)

	// Act
	res, err := api.ListOperations(c.Context(), params.OperationQueryArgs{})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Results, tc.HasLen, 1)
	acts := res.Results[0].Actions
	c.Assert(acts, tc.HasLen, 1)
	ar := acts[0]
	c.Check(ar.Status, tc.Equals, "running")
	c.Check(ar.Message, tc.Equals, "in progress")
	c.Check(ar.Log, tc.HasLen, 1)
	c.Check(ar.Log[0].Timestamp.Equal(when), tc.Equals, true)
	c.Check(ar.Log[0].Message, tc.Equals, "log1")
	c.Check(ar.Output["k"], tc.Equals, "v")
	c.Assert(ar.Error, tc.NotNil)
	c.Check(ar.Error.Message, tc.Matches, ".*task-fail.*")
}

// TestListOperations_EmptyOperations verifies that an empty operations slice results in empty results.
func (s *getOperationSuite) TestListOperations_EmptyOperations(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	api := s.NewActionAPI(c)
	qr := operation.QueryResult{Operations: []operation.OperationInfo{}, Truncated: false}
	s.OperationService.EXPECT().GetOperations(gomock.Any(), gomock.Any()).Return(qr, nil)
	// Act
	res, err := api.ListOperations(c.Context(), params.OperationQueryArgs{})
	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(res.Results), tc.Equals, 0)
	c.Assert(res.Truncated, tc.Equals, false)
}

// toEntities converts tags to params.Entities for Operations tests.
func toEntities(tags ...string) params.Entities {
	ents := make([]params.Entity, len(tags))
	for i, t := range tags {
		ents[i] = params.Entity{Tag: t}
	}
	return params.Entities{Entities: ents}
}

// TestOperations_PermissionDenied verifies read permission is enforced
// and that the service is not called when denied.
func (s *getOperationSuite) TestOperations_PermissionDenied(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	auth := apiservertesting.FakeAuthorizer{Tag: names.NewUserTag("readonly")}
	api, err := NewActionAPI(auth, s.Leadership, s.ApplicationService,
		s.BlockCommandService, s.ModelInfoService, s.OperationService,
		modeltesting.GenModelUUID(c))
	c.Assert(err, tc.ErrorIsNil)
	// Ensure no call
	s.OperationService.EXPECT().GetOperationByID(gomock.Any(), gomock.Any()).Times(0)

	// Act
	_, err = api.Operations(c.Context(), toEntities("operation-1"))

	// Assert
	c.Assert(err, tc.ErrorIs, apiservererrors.ErrPerm)
}

// TestOperations_AllTagsInvalid returns per-entity parse errors and
// does not call the service.
func (s *getOperationSuite) TestOperations_AllTagsInvalid(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	api := s.NewActionAPI(c)
	// No service call expected
	s.OperationService.EXPECT().GetOperationByID(gomock.Any(), gomock.Any()).Times(0)
	arg := toEntities("not-a-tag", "application-foo", "unit-app-0")

	// Act
	res, err := api.Operations(c.Context(), arg)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Results, tc.HasLen, 3)
	for i := range res.Results {
		c.Check(res.Results[i].Error, tc.NotNil)
	}
}

// TestOperations_MixedValidInvalid calls service with only valid IDs and
// aligns results in input order with parse errors preserved.
func (s *getOperationSuite) TestOperations_MixedValidInvalid(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	api := s.NewActionAPI(c)
	s.OperationService.EXPECT().GetOperationByID(gomock.Any(), "1").Return(operation.OperationInfo{
		OperationID: "1",
		Summary:     "a",
	}, nil)
	s.OperationService.EXPECT().GetOperationByID(gomock.Any(), "2").Return(operation.OperationInfo{
		OperationID: "2",
		Summary:     "b",
	}, nil)
	arg := toEntities("operation-1", "bad-tag", "operation-2")

	// Act
	res, err := api.Operations(c.Context(), arg)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Results, tc.HasLen, 3)
	c.Check(res.Results[0].OperationTag, tc.Equals, "operation-1")
	c.Check(res.Results[0].Error, tc.IsNil)
	c.Check(res.Results[0].Summary, tc.Equals, "a")
	c.Check(res.Results[1].Error, tc.NotNil)
	c.Check(res.Results[2].OperationTag, tc.Equals, "operation-2")
	c.Check(res.Results[2].Error, tc.IsNil)
	c.Check(res.Results[2].Summary, tc.Equals, "b")
}

// TestOperations_EmptyInput returns empty results and does not call service.
func (s *getOperationSuite) TestOperations_EmptyInput(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	api := s.NewActionAPI(c)
	s.OperationService.EXPECT().GetOperationByID(gomock.Any(), gomock.Any()).Times(0)

	// Act
	res, err := api.Operations(c.Context(), params.Entities{})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Results, tc.HasLen, 0)
}

// TestOperations_ServiceError ensures service errors are propagated.
func (s *getOperationSuite) TestOperations_ServiceError(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	api := s.NewActionAPI(c)
	s.OperationService.EXPECT().GetOperationByID(gomock.Any(), "1").Return(
		operation.OperationInfo{OperationID: "1"}, nil)
	s.OperationService.EXPECT().GetOperationByID(gomock.Any(), "2").Return(
		operation.OperationInfo{OperationID: "2"}, fmt.Errorf("boom"))
	arg := toEntities("operation-1", "operation-2")

	// Act
	res, err := api.Operations(c.Context(), arg)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Results, tc.HasLen, 2)
	c.Check(res.Results[0].Error, tc.IsNil)
	c.Check(res.Results[1].Error, tc.ErrorMatches, ".*boom.*")
}

// TestOperations_LargeBatch ensures stable mapping for many entries.
func (s *getOperationSuite) TestOperations_LargeBatch(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	api := s.NewActionAPI(c)
	arg := toEntities(
		"operation-1", "operation-2", "operation-3", "operation-4",
		"operation-5", "operation-6", "operation-7", "operation-8",
	)
	s.OperationService.EXPECT().GetOperationByID(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, s string) (operation.OperationInfo, error) {
			return operation.OperationInfo{OperationID: s}, nil
		}).Times(len(arg.Entities))

	// Act
	res, err := api.Operations(c.Context(), arg)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	for i := 1; i < len(arg.Entities); i++ {
		c.Check(res.Results[i-1].OperationTag, tc.Equals, fmt.Sprintf("operation-%d", i))
	}
}

func ptr[T any](v T) *T { return &v }
