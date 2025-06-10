// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/canonical/sqlair"
	"github.com/juju/clock"
	"github.com/juju/tc"

	coreapplication "github.com/juju/juju/core/application"
	corelife "github.com/juju/juju/core/life"
	"github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	corerelation "github.com/juju/juju/core/relation"
	corerelationtesting "github.com/juju/juju/core/relation/testing"
	corestatus "github.com/juju/juju/core/status"
	coreunit "github.com/juju/juju/core/unit"
	unittesting "github.com/juju/juju/core/unit/testing"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/architecture"
	"github.com/juju/juju/domain/application/charm"
	applicationstate "github.com/juju/juju/domain/application/state"
	"github.com/juju/juju/domain/deployment"
	"github.com/juju/juju/domain/life"
	modelerrors "github.com/juju/juju/domain/model/errors"
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/domain/status"
	statuserrors "github.com/juju/juju/domain/status/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
)

type stateSuite struct {
	schematesting.ModelSuite

	state *State
}

func TestStateSuite(t *testing.T) {
	tc.Run(t, &stateSuite{})
}

func (s *stateSuite) SetUpTest(c *tc.C) {
	s.ModelSuite.SetUpTest(c)

	s.state = NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))
}

func (s *stateSuite) TestGetModelStatusInfo(c *tc.C) {
	modelUUID := modeltesting.GenModelUUID(c)
	controllerUUID, err := uuid.NewUUID()
	c.Check(err, tc.ErrorIsNil)

	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO model (uuid, controller_uuid, name, type, cloud, cloud_type, credential_owner)
			VALUES (?, ?, "test", "iaas", "test-model", "ec2", "owner")
		`, modelUUID.String(), controllerUUID.String())
		return err
	})
	c.Check(err, tc.ErrorIsNil)

	modelInfo, err := s.state.GetModelStatusInfo(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Check(modelInfo.Type, tc.Equals, model.IAAS)
}

func (s *stateSuite) TestGetModelStatusInfoNotFound(c *tc.C) {
	state := NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))

	_, err := state.GetModelStatusInfo(c.Context())
	c.Assert(err, tc.ErrorIs, modelerrors.NotFound)
}

func (s *stateSuite) TestGetAllRelationStatuses(c *tc.C) {
	// Arrange: add two relation, one with a status, but not the second one.
	now := time.Now().Truncate(time.Minute).UTC()

	relationID := 7
	relationUUID := s.addRelationWithLifeAndID(c, corelife.Alive, relationID)
	_ = s.addRelationWithLifeAndID(c, corelife.Alive, 8)

	s.addRelationStatusWithMessage(c, relationUUID, corestatus.Suspended, "this is a test", now)

	// Act
	result, err := s.state.GetAllRelationStatuses(c.Context())

	// Assert:
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, []status.RelationStatusInfo{{
		RelationUUID: relationUUID,
		RelationID:   relationID,
		StatusInfo: status.StatusInfo[status.RelationStatusType]{
			Status:  status.RelationStatusTypeSuspended,
			Message: "this is a test",
			Since:   &now,
		},
	}})
}

func (s *stateSuite) TestGetAllRelationStatusesNone(c *tc.C) {
	// Act
	result, err := s.state.GetAllRelationStatuses(c.Context())

	// Assert:
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.HasLen, 0)
}

func (s *stateSuite) TestGetApplicationIDByName(c *tc.C) {
	id, _ := s.createApplication(c, "foo", life.Alive, false, s.appStatus(time.Now()))

	gotID, err := s.state.GetApplicationIDByName(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(gotID, tc.Equals, id)
}

func (s *stateSuite) TestGetApplicationIDByNameNotFound(c *tc.C) {
	_, err := s.state.GetApplicationIDByName(c.Context(), "foo")
	c.Assert(err, tc.ErrorIs, statuserrors.ApplicationNotFound)
}

func (s *stateSuite) TestGetApplicationIDAndNameByUnitName(c *tc.C) {
	u1 := application.AddUnitArg{}
	expectedAppUUID, _ := s.createApplication(c, "foo", life.Alive, false, s.appStatus(time.Now()), u1)

	appUUID, appName, err := s.state.GetApplicationIDAndNameByUnitName(c.Context(), coreunit.Name("foo/0"))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(appUUID, tc.Equals, expectedAppUUID)
	c.Check(appName, tc.Equals, "foo")
}

func (s *stateSuite) TestGetApplicationIDAndNameByUnitNameNotFound(c *tc.C) {
	_, _, err := s.state.GetApplicationIDAndNameByUnitName(c.Context(), "failme")
	c.Assert(err, tc.ErrorIs, statuserrors.UnitNotFound)
}

func (s *stateSuite) TestSetApplicationStatus(c *tc.C) {
	id, _ := s.createApplication(c, "foo", life.Alive, false, s.appStatus(time.Now()))

	now := time.Now().UTC()
	expected := status.StatusInfo[status.WorkloadStatusType]{
		Status:  status.WorkloadStatusActive,
		Message: "message",
		Data:    []byte("data"),
		Since:   ptr(now),
	}

	err := s.state.SetApplicationStatus(c.Context(), id, expected)
	c.Assert(err, tc.ErrorIsNil)

	status, err := s.state.GetApplicationStatus(c.Context(), id)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(status, tc.DeepEquals, expected)
}

func (s *stateSuite) TestSetApplicationStatusMultipleTimes(c *tc.C) {
	id, _ := s.createApplication(c, "foo", life.Alive, false, s.appStatus(time.Now()))

	err := s.state.SetApplicationStatus(c.Context(), id, status.StatusInfo[status.WorkloadStatusType]{
		Status:  status.WorkloadStatusBlocked,
		Message: "blocked",
		Since:   ptr(time.Now().UTC()),
	})
	c.Assert(err, tc.ErrorIsNil)

	now := time.Now().UTC()
	expected := status.StatusInfo[status.WorkloadStatusType]{
		Status:  status.WorkloadStatusActive,
		Message: "message",
		Data:    []byte("data"),
		Since:   ptr(now),
	}

	err = s.state.SetApplicationStatus(c.Context(), id, expected)
	c.Assert(err, tc.ErrorIsNil)

	status, err := s.state.GetApplicationStatus(c.Context(), id)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(status, tc.DeepEquals, expected)
}

func (s *stateSuite) TestSetApplicationStatusWithNoData(c *tc.C) {
	id, _ := s.createApplication(c, "foo", life.Alive, false, s.appStatus(time.Now()))

	now := time.Now().UTC()
	expected := status.StatusInfo[status.WorkloadStatusType]{
		Status:  status.WorkloadStatusActive,
		Message: "message",
		Since:   ptr(now),
	}

	err := s.state.SetApplicationStatus(c.Context(), id, expected)
	c.Assert(err, tc.ErrorIsNil)

	status, err := s.state.GetApplicationStatus(c.Context(), id)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(status, tc.DeepEquals, expected)
}

func (s *stateSuite) TestSetApplicationStatusApplicationNotFound(c *tc.C) {
	now := time.Now().UTC()
	expected := status.StatusInfo[status.WorkloadStatusType]{
		Status:  status.WorkloadStatusActive,
		Message: "message",
		Data:    []byte("data"),
		Since:   ptr(now),
	}

	err := s.state.SetApplicationStatus(c.Context(), "foo", expected)
	c.Assert(err, tc.ErrorIs, statuserrors.ApplicationNotFound)
}

func (s *stateSuite) TestSetApplicationStatusInvalidStatus(c *tc.C) {
	id, _ := s.createApplication(c, "foo", life.Alive, false, s.appStatus(time.Now()))

	expected := status.StatusInfo[status.WorkloadStatusType]{
		Status: status.WorkloadStatusType(99),
	}

	err := s.state.SetApplicationStatus(c.Context(), id, expected)
	c.Assert(err, tc.ErrorMatches, `unknown status.*`)
}

func (s *stateSuite) TestGetApplicationStatusApplicationNotFound(c *tc.C) {
	_, err := s.state.GetApplicationStatus(c.Context(), "foo")
	c.Assert(err, tc.ErrorIs, statuserrors.ApplicationNotFound)
}

func (s *stateSuite) TestGetApplicationStatusNotSet(c *tc.C) {
	id, _ := s.createApplication(c, "foo", life.Alive, false, nil)

	sts, err := s.state.GetApplicationStatus(c.Context(), id)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(sts, tc.DeepEquals, status.StatusInfo[status.WorkloadStatusType]{
		Status: status.WorkloadStatusUnset,
	})
}

func (s *stateSuite) TestSetRelationStatus(c *tc.C) {
	// Arrange: Create relation and statuses.
	relationUUID := s.addRelationWithLifeAndID(c, corelife.Alive, 7)
	now := time.Now().UTC()
	s.addRelationStatusWithMessage(c, relationUUID, corestatus.Joining, "", now)

	sts := status.StatusInfo[status.RelationStatusType]{
		Status:  status.RelationStatusTypeSuspended,
		Message: "message",
		Since:   ptr(now),
	}

	// Act:
	err := s.state.SetRelationStatus(c.Context(), relationUUID, sts)
	c.Assert(err, tc.ErrorIsNil)

	// Assert:
	foundStatus := s.getRelationStatus(c, relationUUID)
	c.Assert(foundStatus, tc.DeepEquals, sts)
}

// TestSetRelationStatusMultipleTimes sets the status multiple times to ensure
// that it is updated correctly the second time.
func (s *stateSuite) TestSetRelationStatusMultipleTimes(c *tc.C) {
	// Arrange: Add relation and create statuses.
	relationUUID := s.addRelationWithLifeAndID(c, corelife.Alive, 7)
	now := time.Now().UTC()
	s.addRelationStatusWithMessage(c, relationUUID, corestatus.Joining, "", now)

	sts1 := status.StatusInfo[status.RelationStatusType]{
		Status:  status.RelationStatusTypeSuspended,
		Message: "message",
		Since:   ptr(now),
	}

	sts2 := status.StatusInfo[status.RelationStatusType]{
		Status:  status.RelationStatusTypeBroken,
		Message: "message2",
		Since:   ptr(now),
	}

	// Act: Set status twice.
	err := s.state.SetRelationStatus(c.Context(), relationUUID, sts1)
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.SetRelationStatus(c.Context(), relationUUID, sts2)
	c.Assert(err, tc.ErrorIsNil)

	// Assert:
	foundStatus := s.getRelationStatus(c, relationUUID)
	c.Assert(foundStatus, tc.DeepEquals, sts2)
}

// TestSetRelationStatusInvalidTransition checks that an invalid relation status
// transition is blocked.
func (s *stateSuite) TestSetRelationStatusInvalidTransition(c *tc.C) {
	// Arrange: Add relation and set status to broken.
	relationUUID := s.addRelationWithLifeAndID(c, corelife.Alive, 7)
	now := time.Now().UTC()
	s.addRelationStatusWithMessage(c, relationUUID, corestatus.Broken, "", now)

	// Arrange: Create joining status, which cannot be transitioned to from broken.
	sts := status.StatusInfo[status.RelationStatusType]{
		Status: status.RelationStatusTypeJoining,
		Since:  ptr(now),
	}

	// Act: Change status to suspended.
	err := s.state.SetRelationStatus(c.Context(), relationUUID, sts)

	// Assert:
	c.Assert(err, tc.ErrorIs, statuserrors.RelationStatusTransitionNotValid)
}

// TestSetRelationStatusSuspendingToSuspended checks that the message from
// Suspending status is preserved when the status is updated to Suspended.
func (s *stateSuite) TestSetRelationStatusSuspendingToSuspended(c *tc.C) {
	// Arrange: Add relation and create suspending status with message.
	relationUUID := s.addRelationWithLifeAndID(c, corelife.Alive, 7)
	now := time.Now().UTC()
	message := "suspending message"
	s.addRelationStatusWithMessage(c, relationUUID, corestatus.Suspending, message, now)

	// Arrange: Create suspended status without message to set.
	suspendedStatus := status.StatusInfo[status.RelationStatusType]{
		Status: status.RelationStatusTypeSuspended,
		Since:  ptr(now),
	}

	// Act: Change status to suspended.
	err := s.state.SetRelationStatus(c.Context(), relationUUID, suspendedStatus)
	c.Assert(err, tc.ErrorIsNil)

	// Assert:
	foundStatus := s.getRelationStatus(c, relationUUID)
	c.Assert(foundStatus, tc.DeepEquals, status.StatusInfo[status.RelationStatusType]{
		Status:  status.RelationStatusTypeSuspended,
		Message: message,
		Since:   ptr(now),
	})
}

func (s *stateSuite) TestSetRelationStatusRelationNotFound(c *tc.C) {
	// Arrange: Create relation and statuses.
	sts := status.StatusInfo[status.RelationStatusType]{
		Since: ptr(time.Now().UTC()),
	}

	// Act:
	err := s.state.SetRelationStatus(c.Context(), "bad-uuid", sts)

	// Assert:
	c.Assert(err, tc.ErrorIs, statuserrors.RelationNotFound)
}

func (s *stateSuite) TestGetRelationUUIDByID(c *tc.C) {
	relationID := 7
	relationUUID := s.addRelationWithLifeAndID(c, corelife.Alive, relationID)

	gotUUID, err := s.state.GetRelationUUIDByID(c.Context(), relationID)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(gotUUID, tc.Equals, relationUUID)
}

func (s *stateSuite) TestGetRelationUUIDByIDNotFound(c *tc.C) {
	_, err := s.state.GetRelationUUIDByID(c.Context(), 666)
	c.Assert(err, tc.ErrorIs, statuserrors.RelationNotFound)
}

func (s *stateSuite) TestImportRelationStatus(c *tc.C) {
	// Arrange: Create relation and statuses.
	relationID := 7
	relationUUID := s.addRelationWithLifeAndID(c, corelife.Alive, relationID)
	now := time.Now().UTC()
	s.addRelationStatusWithMessage(c, relationUUID, corestatus.Joining, "", now)

	sts := status.StatusInfo[status.RelationStatusType]{
		Status:  status.RelationStatusTypeSuspended,
		Message: "message",
		Since:   ptr(now),
	}

	// Act:
	err := s.state.ImportRelationStatus(c.Context(), relationUUID, sts)
	c.Assert(err, tc.ErrorIsNil)

	// Assert:
	foundStatus := s.getRelationStatus(c, relationUUID)
	c.Assert(foundStatus, tc.DeepEquals, sts)
}

func (s *stateSuite) TestImportRelationStatusRelationNotFound(c *tc.C) {
	// Arrange: Create relation and statuses.
	sts := status.StatusInfo[status.RelationStatusType]{
		Since: ptr(time.Now().UTC()),
	}

	// Act:
	err := s.state.ImportRelationStatus(c.Context(), corerelationtesting.GenRelationUUID(c), sts)

	// Assert:
	c.Assert(err, tc.ErrorIs, statuserrors.RelationNotFound)
}

func (s *stateSuite) getRelationStatus(c *tc.C, relationUUID corerelation.UUID) status.StatusInfo[status.RelationStatusType] {
	var (
		statusType int
		reason     string
		updated_at *time.Time
	)
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRow(`
SELECT relation_status_type_id, suspended_reason, updated_at
FROM   relation_status
WHERE  relation_uuid = ?
`, relationUUID).Scan(&statusType, &reason, &updated_at)
	})
	c.Assert(err, tc.ErrorIsNil)
	encodedStatus, err := status.DecodeRelationStatus(statusType)
	c.Assert(err, tc.ErrorIsNil)
	return status.StatusInfo[status.RelationStatusType]{
		Status:  encodedStatus,
		Message: reason,
		Since:   updated_at,
	}
}

func (s *stateSuite) TestSetK8sPodStatus(c *tc.C) {
	u1 := application.AddUnitArg{}
	_, unitUUIDs := s.createApplication(c, "foo", life.Alive, false, s.appStatus(time.Now()), u1)
	unitUUID := unitUUIDs[0]

	status := status.StatusInfo[status.K8sPodStatusType]{
		Status:  status.K8sPodStatusRunning,
		Message: "it's running",
		Data:    []byte(`{"foo": "bar"}`),
		Since:   ptr(time.Now()),
	}

	err := s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		return s.state.setK8sPodStatus(ctx, tx, unitUUID, status)
	})
	c.Assert(err, tc.ErrorIsNil)
	s.assertUnitStatus(
		c, "k8s_pod", unitUUID, int(status.Status), status.Message, status.Since, status.Data)
}

func (s *stateSuite) TestSetUnitAgentStatus(c *tc.C) {
	u1 := application.AddUnitArg{}
	_, unitUUIDs := s.createApplication(c, "foo", life.Alive, false, s.appStatus(time.Now()), u1)
	unitUUID := unitUUIDs[0]

	status := status.StatusInfo[status.UnitAgentStatusType]{
		Status:  status.UnitAgentStatusExecuting,
		Message: "it's executing",
		Data:    []byte(`{"foo": "bar"}`),
		Since:   ptr(time.Now()),
	}

	err := s.state.SetUnitAgentStatus(c.Context(), unitUUID, status)
	c.Assert(err, tc.ErrorIsNil)
	s.assertUnitStatus(
		c, "unit_agent", unitUUID, int(status.Status), status.Message, status.Since, status.Data)
}

func (s *stateSuite) TestSetUnitAgentStatusNotFound(c *tc.C) {
	status := status.StatusInfo[status.UnitAgentStatusType]{
		Status:  status.UnitAgentStatusExecuting,
		Message: "it's executing",
		Data:    []byte(`{"foo": "bar"}`),
		Since:   ptr(time.Now()),
	}

	unitUUID := unittesting.GenUnitUUID(c)

	err := s.state.SetUnitAgentStatus(c.Context(), unitUUID, status)
	c.Assert(err, tc.ErrorIs, statuserrors.UnitNotFound)
}

func (s *stateSuite) TestGetUnitAgentStatusUnset(c *tc.C) {
	u1 := application.AddUnitArg{}
	_, unitUUIDs := s.createApplication(c, "foo", life.Alive, false, s.appStatus(time.Now()), u1)
	unitUUID := unitUUIDs[0]

	_, err := s.state.GetUnitAgentStatus(c.Context(), unitUUID)
	c.Assert(err, tc.ErrorIs, statuserrors.UnitStatusNotFound)
}

func (s *stateSuite) TestGetUnitAgentStatusDead(c *tc.C) {
	u1 := application.AddUnitArg{}
	_, unitUUIDs := s.createApplication(c, "foo", life.Dead, false, s.appStatus(time.Now()), u1)
	unitUUID := unitUUIDs[0]

	_, err := s.state.GetUnitAgentStatus(c.Context(), unitUUID)
	c.Assert(err, tc.ErrorIs, statuserrors.UnitIsDead)
}

func (s *stateSuite) TestGetUnitAgentStatus(c *tc.C) {
	u1 := application.AddUnitArg{}
	_, unitUUIDs := s.createApplication(c, "foo", life.Alive, false, s.appStatus(time.Now()), u1)
	unitUUID := unitUUIDs[0]

	status := status.StatusInfo[status.UnitAgentStatusType]{
		Status:  status.UnitAgentStatusExecuting,
		Message: "it's executing",
		Data:    []byte(`{"foo": "bar"}`),
		Since:   ptr(time.Now()),
	}

	err := s.state.SetUnitAgentStatus(c.Context(), unitUUID, status)
	c.Assert(err, tc.ErrorIsNil)

	gotStatus, err := s.state.GetUnitAgentStatus(c.Context(), unitUUID)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(gotStatus.Present, tc.IsFalse)
	assertStatusInfoEqual(c, gotStatus.StatusInfo, status)
}

func (s *stateSuite) TestGetUnitAgentStatusPresent(c *tc.C) {
	u1 := application.AddUnitArg{}
	_, unitUUIDs := s.createApplication(c, "foo", life.Alive, false, s.appStatus(time.Now()), u1)
	unitUUID := unitUUIDs[0]

	status := status.StatusInfo[status.UnitAgentStatusType]{
		Status:  status.UnitAgentStatusExecuting,
		Message: "it's executing",
		Data:    []byte(`{"foo": "bar"}`),
		Since:   ptr(time.Now()),
	}

	err := s.state.SetUnitAgentStatus(c.Context(), unitUUID, status)
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.SetUnitPresence(c.Context(), coreunit.Name("foo/0"))
	c.Assert(err, tc.ErrorIsNil)

	gotStatus, err := s.state.GetUnitAgentStatus(c.Context(), unitUUID)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(gotStatus.Present, tc.IsTrue)
	assertStatusInfoEqual(c, gotStatus.StatusInfo, status)

	err = s.state.DeleteUnitPresence(c.Context(), coreunit.Name("foo/0"))
	c.Assert(err, tc.ErrorIsNil)

	gotStatus, err = s.state.GetUnitAgentStatus(c.Context(), unitUUID)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(gotStatus.Present, tc.IsFalse)
	assertStatusInfoEqual(c, gotStatus.StatusInfo, status)
}

func (s *stateSuite) TestGetUnitWorkloadStatusUnitNotFound(c *tc.C) {
	_, err := s.state.GetUnitWorkloadStatus(c.Context(), "missing-uuid")
	c.Assert(err, tc.ErrorIs, statuserrors.UnitNotFound)
}

func (s *stateSuite) TestGetUnitWorkloadStatusDead(c *tc.C) {
	u1 := application.AddUnitArg{}
	_, unitUUIDs := s.createApplication(c, "foo", life.Dead, false, s.appStatus(time.Now()), u1)
	unitUUID := unitUUIDs[0]

	_, err := s.state.GetUnitWorkloadStatus(c.Context(), unitUUID)
	c.Assert(err, tc.ErrorIs, statuserrors.UnitIsDead)
}

func (s *stateSuite) TestGetUnitWorkloadStatusUnsetStatus(c *tc.C) {
	u1 := application.AddUnitArg{}
	_, unitUUIDs := s.createApplication(c, "foo", life.Alive, false, s.appStatus(time.Now()), u1)
	unitUUID := unitUUIDs[0]

	_, err := s.state.GetUnitWorkloadStatus(c.Context(), unitUUID)
	c.Assert(err, tc.ErrorIs, statuserrors.UnitStatusNotFound)
}

func (s *stateSuite) TestSetWorkloadStatus(c *tc.C) {
	u1 := application.AddUnitArg{}
	_, unitUUIDs := s.createApplication(c, "foo", life.Alive, false, s.appStatus(time.Now()), u1)
	unitUUID := unitUUIDs[0]

	sts := status.StatusInfo[status.WorkloadStatusType]{
		Status:  status.WorkloadStatusActive,
		Message: "it's active!",
		Data:    []byte(`{"foo": "bar"}`),
		Since:   ptr(time.Now()),
	}

	err := s.state.SetUnitWorkloadStatus(c.Context(), unitUUID, sts)
	c.Assert(err, tc.ErrorIsNil)

	gotStatus, err := s.state.GetUnitWorkloadStatus(c.Context(), unitUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(gotStatus.Present, tc.IsFalse)
	assertStatusInfoEqual(c, gotStatus.StatusInfo, sts)

	// Run SetUnitWorkloadStatus followed by GetUnitWorkloadStatus to ensure that
	// the new status overwrites the old one.
	sts = status.StatusInfo[status.WorkloadStatusType]{
		Status:  status.WorkloadStatusTerminated,
		Message: "it's terminated",
		Data:    []byte(`{"bar": "foo"}`),
		Since:   ptr(time.Now()),
	}

	err = s.state.SetUnitWorkloadStatus(c.Context(), unitUUID, sts)
	c.Assert(err, tc.ErrorIsNil)

	gotStatus, err = s.state.GetUnitWorkloadStatus(c.Context(), unitUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(gotStatus.Present, tc.IsFalse)
	assertStatusInfoEqual(c, gotStatus.StatusInfo, sts)
}

func (s *stateSuite) TestSetUnitWorkloadStatusToError(c *tc.C) {
	u1 := application.AddUnitArg{}
	_, unitUUIDs := s.createApplication(c, "foo", life.Alive, false, s.appStatus(time.Now()), u1)
	unitUUID := unitUUIDs[0]

	sts := status.StatusInfo[status.WorkloadStatusType]{
		Status:  status.WorkloadStatusError,
		Message: "it's an error!",
		Data:    []byte("some data"),
		Since:   ptr(time.Now()),
	}

	err := s.state.SetUnitWorkloadStatus(c.Context(), unitUUID, sts)
	c.Assert(err, tc.ErrorIsNil)

	gotStatus, err := s.state.GetUnitWorkloadStatus(c.Context(), unitUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(gotStatus.Present, tc.IsFalse)
	assertStatusInfoEqual(c, gotStatus.StatusInfo, sts)
}

func (s *stateSuite) TestSetWorkloadStatusPresent(c *tc.C) {
	u1 := application.AddUnitArg{}
	_, unitUUIDs := s.createApplication(c, "foo", life.Alive, false, s.appStatus(time.Now()), u1)
	unitUUID := unitUUIDs[0]

	sts := status.StatusInfo[status.WorkloadStatusType]{
		Status:  status.WorkloadStatusActive,
		Message: "it's active!",
		Data:    []byte(`{"foo": "bar"}`),
		Since:   ptr(time.Now()),
	}

	err := s.state.SetUnitWorkloadStatus(c.Context(), unitUUID, sts)
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.SetUnitPresence(c.Context(), coreunit.Name("foo/0"))
	c.Assert(err, tc.ErrorIsNil)

	gotStatus, err := s.state.GetUnitWorkloadStatus(c.Context(), unitUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(gotStatus.Present, tc.IsTrue)
	assertStatusInfoEqual(c, gotStatus.StatusInfo, sts)

	// Run SetUnitWorkloadStatus followed by GetUnitWorkloadStatus to ensure that
	// the new status overwrites the old one.
	sts = status.StatusInfo[status.WorkloadStatusType]{
		Status:  status.WorkloadStatusTerminated,
		Message: "it's terminated",
		Data:    []byte(`{"bar": "foo"}`),
		Since:   ptr(time.Now()),
	}

	err = s.state.SetUnitWorkloadStatus(c.Context(), unitUUID, sts)
	c.Assert(err, tc.ErrorIsNil)

	gotStatus, err = s.state.GetUnitWorkloadStatus(c.Context(), unitUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(gotStatus.Present, tc.IsTrue)
	assertStatusInfoEqual(c, gotStatus.StatusInfo, sts)
}

func (s *stateSuite) TestSetUnitWorkloadStatusNotFound(c *tc.C) {
	status := status.StatusInfo[status.WorkloadStatusType]{
		Status:  status.WorkloadStatusTerminated,
		Message: "it's terminated",
		Data:    []byte(`{"foo": "bar"}`),
		Since:   ptr(time.Now()),
	}

	err := s.state.SetUnitWorkloadStatus(c.Context(), "missing-uuid", status)
	c.Assert(err, tc.ErrorIs, statuserrors.UnitNotFound)
}

func (s *stateSuite) TestGetUnitK8sPodStatusUnset(c *tc.C) {
	u1 := application.AddUnitArg{}
	_, unitUUIDs := s.createApplication(c, "foo", life.Alive, false, s.appStatus(time.Now()), u1)
	unitUUID := unitUUIDs[0]

	sts, err := s.state.GetUnitK8sPodStatus(c.Context(), unitUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(sts, tc.DeepEquals, status.StatusInfo[status.K8sPodStatusType]{
		Status: status.K8sPodStatusUnset,
	})
}

func (s *stateSuite) TestGetUnitK8sPodStatusUnitNotFound(c *tc.C) {
	_, err := s.state.GetUnitK8sPodStatus(c.Context(), "missing-uuid")
	c.Assert(err, tc.ErrorIs, statuserrors.UnitNotFound)
}

func (s *stateSuite) TestGetUnitK8sPodStatusDead(c *tc.C) {
	u1 := application.AddUnitArg{}
	_, unitUUIDs := s.createApplication(c, "foo", life.Dead, false, s.appStatus(time.Now()), u1)
	unitUUID := unitUUIDs[0]

	_, err := s.state.GetUnitK8sPodStatus(c.Context(), unitUUID)
	c.Assert(err, tc.ErrorIs, statuserrors.UnitIsDead)
}

func (s *stateSuite) TestGetUnitK8sPodStatus(c *tc.C) {
	u1 := application.AddUnitArg{}
	_, unitUUIDs := s.createApplication(c, "foo", life.Alive, false, s.appStatus(time.Now()), u1)
	unitUUID := unitUUIDs[0]

	now := time.Now()

	s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		return s.state.setK8sPodStatus(ctx, tx, unitUUID, status.StatusInfo[status.K8sPodStatusType]{
			Status:  status.K8sPodStatusRunning,
			Message: "it's running",
			Data:    []byte(`{"foo": "bar"}`),
			Since:   &now,
		})
	})

	sts, err := s.state.GetUnitK8sPodStatus(c.Context(), unitUUID)
	c.Assert(err, tc.ErrorIsNil)
	assertStatusInfoEqual(c, sts, status.StatusInfo[status.K8sPodStatusType]{
		Status:  status.K8sPodStatusRunning,
		Message: "it's running",
		Data:    []byte(`{"foo": "bar"}`),
		Since:   &now,
	})
}

func (s *stateSuite) TestGetUnitWorkloadStatusesForApplication(c *tc.C) {
	u1 := application.AddUnitArg{}
	appId, unitUUIDs := s.createApplication(c, "foo", life.Alive, false, s.appStatus(time.Now()), u1)
	unitUUID := unitUUIDs[0]

	status := status.StatusInfo[status.WorkloadStatusType]{
		Status:  status.WorkloadStatusActive,
		Message: "it's active!",
		Data:    []byte(`{"foo": "bar"}`),
		Since:   ptr(time.Now()),
	}
	err := s.state.SetUnitWorkloadStatus(c.Context(), unitUUID, status)
	c.Assert(err, tc.ErrorIsNil)

	results, err := s.state.GetUnitWorkloadStatusesForApplication(c.Context(), appId)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(results, tc.HasLen, 1)
	result, ok := results["foo/0"]
	c.Assert(ok, tc.IsTrue)
	c.Check(result.Present, tc.IsFalse)
	assertStatusInfoEqual(c, result.StatusInfo, status)
}

func (s *stateSuite) TestGetUnitWorkloadStatusesForApplicationMultipleUnits(c *tc.C) {
	u1 := application.AddUnitArg{}
	u2 := application.AddUnitArg{}
	appId, unitUUIDs := s.createApplication(c, "foo", life.Alive, false, s.appStatus(time.Now()), u1, u2)
	unitUUID1 := unitUUIDs[0]
	unitUUID2 := unitUUIDs[1]

	status1 := status.StatusInfo[status.WorkloadStatusType]{
		Status:  status.WorkloadStatusActive,
		Message: "it's active!",
		Data:    []byte(`{"foo": "bar"}`),
		Since:   ptr(time.Now()),
	}
	err := s.state.SetUnitWorkloadStatus(c.Context(), unitUUID1, status1)
	c.Assert(err, tc.ErrorIsNil)

	status2 := status.StatusInfo[status.WorkloadStatusType]{
		Status:  status.WorkloadStatusTerminated,
		Message: "it's terminated",
		Data:    []byte(`{"bar": "foo"}`),
		Since:   ptr(time.Now()),
	}
	err = s.state.SetUnitWorkloadStatus(c.Context(), unitUUID2, status2)
	c.Assert(err, tc.ErrorIsNil)

	results, err := s.state.GetUnitWorkloadStatusesForApplication(c.Context(), appId)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.HasLen, 2, tc.Commentf("expected 2, got %d", len(results)))

	result1, ok := results["foo/0"]
	c.Assert(ok, tc.IsTrue)
	c.Check(result1.Present, tc.IsFalse)
	assertStatusInfoEqual(c, result1.StatusInfo, status1)

	result2, ok := results["foo/1"]
	c.Assert(ok, tc.IsTrue)
	c.Check(result2.Present, tc.IsFalse)
	assertStatusInfoEqual(c, result2.StatusInfo, status2)
}

func (s *stateSuite) TestGetUnitWorkloadStatusesForApplicationMultipleUnitsPresent(c *tc.C) {
	u1 := application.AddUnitArg{}
	u2 := application.AddUnitArg{}
	appId, unitUUIDs := s.createApplication(c, "foo", life.Alive, false, s.appStatus(time.Now()), u1, u2)
	unitUUID1 := unitUUIDs[0]
	unitUUID2 := unitUUIDs[1]

	status1 := status.StatusInfo[status.WorkloadStatusType]{
		Status:  status.WorkloadStatusActive,
		Message: "it's active!",
		Data:    []byte(`{"foo": "bar"}`),
		Since:   ptr(time.Now()),
	}
	err := s.state.SetUnitWorkloadStatus(c.Context(), unitUUID1, status1)
	c.Assert(err, tc.ErrorIsNil)

	status2 := status.StatusInfo[status.WorkloadStatusType]{
		Status:  status.WorkloadStatusTerminated,
		Message: "it's terminated",
		Data:    []byte(`{"bar": "foo"}`),
		Since:   ptr(time.Now()),
	}
	err = s.state.SetUnitWorkloadStatus(c.Context(), unitUUID2, status2)
	c.Assert(err, tc.ErrorIsNil)
	err = s.state.SetUnitPresence(c.Context(), coreunit.Name("foo/1"))
	c.Assert(err, tc.ErrorIsNil)

	results, err := s.state.GetUnitWorkloadStatusesForApplication(c.Context(), appId)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.HasLen, 2, tc.Commentf("expected 2, got %d", len(results)))

	result1, ok := results["foo/0"]
	c.Assert(ok, tc.IsTrue)
	c.Check(result1.Present, tc.IsFalse)
	assertStatusInfoEqual(c, result1.StatusInfo, status1)

	result2, ok := results["foo/1"]
	c.Assert(ok, tc.IsTrue)
	c.Check(result2.Present, tc.IsTrue)
	assertStatusInfoEqual(c, result2.StatusInfo, status2)
}

func (s *stateSuite) TestGetUnitWorkloadStatusesForApplicationNotFound(c *tc.C) {
	_, err := s.state.GetUnitWorkloadStatusesForApplication(c.Context(), "missing")
	c.Assert(err, tc.ErrorIs, statuserrors.ApplicationNotFound)
}

func (s *stateSuite) TestGetUnitWorkloadStatusesForApplicationNoUnits(c *tc.C) {
	appId, _ := s.createApplication(c, "foo", life.Alive, false, s.appStatus(time.Now()))

	results, err := s.state.GetUnitWorkloadStatusesForApplication(c.Context(), appId)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.HasLen, 0)
}

func (s *stateSuite) TestGetAllUnitStatusesForApplication(c *tc.C) {
	u1 := application.AddUnitArg{}
	appId, unitUUIDs := s.createApplication(c, "foo", life.Alive, false, s.appStatus(time.Now()), u1)
	unitUUID := unitUUIDs[0]

	workloadStatus := status.StatusInfo[status.WorkloadStatusType]{
		Status:  status.WorkloadStatusActive,
		Message: "it's active",
		Data:    []byte(`{"bar": "foo"}`),
		Since:   ptr(time.Now()),
	}
	agentStatus := status.StatusInfo[status.UnitAgentStatusType]{
		Status:  status.UnitAgentStatusAllocating,
		Message: "it's allocating",
		Data:    []byte(`{"baz": "qux"}`),
		Since:   ptr(time.Now()),
	}
	k8sPodStatus := status.StatusInfo[status.K8sPodStatusType]{
		Status:  status.K8sPodStatusRunning,
		Message: "it's running",
		Data:    []byte(`{"foo": "bar"}`),
		Since:   ptr(time.Now()),
	}
	err := s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		err := s.state.setK8sPodStatus(ctx, tx, unitUUID, k8sPodStatus)
		if err != nil {
			return err
		}
		err = s.state.setUnitAgentStatus(ctx, tx, unitUUID, agentStatus)
		if err != nil {
			return err
		}
		err = s.state.setUnitWorkloadStatus(ctx, tx, unitUUID, workloadStatus)
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	fullStatuses, err := s.state.GetAllFullUnitStatusesForApplication(c.Context(), appId)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(fullStatuses, tc.HasLen, 1)
	fullStatus, ok := fullStatuses["foo/0"]
	c.Assert(ok, tc.IsTrue)

	assertStatusInfoEqual(c, fullStatus.WorkloadStatus, workloadStatus)
	assertStatusInfoEqual(c, fullStatus.AgentStatus, agentStatus)
	assertStatusInfoEqual(c, fullStatus.K8sPodStatus, k8sPodStatus)
}

func (s *stateSuite) TestGetUnitK8sPodStatusForApplicationMultipleUnits(c *tc.C) {
	u1 := application.AddUnitArg{}
	u2 := application.AddUnitArg{}
	appId, unitUUIDs := s.createApplication(c, "foo", life.Alive, false, s.appStatus(time.Now()), u1, u2)
	unitUUID1 := unitUUIDs[0]
	unitUUID2 := unitUUIDs[1]

	workloadStatus := status.StatusInfo[status.WorkloadStatusType]{
		Status: status.WorkloadStatusActive,
	}
	agentStatus := status.StatusInfo[status.UnitAgentStatusType]{
		Status: status.UnitAgentStatusIdle,
	}

	status1 := status.StatusInfo[status.K8sPodStatusType]{
		Status:  status.K8sPodStatusRunning,
		Message: "it's running!",
		Data:    []byte(`{"foo": "bar"}`),
		Since:   ptr(time.Now()),
	}
	err := s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		err := s.state.setUnitWorkloadStatus(ctx, tx, unitUUID1, workloadStatus)
		if err != nil {
			return err
		}
		err = s.state.setUnitAgentStatus(ctx, tx, unitUUID1, agentStatus)
		if err != nil {
			return err
		}
		return s.state.setK8sPodStatus(ctx, tx, unitUUID1, status1)
	})
	c.Assert(err, tc.ErrorIsNil)

	status2 := status.StatusInfo[status.K8sPodStatusType]{
		Status:  status.K8sPodStatusBlocked,
		Message: "it's blocked",
		Data:    []byte(`{"bar": "foo"}`),
		Since:   ptr(time.Now()),
	}
	err = s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		err := s.state.setUnitWorkloadStatus(ctx, tx, unitUUID2, workloadStatus)
		if err != nil {
			return err
		}
		err = s.state.setUnitAgentStatus(ctx, tx, unitUUID2, agentStatus)
		if err != nil {
			return err
		}
		return s.state.setK8sPodStatus(ctx, tx, unitUUID2, status2)
	})
	c.Assert(err, tc.ErrorIsNil)

	fullStatuses, err := s.state.GetAllFullUnitStatusesForApplication(c.Context(), appId)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(fullStatuses, tc.HasLen, 2)
	result1, ok := fullStatuses["foo/0"]
	c.Assert(ok, tc.IsTrue)
	assertStatusInfoEqual(c, result1.K8sPodStatus, status1)

	result2, ok := fullStatuses["foo/1"]
	c.Assert(ok, tc.IsTrue)
	assertStatusInfoEqual(c, result2.K8sPodStatus, status2)
}

func (s *stateSuite) TestGetAllUnitStatusesForApplicationNotFound(c *tc.C) {
	_, err := s.state.GetAllFullUnitStatusesForApplication(c.Context(), "missing")
	c.Assert(err, tc.ErrorIs, statuserrors.ApplicationNotFound)
}

func (s *stateSuite) TestGetAllUnitStatusesForApplicationNoUnits(c *tc.C) {
	appId, _ := s.createApplication(c, "foo", life.Alive, false, s.appStatus(time.Now()))

	fullStatuses, err := s.state.GetAllFullUnitStatusesForApplication(c.Context(), appId)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(fullStatuses, tc.HasLen, 0)
}

func (s *stateSuite) TestGetAllUnitStatusesForApplicationUnitsWithoutStatuses(c *tc.C) {
	u1 := application.AddUnitArg{}
	u2 := application.AddUnitArg{}
	appId, _ := s.createApplication(c, "foo", life.Alive, false, s.appStatus(time.Now()), u1, u2)

	_, err := s.state.GetAllFullUnitStatusesForApplication(c.Context(), appId)
	c.Assert(err, tc.ErrorIs, statuserrors.UnitStatusNotFound)
}

func (s *stateSuite) TestGetAllFullUnitStatusesEmptyModel(c *tc.C) {
	res, err := s.state.GetAllUnitWorkloadAgentStatuses(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(res, tc.HasLen, 0)
}

func (s *stateSuite) TestGetAllFullUnitStatusesNotFound(c *tc.C) {
	u1 := application.AddUnitArg{}
	s.createApplication(c, "foo", life.Alive, false, s.appStatus(time.Now()), u1)

	_, err := s.state.GetAllUnitWorkloadAgentStatuses(c.Context())
	c.Assert(err, tc.ErrorIs, statuserrors.UnitStatusNotFound)
}

func (s *stateSuite) TestGetAllFullUnitStatuses(c *tc.C) {
	u1 := application.AddUnitArg{}
	u2 := application.AddUnitArg{}
	u3 := application.AddUnitArg{}
	_, fooUnitUUIDs := s.createApplication(c, "foo", life.Alive, false, s.appStatus(time.Now()), u1, u2)
	u1UUID := fooUnitUUIDs[0]
	u2UUID := fooUnitUUIDs[1]
	_, barUnitUUIDs := s.createApplication(c, "bar", life.Alive, false, s.appStatus(time.Now()), u3)
	u3UUID := barUnitUUIDs[0]

	u1Workload := status.StatusInfo[status.WorkloadStatusType]{
		Status:  status.WorkloadStatusActive,
		Message: "u1 is active!",
		Data:    []byte(`{"u1": "workload"}`),
		Since:   ptr(time.Now()),
	}
	err := s.state.SetUnitWorkloadStatus(c.Context(), u1UUID, u1Workload)
	c.Assert(err, tc.ErrorIsNil)

	u1Agent := status.StatusInfo[status.UnitAgentStatusType]{
		Status:  status.UnitAgentStatusIdle,
		Message: "u1 is idle!",
		Data:    []byte(`{"u1": "agent"}`),
		Since:   ptr(time.Now()),
	}
	err = s.state.SetUnitAgentStatus(c.Context(), u1UUID, u1Agent)
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.SetUnitPresence(c.Context(), "foo/0")
	c.Assert(err, tc.ErrorIsNil)

	u2Workload := status.StatusInfo[status.WorkloadStatusType]{
		Status:  status.WorkloadStatusBlocked,
		Message: "u2 is blocked!",
		Data:    []byte(`{"u2": "workload"}`),
		Since:   ptr(time.Now()),
	}
	err = s.state.SetUnitWorkloadStatus(c.Context(), u2UUID, u2Workload)
	c.Assert(err, tc.ErrorIsNil)

	u2Agent := status.StatusInfo[status.UnitAgentStatusType]{
		Status:  status.UnitAgentStatusAllocating,
		Message: "u2 is allocating!",
		Data:    []byte(`{"u2": "agent"}`),
		Since:   ptr(time.Now()),
	}
	err = s.state.SetUnitAgentStatus(c.Context(), u2UUID, u2Agent)
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.SetUnitPresence(c.Context(), "foo/1")
	c.Assert(err, tc.ErrorIsNil)
	err = s.state.DeleteUnitPresence(c.Context(), "foo/1")
	c.Assert(err, tc.ErrorIsNil)

	u3Workload := status.StatusInfo[status.WorkloadStatusType]{
		Status:  status.WorkloadStatusMaintenance,
		Message: "u3 is maintenance!",
		Data:    []byte(`{"u3": "workload"}`),
		Since:   ptr(time.Now()),
	}
	err = s.state.SetUnitWorkloadStatus(c.Context(), u3UUID, u3Workload)
	c.Assert(err, tc.ErrorIsNil)

	u3Agent := status.StatusInfo[status.UnitAgentStatusType]{
		Status:  status.UnitAgentStatusRebooting,
		Message: "u3 is rebooting!",
		Data:    []byte(`{"u3": "agent"}`),
		Since:   ptr(time.Now()),
	}
	err = s.state.SetUnitAgentStatus(c.Context(), u3UUID, u3Agent)
	c.Assert(err, tc.ErrorIsNil)

	res, err := s.state.GetAllUnitWorkloadAgentStatuses(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(res, tc.HasLen, 3)

	u1Full, ok := res["foo/0"]
	c.Assert(ok, tc.IsTrue)
	c.Check(u1Full.WorkloadStatus.Status, tc.Equals, status.WorkloadStatusActive)
	c.Check(u1Full.WorkloadStatus.Message, tc.Equals, "u1 is active!")
	c.Check(u1Full.WorkloadStatus.Data, tc.DeepEquals, []byte(`{"u1": "workload"}`))
	c.Check(u1Full.AgentStatus.Status, tc.Equals, status.UnitAgentStatusIdle)
	c.Check(u1Full.AgentStatus.Message, tc.Equals, "u1 is idle!")
	c.Check(u1Full.AgentStatus.Data, tc.DeepEquals, []byte(`{"u1": "agent"}`))
	c.Check(u1Full.Present, tc.Equals, true)

	u2Full, ok := res["foo/1"]
	c.Assert(ok, tc.IsTrue)
	c.Check(u2Full.WorkloadStatus.Status, tc.Equals, status.WorkloadStatusBlocked)
	c.Check(u2Full.WorkloadStatus.Message, tc.Equals, "u2 is blocked!")
	c.Check(u2Full.WorkloadStatus.Data, tc.DeepEquals, []byte(`{"u2": "workload"}`))
	c.Check(u2Full.AgentStatus.Status, tc.Equals, status.UnitAgentStatusAllocating)
	c.Check(u2Full.AgentStatus.Message, tc.Equals, "u2 is allocating!")
	c.Check(u2Full.AgentStatus.Data, tc.DeepEquals, []byte(`{"u2": "agent"}`))
	c.Check(u2Full.Present, tc.Equals, false)

	u3Full, ok := res["bar/0"]
	c.Assert(ok, tc.IsTrue)
	c.Check(u3Full.WorkloadStatus.Status, tc.Equals, status.WorkloadStatusMaintenance)
	c.Check(u3Full.WorkloadStatus.Message, tc.Equals, "u3 is maintenance!")
	c.Check(u3Full.WorkloadStatus.Data, tc.DeepEquals, []byte(`{"u3": "workload"}`))
	c.Check(u3Full.AgentStatus.Status, tc.Equals, status.UnitAgentStatusRebooting)
	c.Check(u3Full.AgentStatus.Message, tc.Equals, "u3 is rebooting!")
	c.Check(u3Full.AgentStatus.Data, tc.DeepEquals, []byte(`{"u3": "agent"}`))
	c.Check(u3Full.Present, tc.Equals, false)
}

func (s *stateSuite) TestGetAllApplicationStatusesEmptyModel(c *tc.C) {
	statuses, err := s.state.GetAllApplicationStatuses(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(statuses, tc.HasLen, 0)
}

func (s *stateSuite) TestGetAllApplicationStatusesUnsetStatuses(c *tc.C) {
	u1 := application.AddUnitArg{}
	s.createApplication(c, "foo", life.Alive, false, nil, u1)
	s.createApplication(c, "bar", life.Alive, false, nil)

	statuses, err := s.state.GetAllApplicationStatuses(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(statuses, tc.HasLen, 0)
}

func (s *stateSuite) TestGetAllApplicationStatuses(c *tc.C) {
	u1 := application.AddUnitArg{}
	app1ID, _ := s.createApplication(c, "foo", life.Alive, false, s.appStatus(time.Now()), u1)
	app2ID, _ := s.createApplication(c, "bar", life.Alive, false, s.appStatus(time.Now()))
	s.createApplication(c, "goo", life.Alive, false, s.appStatus(time.Now()))

	app1Status := status.StatusInfo[status.WorkloadStatusType]{
		Status:  status.WorkloadStatusActive,
		Message: "it's active",
		Data:    []byte(`{"foo": "bar"}`),
		Since:   ptr(time.Now()),
	}
	app2Status := status.StatusInfo[status.WorkloadStatusType]{
		Status:  status.WorkloadStatusBlocked,
		Message: "it's blocked",
		Data:    []byte(`{"bar": "foo"}`),
		Since:   ptr(time.Now()),
	}
	s.state.SetApplicationStatus(c.Context(), app1ID, app1Status)
	s.state.SetApplicationStatus(c.Context(), app2ID, app2Status)

	statuses, err := s.state.GetAllApplicationStatuses(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	res1, ok := statuses["foo"]
	c.Assert(ok, tc.IsTrue)
	assertStatusInfoEqual(c, res1, app1Status)

	res2, ok := statuses["bar"]
	c.Assert(ok, tc.IsTrue)
	assertStatusInfoEqual(c, res2, app2Status)
}

func (s *stateSuite) TestSetUnitPresence(c *tc.C) {
	u1 := application.AddUnitArg{}
	u2 := application.AddUnitArg{}
	s.createApplication(c, "foo", life.Alive, false, s.appStatus(time.Now()), u1, u2)

	err := s.state.SetUnitPresence(c.Context(), "foo/0")
	c.Assert(err, tc.ErrorIsNil)

	var lastSeen time.Time
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, "SELECT last_seen FROM v_unit_agent_presence WHERE name=?", "foo/0").Scan(&lastSeen); err != nil {
			return err
		}
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(lastSeen.IsZero(), tc.IsFalse)
	c.Check(lastSeen.After(time.Now().Add(-time.Minute)), tc.IsTrue)
}

func (s *stateSuite) TestSetUnitPresenceNotFound(c *tc.C) {
	err := s.state.SetUnitPresence(c.Context(), "foo/665")
	c.Assert(err, tc.ErrorIs, statuserrors.UnitNotFound)
}

func (s *stateSuite) TestDeleteUnitPresenceNotFound(c *tc.C) {
	err := s.state.DeleteUnitPresence(c.Context(), "foo/665")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *stateSuite) TestDeleteUnitPresence(c *tc.C) {
	u1 := application.AddUnitArg{}
	u2 := application.AddUnitArg{}
	s.createApplication(c, "foo", life.Alive, false, s.appStatus(time.Now()), u1, u2)

	err := s.state.SetUnitPresence(c.Context(), "foo/0")
	c.Assert(err, tc.ErrorIsNil)

	var lastSeen time.Time
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, "SELECT last_seen FROM v_unit_agent_presence WHERE name=?", "foo/0").Scan(&lastSeen); err != nil {
			return err
		}
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(lastSeen.IsZero(), tc.IsFalse)
	c.Check(lastSeen.After(time.Now().Add(-time.Minute)), tc.IsTrue)

	err = s.state.DeleteUnitPresence(c.Context(), "foo/0")
	c.Assert(err, tc.ErrorIsNil)

	var count int
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM v_unit_agent_presence WHERE name=?", "foo/0").Scan(&count); err != nil {
			return err
		}
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, 0)
}

func (s *stateSuite) TestGetApplicationAndUnitStatusesNoApplications(c *tc.C) {
	statuses, err := s.state.GetApplicationAndUnitStatuses(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(statuses, tc.DeepEquals, map[string]status.Application{})
}

func (s *stateSuite) TestGetApplicationAndUnitStatusesNoAppStatuses(c *tc.C) {
	u1 := application.AddUnitArg{}
	u2 := application.AddUnitArg{}
	appUUID, _ := s.createApplication(c, "foo", life.Alive, false, nil, u1, u2)

	statuses, err := s.state.GetApplicationAndUnitStatuses(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(statuses, tc.DeepEquals, map[string]status.Application{
		"foo": {
			ID:   appUUID,
			Life: life.Alive,
			CharmLocator: charm.CharmLocator{
				Name:         "foo",
				Revision:     42,
				Source:       "charmhub",
				Architecture: architecture.ARM64,
			},
			Platform: deployment.Platform{
				OSType:       deployment.Ubuntu,
				Channel:      "22.04/stable",
				Architecture: architecture.ARM64,
			},
			Channel: &deployment.Channel{
				Track:  "track",
				Risk:   "stable",
				Branch: "branch",
			},
			Scale: ptr(2),
			Units: map[coreunit.Name]status.Unit{
				"foo/0": {
					Life:            life.Alive,
					ApplicationName: "foo",
					CharmLocator: charm.CharmLocator{
						Name:         "foo",
						Revision:     42,
						Source:       "charmhub",
						Architecture: architecture.ARM64,
					},
				},
				"foo/1": {
					Life:            life.Alive,
					ApplicationName: "foo",
					CharmLocator: charm.CharmLocator{
						Name:         "foo",
						Revision:     42,
						Source:       "charmhub",
						Architecture: architecture.ARM64,
					},
				},
			},
		},
	})
}

func (s *stateSuite) TestGetApplicationAndUnitStatuses(c *tc.C) {
	now := time.Now()

	u1 := application.AddUnitArg{
		UnitStatusArg: application.UnitStatusArg{
			AgentStatus: &status.StatusInfo[status.UnitAgentStatusType]{
				Status:  status.UnitAgentStatusIdle,
				Message: "it's idle",
				Data:    []byte(`{"foo": "bar"}`),
				Since:   ptr(now),
			},
			WorkloadStatus: &status.StatusInfo[status.WorkloadStatusType]{
				Status:  status.WorkloadStatusActive,
				Message: "it's active",
				Data:    []byte(`{"bar": "foo"}`),
				Since:   ptr(now),
			},
		},
	}
	u2 := application.AddUnitArg{
		UnitStatusArg: application.UnitStatusArg{
			AgentStatus: &status.StatusInfo[status.UnitAgentStatusType]{
				Status:  status.UnitAgentStatusError,
				Message: "error",
				Data:    []byte(`{"error": "error"}`),
				Since:   ptr(now),
			},
			WorkloadStatus: &status.StatusInfo[status.WorkloadStatusType]{
				Status:  status.WorkloadStatusError,
				Message: "also in error",
				Data:    []byte(`{"error": "oh noes"}`),
				Since:   ptr(now),
			},
		},
	}

	appStatus := s.appStatus(now)
	appUUID, _ := s.createApplication(c, "foo", life.Alive, false, appStatus, u1, u2)

	statuses, err := s.state.GetApplicationAndUnitStatuses(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(statuses, tc.DeepEquals, map[string]status.Application{
		"foo": {
			ID:     appUUID,
			Life:   life.Alive,
			Status: *appStatus,
			CharmLocator: charm.CharmLocator{
				Name:         "foo",
				Revision:     42,
				Source:       "charmhub",
				Architecture: architecture.ARM64,
			},
			Platform: deployment.Platform{
				OSType:       deployment.Ubuntu,
				Channel:      "22.04/stable",
				Architecture: architecture.ARM64,
			},
			Channel: &deployment.Channel{
				Track:  "track",
				Risk:   "stable",
				Branch: "branch",
			},
			Scale: ptr(2),
			Units: map[coreunit.Name]status.Unit{
				"foo/0": {
					Life:            life.Alive,
					ApplicationName: "foo",
					AgentStatus: status.StatusInfo[status.UnitAgentStatusType]{
						Status:  status.UnitAgentStatusIdle,
						Message: "it's idle",
						Data:    []byte(`{"foo": "bar"}`),
						Since:   ptr(now),
					},
					WorkloadStatus: status.StatusInfo[status.WorkloadStatusType]{
						Status:  status.WorkloadStatusActive,
						Message: "it's active",
						Data:    []byte(`{"bar": "foo"}`),
						Since:   ptr(now),
					},
					K8sPodStatus: status.StatusInfo[status.K8sPodStatusType]{
						Status: status.K8sPodStatusUnset,
						Since:  ptr(now),
					},
					CharmLocator: charm.CharmLocator{
						Name:         "foo",
						Revision:     42,
						Source:       "charmhub",
						Architecture: architecture.ARM64,
					},
				},
				"foo/1": {
					Life:            life.Alive,
					ApplicationName: "foo",
					AgentStatus: status.StatusInfo[status.UnitAgentStatusType]{
						Status:  status.UnitAgentStatusError,
						Message: "error",
						Data:    []byte(`{"error": "error"}`),
						Since:   ptr(now),
					},
					WorkloadStatus: status.StatusInfo[status.WorkloadStatusType]{
						Status:  status.WorkloadStatusError,
						Message: "also in error",
						Data:    []byte(`{"error": "oh noes"}`),
						Since:   ptr(now),
					},
					K8sPodStatus: status.StatusInfo[status.K8sPodStatusType]{
						Status: status.K8sPodStatusUnset,
						Since:  ptr(now),
					},
					CharmLocator: charm.CharmLocator{
						Name:         "foo",
						Revision:     42,
						Source:       "charmhub",
						Architecture: architecture.ARM64,
					},
				},
			},
		},
	})
}

func (s *stateSuite) TestGetApplicationAndUnitStatusesSubordinate(c *tc.C) {
	now := time.Now()
	u1 := application.AddUnitArg{}
	u2 := application.AddUnitArg{}
	u3 := application.AddUnitArg{
		UnitStatusArg: application.UnitStatusArg{
			AgentStatus: &status.StatusInfo[status.UnitAgentStatusType]{
				Status:  status.UnitAgentStatusError,
				Message: "error",
				Data:    []byte(`{"error": "error"}`),
				Since:   ptr(now),
			},
			WorkloadStatus: &status.StatusInfo[status.WorkloadStatusType]{
				Status:  status.WorkloadStatusError,
				Message: "also in error",
				Data:    []byte(`{"error": "oh noes"}`),
				Since:   ptr(now),
			},
		},
	}

	appStatus := s.appStatus(now)
	appUUID0, units0 := s.createApplication(c, "foo", life.Alive, false, appStatus, u1)
	c.Assert(units0, tc.HasLen, 1)

	appUUID1, units1 := s.createApplication(c, "sub", life.Alive, true, appStatus, u2, u3)
	c.Assert(units1, tc.HasLen, 2)
	for _, unit := range units1 {
		s.setApplicationSubordinate(c, units0[0], unit)
	}

	statuses, err := s.state.GetApplicationAndUnitStatuses(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(statuses, tc.DeepEquals, map[string]status.Application{
		"foo": {
			ID:          appUUID0,
			Life:        life.Alive,
			Status:      *appStatus,
			Subordinate: false,
			CharmLocator: charm.CharmLocator{
				Name:         "foo",
				Revision:     42,
				Source:       "charmhub",
				Architecture: architecture.ARM64,
			},
			Platform: deployment.Platform{
				OSType:       deployment.Ubuntu,
				Channel:      "22.04/stable",
				Architecture: architecture.ARM64,
			},
			Channel: &deployment.Channel{
				Track:  "track",
				Risk:   "stable",
				Branch: "branch",
			},
			Scale: ptr(1),
			Units: map[coreunit.Name]status.Unit{
				"foo/0": {
					Life:            life.Alive,
					ApplicationName: "foo",
					SubordinateNames: map[coreunit.Name]struct{}{
						"sub/0": {},
						"sub/1": {},
					},
					CharmLocator: charm.CharmLocator{
						Name:         "foo",
						Revision:     42,
						Source:       "charmhub",
						Architecture: architecture.ARM64,
					},
				},
			},
		},
		"sub": {
			ID:          appUUID1,
			Life:        life.Alive,
			Status:      *appStatus,
			Subordinate: true,
			CharmLocator: charm.CharmLocator{
				Name:         "sub",
				Revision:     42,
				Source:       "charmhub",
				Architecture: architecture.ARM64,
			},
			Platform: deployment.Platform{
				OSType:       deployment.Ubuntu,
				Channel:      "22.04/stable",
				Architecture: architecture.ARM64,
			},
			Channel: &deployment.Channel{
				Track:  "track",
				Risk:   "stable",
				Branch: "branch",
			},
			Scale: ptr(2),
			Units: map[coreunit.Name]status.Unit{
				"sub/0": {
					Life:            life.Alive,
					ApplicationName: "sub",
					Subordinate:     true,
					PrincipalName:   ptr(coreunit.Name("foo/0")),
					CharmLocator: charm.CharmLocator{
						Name:         "sub",
						Revision:     42,
						Source:       "charmhub",
						Architecture: architecture.ARM64,
					},
				},
				"sub/1": {
					Life:            life.Alive,
					ApplicationName: "sub",
					Subordinate:     true,
					PrincipalName:   ptr(coreunit.Name("foo/0")),
					AgentStatus: status.StatusInfo[status.UnitAgentStatusType]{
						Status:  status.UnitAgentStatusError,
						Message: "error",
						Data:    []byte(`{"error": "error"}`),
						Since:   ptr(now),
					},
					WorkloadStatus: status.StatusInfo[status.WorkloadStatusType]{
						Status:  status.WorkloadStatusError,
						Message: "also in error",
						Data:    []byte(`{"error": "oh noes"}`),
						Since:   ptr(now),
					},
					K8sPodStatus: status.StatusInfo[status.K8sPodStatusType]{
						Status: status.K8sPodStatusUnset,
						Since:  ptr(now),
					},
					CharmLocator: charm.CharmLocator{
						Name:         "sub",
						Revision:     42,
						Source:       "charmhub",
						Architecture: architecture.ARM64,
					},
				},
			},
		},
	})
}

func (s *stateSuite) TestGetApplicationAndUnitStatusesLXDProfile(c *tc.C) {
	u1 := application.AddUnitArg{}
	u2 := application.AddUnitArg{}
	now := time.Now()

	appStatus := s.appStatus(now)
	appUUID, _ := s.createApplication(c, "foo", life.Alive, false, appStatus, u1, u2)
	s.setApplicationLXDProfile(c, appUUID, "{}")

	statuses, err := s.state.GetApplicationAndUnitStatuses(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(statuses, tc.DeepEquals, map[string]status.Application{
		"foo": {
			ID:     appUUID,
			Life:   life.Alive,
			Status: *appStatus,
			CharmLocator: charm.CharmLocator{
				Name:         "foo",
				Revision:     42,
				Source:       "charmhub",
				Architecture: architecture.ARM64,
			},
			Platform: deployment.Platform{
				OSType:       deployment.Ubuntu,
				Channel:      "22.04/stable",
				Architecture: architecture.ARM64,
			},
			Channel: &deployment.Channel{
				Track:  "track",
				Risk:   "stable",
				Branch: "branch",
			},
			LXDProfile: []byte("{}"),
			Scale:      ptr(2),
			Units: map[coreunit.Name]status.Unit{
				"foo/0": {
					Life:            life.Alive,
					ApplicationName: "foo",
					CharmLocator: charm.CharmLocator{
						Name:         "foo",
						Revision:     42,
						Source:       "charmhub",
						Architecture: architecture.ARM64,
					},
				},
				"foo/1": {
					Life:            life.Alive,
					ApplicationName: "foo",
					CharmLocator: charm.CharmLocator{
						Name:         "foo",
						Revision:     42,
						Source:       "charmhub",
						Architecture: architecture.ARM64,
					},
				},
			},
		},
	})
}

func (s *stateSuite) TestGetApplicationAndUnitStatusesWorkloadVersion(c *tc.C) {
	u1 := application.AddUnitArg{}
	u2 := application.AddUnitArg{}
	now := time.Now()

	appStatus := s.appStatus(now)
	appUUID, unitUUDs := s.createApplication(c, "foo", life.Alive, false, appStatus, u1, u2)
	c.Assert(unitUUDs, tc.HasLen, 2)
	s.setWorkloadVersion(c, appUUID, unitUUDs[0], "blah")

	statuses, err := s.state.GetApplicationAndUnitStatuses(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(statuses, tc.DeepEquals, map[string]status.Application{
		"foo": {
			ID:     appUUID,
			Life:   life.Alive,
			Status: *appStatus,
			CharmLocator: charm.CharmLocator{
				Name:         "foo",
				Revision:     42,
				Source:       "charmhub",
				Architecture: architecture.ARM64,
			},
			Platform: deployment.Platform{
				OSType:       deployment.Ubuntu,
				Channel:      "22.04/stable",
				Architecture: architecture.ARM64,
			},
			Channel: &deployment.Channel{
				Track:  "track",
				Risk:   "stable",
				Branch: "branch",
			},
			Scale:           ptr(2),
			WorkloadVersion: ptr("blah"),
			Units: map[coreunit.Name]status.Unit{
				"foo/0": {
					Life:            life.Alive,
					ApplicationName: "foo",
					CharmLocator: charm.CharmLocator{
						Name:         "foo",
						Revision:     42,
						Source:       "charmhub",
						Architecture: architecture.ARM64,
					},
					WorkloadVersion: ptr("blah"),
				},
				"foo/1": {
					Life:            life.Alive,
					ApplicationName: "foo",
					CharmLocator: charm.CharmLocator{
						Name:         "foo",
						Revision:     42,
						Source:       "charmhub",
						Architecture: architecture.ARM64,
					},
				},
			},
		},
	})
}

func (s *stateSuite) setWorkloadVersion(c *tc.C, appUUID coreapplication.ID, unitUUID coreunit.UUID, version string) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `UPDATE application_workload_version SET version=? WHERE application_uuid=?`, version, appUUID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE unit_workload_version SET version=? WHERE unit_uuid=?`, version, unitUUID); err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *stateSuite) TestGetApplicationAndUnitStatusesWithRelations(c *tc.C) {
	u1 := application.AddUnitArg{}
	u2 := application.AddUnitArg{}
	now := time.Now()

	appStatus := s.appStatus(now)
	appUUID, _ := s.createApplication(c, "foo", life.Alive, false, appStatus, u1, u2)

	relationUUID := s.addRelationWithLifeAndID(c, corelife.Alive, 7)
	s.addRelationStatusWithMessage(c, relationUUID, corestatus.Active, "this is a test", now)
	s.addRelationToApplication(c, appUUID, relationUUID)

	statuses, err := s.state.GetApplicationAndUnitStatuses(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(statuses, tc.DeepEquals, map[string]status.Application{
		"foo": {
			ID:     appUUID,
			Life:   life.Alive,
			Status: *appStatus,
			CharmLocator: charm.CharmLocator{
				Name:         "foo",
				Revision:     42,
				Source:       "charmhub",
				Architecture: architecture.ARM64,
			},
			Relations: []corerelation.UUID{
				relationUUID,
			},
			Platform: deployment.Platform{
				OSType:       deployment.Ubuntu,
				Channel:      "22.04/stable",
				Architecture: architecture.ARM64,
			},
			Channel: &deployment.Channel{
				Track:  "track",
				Risk:   "stable",
				Branch: "branch",
			},
			Exposed: true,
			Scale:   ptr(2),
			Units: map[coreunit.Name]status.Unit{
				"foo/0": {
					Life:            life.Alive,
					ApplicationName: "foo",
					CharmLocator: charm.CharmLocator{
						Name:         "foo",
						Revision:     42,
						Source:       "charmhub",
						Architecture: architecture.ARM64,
					},
				},
				"foo/1": {
					Life:            life.Alive,
					ApplicationName: "foo",
					CharmLocator: charm.CharmLocator{
						Name:         "foo",
						Revision:     42,
						Source:       "charmhub",
						Architecture: architecture.ARM64,
					},
				},
			},
		},
	})
}

func (s *stateSuite) TestGetApplicationAndUnitStatusesWithMultipleRelations(c *tc.C) {
	u1 := application.AddUnitArg{}
	u2 := application.AddUnitArg{}
	now := time.Now()

	appStatus := s.appStatus(now)
	appUUID, _ := s.createApplication(c, "foo", life.Alive, false, appStatus, u1, u2)

	var relations []corerelation.UUID
	for range 3 {
		relationUUID := s.addRelationWithLifeAndID(c, corelife.Alive, 7+len(relations))
		s.addRelationStatusWithMessage(c, relationUUID, corestatus.Active, "this is a test", now)
		s.addRelationToApplication(c, appUUID, relationUUID)
		relations = append(relations, relationUUID)
	}
	sort.Slice(relations, func(i, j int) bool {
		return relations[i].String() < relations[j].String()
	})

	statuses, err := s.state.GetApplicationAndUnitStatuses(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(statuses, tc.DeepEquals, map[string]status.Application{
		"foo": {
			ID:     appUUID,
			Life:   life.Alive,
			Status: *appStatus,
			CharmLocator: charm.CharmLocator{
				Name:         "foo",
				Revision:     42,
				Source:       "charmhub",
				Architecture: architecture.ARM64,
			},
			Relations: relations,
			Platform: deployment.Platform{
				OSType:       deployment.Ubuntu,
				Channel:      "22.04/stable",
				Architecture: architecture.ARM64,
			},
			Channel: &deployment.Channel{
				Track:  "track",
				Risk:   "stable",
				Branch: "branch",
			},
			Exposed: true,
			Scale:   ptr(2),
			Units: map[coreunit.Name]status.Unit{
				"foo/0": {
					Life:            life.Alive,
					ApplicationName: "foo",
					CharmLocator: charm.CharmLocator{
						Name:         "foo",
						Revision:     42,
						Source:       "charmhub",
						Architecture: architecture.ARM64,
					},
				},
				"foo/1": {
					Life:            life.Alive,
					ApplicationName: "foo",
					CharmLocator: charm.CharmLocator{
						Name:         "foo",
						Revision:     42,
						Source:       "charmhub",
						Architecture: architecture.ARM64,
					},
				},
			},
		},
	})
}

func (s *stateSuite) TestGetApplicationAndUnitModelStatuses(c *tc.C) {
	u1 := application.AddUnitArg{}
	u2 := application.AddUnitArg{}
	now := time.Now()

	appStatus := s.appStatus(now)
	s.createApplication(c, "foo", life.Alive, false, appStatus, u1, u2)

	appUnitCount, err := s.state.GetApplicationAndUnitModelStatuses(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	c.Check(appUnitCount, tc.DeepEquals, map[string]int{
		"foo": 2,
	})
}

func (s *stateSuite) TestGetApplicationAndUnitModelStatusesMultiple(c *tc.C) {
	u1 := application.AddUnitArg{}
	u2 := application.AddUnitArg{}
	u3 := application.AddUnitArg{}
	now := time.Now()

	appStatus := s.appStatus(now)
	s.createApplication(c, "foo", life.Alive, false, appStatus, u1, u2)
	s.createApplication(c, "bar", life.Alive, false, appStatus, u3)

	appUnitCount, err := s.state.GetApplicationAndUnitModelStatuses(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	c.Check(appUnitCount, tc.DeepEquals, map[string]int{
		"foo": 2,
		"bar": 1,
	})
}

func (s *stateSuite) TestGetApplicationAndUnitModelStatusesNoApplication(c *tc.C) {
	appUnitCount, err := s.state.GetApplicationAndUnitModelStatuses(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	c.Check(appUnitCount, tc.DeepEquals, map[string]int{})
}

func (s *stateSuite) TestGetApplicationAndUnitModelStatusesNoUnits(c *tc.C) {
	now := time.Now()
	appStatus := s.appStatus(now)
	s.createApplication(c, "foo", life.Alive, false, appStatus)

	appUnitCount, err := s.state.GetApplicationAndUnitModelStatuses(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	c.Check(appUnitCount, tc.DeepEquals, map[string]int{
		"foo": 0,
	})
}

func (s *stateSuite) appStatus(now time.Time) *status.StatusInfo[status.WorkloadStatusType] {
	return &status.StatusInfo[status.WorkloadStatusType]{
		Status:  status.WorkloadStatusActive,
		Message: "it's active!",
		Data:    []byte(`{"foo": "bar"}`),
		Since:   ptr(now),
	}
}

// addRelationWithLifeAndID inserts a new relation into the database with the
// given details.
func (s *stateSuite) addRelationWithLifeAndID(c *tc.C, life corelife.Value, relationID int) corerelation.UUID {
	relationUUID := corerelationtesting.GenRelationUUID(c)
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.Exec(`
INSERT INTO relation (uuid, relation_id, life_id)
SELECT ?, ?, id
FROM life
WHERE value = ?
`, relationUUID, relationID, life)
		return err
	})
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Arrange) Failed to insert relation %s, id %d", relationUUID, relationID))
	return relationUUID
}

// addRelationStatusWithMessage inserts a relation status into the relation_status table.
func (s *stateSuite) addRelationStatusWithMessage(c *tc.C, relationUUID corerelation.UUID, status corestatus.Status,
	message string, since time.Time) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.Exec(`
INSERT INTO relation_status (relation_uuid, relation_status_type_id, suspended_reason, updated_at)
SELECT ?, rst.id, ?, ?
FROM relation_status_type rst
WHERE rst.name = ?
`, relationUUID, message, since, status)
		return err
	})
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Arrange) Failed to insert relation status %s, status %s, message %q",
		relationUUID, status, message))
}

func (s *stateSuite) addRelationToApplication(c *tc.C, appUUID coreapplication.ID, relationUUID corerelation.UUID) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		var charmRelationUUID string
		err := tx.QueryRowContext(ctx, `SELECT uuid FROM charm_relation WHERE name = 'endpoint'`).Scan(&charmRelationUUID)
		if err != nil {
			return err
		}

		var endpointUUID string
		err = tx.QueryRowContext(ctx, `SELECT uuid FROM application_endpoint WHERE application_uuid = ? AND charm_relation_uuid = ?`, appUUID, charmRelationUUID).Scan(&endpointUUID)
		if err != nil {
			return err
		}

		relationEndpointUUID := uuid.MustNewUUID().String()
		_, err = tx.ExecContext(ctx, `INSERT INTO relation_endpoint (uuid, relation_uuid, endpoint_uuid) VALUES (?, ?, ?);`, relationEndpointUUID, relationUUID, endpointUUID)
		if err != nil {
			return err
		}

		_, err = tx.ExecContext(ctx, `INSERT INTO application_exposed_endpoint_cidr (application_uuid, application_endpoint_uuid, cidr) VALUES (?, ?, "10.0.0.0/24") ON CONFLICT DO NOTHING;`, appUUID, endpointUUID)
		if err != nil {
			return err
		}

		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *stateSuite) createApplication(c *tc.C, name string, l life.Life, subordinate bool, appStatus *status.StatusInfo[status.WorkloadStatusType], units ...application.AddUnitArg) (coreapplication.ID, []coreunit.UUID) {
	appState := applicationstate.NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))

	platform := deployment.Platform{
		Channel:      "22.04/stable",
		OSType:       deployment.Ubuntu,
		Architecture: architecture.ARM64,
	}
	channel := &deployment.Channel{
		Track:  "track",
		Risk:   "stable",
		Branch: "branch",
	}
	ctx := c.Context()

	appID, err := appState.CreateCAASApplication(ctx, name, application.AddCAASApplicationArg{
		BaseAddApplicationArg: application.BaseAddApplicationArg{
			Platform: platform,
			Channel:  channel,
			Charm: charm.Charm{
				Metadata: charm.Metadata{
					Name:        name,
					Subordinate: subordinate,
					Provides: map[string]charm.Relation{
						"endpoint": {
							Name:  "endpoint",
							Role:  charm.RoleProvider,
							Scope: charm.ScopeGlobal,
						},
						"misc": {
							Name:  "misc",
							Role:  charm.RoleProvider,
							Scope: charm.ScopeGlobal,
						},
					},
				},
				Manifest:      s.minimalManifest(c),
				ReferenceName: name,
				Source:        charm.CharmHubSource,
				Revision:      42,
				Hash:          "hash",
				Architecture:  architecture.ARM64,
			},
			CharmDownloadInfo: &charm.DownloadInfo{
				Provenance:         charm.ProvenanceDownload,
				CharmhubIdentifier: "ident",
				DownloadURL:        "https://example.com",
				DownloadSize:       42,
			},
			Status: appStatus,
		},
		Scale: len(units),
	}, nil)
	c.Assert(err, tc.ErrorIsNil)

	unitNames, _, err := appState.AddIAASUnits(ctx, appID, units...)
	c.Assert(err, tc.ErrorIsNil)

	var unitUUIDs = make([]coreunit.UUID, len(units))
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "UPDATE application SET life_id = ? WHERE name = ?", l, name)
		if err != nil {
			return err
		}

		_, err = tx.ExecContext(ctx, "UPDATE unit SET life_id = ? WHERE application_uuid = ?", l, appID)
		if err != nil {
			return err
		}

		for i, unitName := range unitNames {
			var uuid coreunit.UUID
			err := tx.QueryRowContext(ctx, "SELECT uuid FROM unit WHERE name = ?", unitName).Scan(&uuid)
			if err != nil {
				return err
			}
			unitUUIDs[i] = uuid
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	return appID, unitUUIDs
}

func (s *stateSuite) setApplicationLXDProfile(c *tc.C, appUUID coreapplication.ID, profile string) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
UPDATE charm SET lxd_profile = ? WHERE uuid = (SELECT charm_uuid FROM application WHERE uuid = ?)
`, profile, appUUID)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *stateSuite) setApplicationSubordinate(c *tc.C, principal coreunit.UUID, subordinate coreunit.UUID) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO unit_principal (unit_uuid, principal_uuid)
VALUES (?, ?);`, subordinate, principal)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *stateSuite) minimalManifest(c *tc.C) charm.Manifest {
	return charm.Manifest{
		Bases: []charm.Base{
			{
				Name: "ubuntu",
				Channel: charm.Channel{
					Risk: charm.RiskStable,
				},
				Architectures: []string{"amd64"},
			},
		},
	}
}

func (s *stateSuite) assertUnitStatus(c *tc.C, statusType, unitUUID coreunit.UUID, statusID int, message string, since *time.Time, data []byte) {
	var (
		gotStatusID int
		gotMessage  string
		gotSince    *time.Time
		gotData     []byte
	)
	queryInfo := fmt.Sprintf(`SELECT status_id, message, data, updated_at FROM %s_status WHERE unit_uuid = ?`, statusType)
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, queryInfo, unitUUID).
			Scan(&gotStatusID, &gotMessage, &gotData, &gotSince); err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(gotStatusID, tc.Equals, statusID)
	c.Check(gotMessage, tc.Equals, message)
	c.Check(gotSince, tc.DeepEquals, since)
	c.Check(gotData, tc.DeepEquals, data)
}

func assertStatusInfoEqual[T status.StatusID](c *tc.C, got, want status.StatusInfo[T]) {
	c.Check(got.Status, tc.Equals, want.Status)
	c.Check(got.Message, tc.Equals, want.Message)
	c.Check(got.Data, tc.DeepEquals, want.Data)
	c.Check(got.Since.Sub(*want.Since), tc.Equals, time.Duration(0))
}

func ptr[T any](v T) *T {
	return &v
}
