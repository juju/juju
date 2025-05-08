// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"time"

	"github.com/canonical/sqlair"
	"github.com/juju/clock"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coreapplication "github.com/juju/juju/core/application"
	corelife "github.com/juju/juju/core/life"
	"github.com/juju/juju/core/model"
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
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/domain/status"
	statuserrors "github.com/juju/juju/domain/status/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
)

type stateSuite struct {
	schematesting.ModelSuite
	modelUUID string

	state *State
}

var _ = gc.Suite(&stateSuite{})

func (s *stateSuite) SetUpTest(c *gc.C) {
	s.ModelSuite.SetUpTest(c)

	modelUUID := uuid.MustNewUUID().String()
	controllerUUID := uuid.MustNewUUID().String()

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO model (uuid, controller_uuid, name, type, cloud, cloud_type, credential_owner)
			VALUES (?, ?, "test", "iaas", "test-model", "ec2", "owner")
		`, modelUUID, controllerUUID)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	s.state = NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))
	s.modelUUID = modelUUID
}

func (s *stateSuite) TestGetModelInfo(c *gc.C) {
	modelInfo, err := s.state.GetModelInfo(context.Background())

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(modelInfo.Type, gc.Equals, model.IAAS.String())
}

func (s *stateSuite) TestGetModelInfoNotFound(c *gc.C) {
	state := NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))

	_, err := state.GetModelInfo(context.Background())
	c.Assert(err, jc.ErrorIsNil)
}

func (s *stateSuite) TestGetAllRelationStatuses(c *gc.C) {
	// Arrange: add two relation, one with a status, but not the second one.
	now := time.Now().Truncate(time.Minute).UTC()

	relationID := 7
	relationUUID := s.addRelationWithLifeAndID(c, corelife.Alive, relationID)
	_ = s.addRelationWithLifeAndID(c, corelife.Alive, 8)

	s.addRelationStatusWithMessage(c, relationUUID, corestatus.Suspended, "this is a test", now)

	// Act
	result, err := s.state.GetAllRelationStatuses(context.Background())

	// Assert:
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, []status.RelationStatusInfo{{
		RelationUUID: relationUUID,
		RelationID:   relationID,
		StatusInfo: status.StatusInfo[status.RelationStatusType]{
			Status:  status.RelationStatusTypeSuspended,
			Message: "this is a test",
			Since:   &now,
		},
	}})
}

func (s *stateSuite) TestGetAllRelationStatusesNone(c *gc.C) {
	// Act
	result, err := s.state.GetAllRelationStatuses(context.Background())

	// Assert:
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.HasLen, 0)
}

func (s *stateSuite) TestGetApplicationIDByName(c *gc.C) {
	id, _ := s.createApplication(c, "foo", life.Alive, false, s.appStatus(time.Now()))

	gotID, err := s.state.GetApplicationIDByName(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(gotID, gc.Equals, id)
}

func (s *stateSuite) TestGetApplicationIDByNameNotFound(c *gc.C) {
	_, err := s.state.GetApplicationIDByName(context.Background(), "foo")
	c.Assert(err, jc.ErrorIs, statuserrors.ApplicationNotFound)
}

func (s *stateSuite) TestGetApplicationIDAndNameByUnitName(c *gc.C) {
	u1 := application.AddUnitArg{}
	expectedAppUUID, _ := s.createApplication(c, "foo", life.Alive, false, s.appStatus(time.Now()), u1)

	appUUID, appName, err := s.state.GetApplicationIDAndNameByUnitName(context.Background(), coreunit.Name("foo/0"))
	c.Assert(err, jc.ErrorIsNil)
	c.Check(appUUID, gc.Equals, expectedAppUUID)
	c.Check(appName, gc.Equals, "foo")
}

func (s *stateSuite) TestGetApplicationIDAndNameByUnitNameNotFound(c *gc.C) {
	_, _, err := s.state.GetApplicationIDAndNameByUnitName(context.Background(), "failme")
	c.Assert(err, jc.ErrorIs, statuserrors.UnitNotFound)
}

func (s *stateSuite) TestSetApplicationStatus(c *gc.C) {
	id, _ := s.createApplication(c, "foo", life.Alive, false, s.appStatus(time.Now()))

	now := time.Now().UTC()
	expected := status.StatusInfo[status.WorkloadStatusType]{
		Status:  status.WorkloadStatusActive,
		Message: "message",
		Data:    []byte("data"),
		Since:   ptr(now),
	}

	err := s.state.SetApplicationStatus(context.Background(), id, expected)
	c.Assert(err, jc.ErrorIsNil)

	status, err := s.state.GetApplicationStatus(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(status, jc.DeepEquals, expected)
}

func (s *stateSuite) TestSetApplicationStatusMultipleTimes(c *gc.C) {
	id, _ := s.createApplication(c, "foo", life.Alive, false, s.appStatus(time.Now()))

	err := s.state.SetApplicationStatus(context.Background(), id, status.StatusInfo[status.WorkloadStatusType]{
		Status:  status.WorkloadStatusBlocked,
		Message: "blocked",
		Since:   ptr(time.Now().UTC()),
	})
	c.Assert(err, jc.ErrorIsNil)

	now := time.Now().UTC()
	expected := status.StatusInfo[status.WorkloadStatusType]{
		Status:  status.WorkloadStatusActive,
		Message: "message",
		Data:    []byte("data"),
		Since:   ptr(now),
	}

	err = s.state.SetApplicationStatus(context.Background(), id, expected)
	c.Assert(err, jc.ErrorIsNil)

	status, err := s.state.GetApplicationStatus(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(status, jc.DeepEquals, expected)
}

func (s *stateSuite) TestSetApplicationStatusWithNoData(c *gc.C) {
	id, _ := s.createApplication(c, "foo", life.Alive, false, s.appStatus(time.Now()))

	now := time.Now().UTC()
	expected := status.StatusInfo[status.WorkloadStatusType]{
		Status:  status.WorkloadStatusActive,
		Message: "message",
		Since:   ptr(now),
	}

	err := s.state.SetApplicationStatus(context.Background(), id, expected)
	c.Assert(err, jc.ErrorIsNil)

	status, err := s.state.GetApplicationStatus(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(status, jc.DeepEquals, expected)
}

func (s *stateSuite) TestSetApplicationStatusApplicationNotFound(c *gc.C) {
	now := time.Now().UTC()
	expected := status.StatusInfo[status.WorkloadStatusType]{
		Status:  status.WorkloadStatusActive,
		Message: "message",
		Data:    []byte("data"),
		Since:   ptr(now),
	}

	err := s.state.SetApplicationStatus(context.Background(), "foo", expected)
	c.Assert(err, jc.ErrorIs, statuserrors.ApplicationNotFound)
}

func (s *stateSuite) TestSetApplicationStatusInvalidStatus(c *gc.C) {
	id, _ := s.createApplication(c, "foo", life.Alive, false, s.appStatus(time.Now()))

	expected := status.StatusInfo[status.WorkloadStatusType]{
		Status: status.WorkloadStatusType(99),
	}

	err := s.state.SetApplicationStatus(context.Background(), id, expected)
	c.Assert(err, gc.ErrorMatches, `unknown status.*`)
}

func (s *stateSuite) TestGetApplicationStatusApplicationNotFound(c *gc.C) {
	_, err := s.state.GetApplicationStatus(context.Background(), "foo")
	c.Assert(err, jc.ErrorIs, statuserrors.ApplicationNotFound)
}

func (s *stateSuite) TestGetApplicationStatusNotSet(c *gc.C) {
	id, _ := s.createApplication(c, "foo", life.Alive, false, nil)

	sts, err := s.state.GetApplicationStatus(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(sts, gc.DeepEquals, status.StatusInfo[status.WorkloadStatusType]{
		Status: status.WorkloadStatusUnset,
	})
}

func (s *stateSuite) TestSetRelationStatus(c *gc.C) {
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
	err := s.state.SetRelationStatus(context.Background(), relationUUID, sts)
	c.Assert(err, jc.ErrorIsNil)

	// Assert:
	foundStatus := s.getRelationStatus(c, relationUUID)
	c.Assert(foundStatus, jc.DeepEquals, sts)
}

// TestSetRelationStatusMultipleTimes sets the status multiple times to ensure
// that it is updated correctly the second time.
func (s *stateSuite) TestSetRelationStatusMultipleTimes(c *gc.C) {
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
	err := s.state.SetRelationStatus(context.Background(), relationUUID, sts1)
	c.Assert(err, jc.ErrorIsNil)

	err = s.state.SetRelationStatus(context.Background(), relationUUID, sts2)
	c.Assert(err, jc.ErrorIsNil)

	// Assert:
	foundStatus := s.getRelationStatus(c, relationUUID)
	c.Assert(foundStatus, jc.DeepEquals, sts2)
}

// TestSetRelationStatusInvalidTransition checks that an invalid relation status
// transition is blocked.
func (s *stateSuite) TestSetRelationStatusInvalidTransition(c *gc.C) {
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
	err := s.state.SetRelationStatus(context.Background(), relationUUID, sts)

	// Assert:
	c.Assert(err, jc.ErrorIs, statuserrors.RelationStatusTransitionNotValid)
}

// TestSetRelationStatusSuspendingToSuspended checks that the message from
// Suspending status is preserved when the status is updated to Suspended.
func (s *stateSuite) TestSetRelationStatusSuspendingToSuspended(c *gc.C) {
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
	err := s.state.SetRelationStatus(context.Background(), relationUUID, suspendedStatus)
	c.Assert(err, jc.ErrorIsNil)

	// Assert:
	foundStatus := s.getRelationStatus(c, relationUUID)
	c.Assert(foundStatus, jc.DeepEquals, status.StatusInfo[status.RelationStatusType]{
		Status:  status.RelationStatusTypeSuspended,
		Message: message,
		Since:   ptr(now),
	})
}

func (s *stateSuite) TestSetRelationStatusRelationNotFound(c *gc.C) {
	// Arrange: Create relation and statuses.
	sts := status.StatusInfo[status.RelationStatusType]{
		Since: ptr(time.Now().UTC()),
	}

	// Act:
	err := s.state.SetRelationStatus(context.Background(), "bad-uuid", sts)

	// Assert:
	c.Assert(err, jc.ErrorIs, statuserrors.RelationNotFound)
}

func (s *stateSuite) TestImportRelationStatus(c *gc.C) {
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
	err := s.state.ImportRelationStatus(context.Background(), relationID, sts)
	c.Assert(err, jc.ErrorIsNil)

	// Assert:
	foundStatus := s.getRelationStatus(c, relationUUID)
	c.Assert(foundStatus, jc.DeepEquals, sts)
}

func (s *stateSuite) TestImportRelationStatusRelationNotFound(c *gc.C) {
	// Arrange: Create relation and statuses.
	sts := status.StatusInfo[status.RelationStatusType]{
		Since: ptr(time.Now().UTC()),
	}

	// Act:
	err := s.state.ImportRelationStatus(context.Background(), 0, sts)

	// Assert:
	c.Assert(err, jc.ErrorIs, statuserrors.RelationNotFound)
}

func (s *stateSuite) getRelationStatus(c *gc.C, relationUUID corerelation.UUID) status.StatusInfo[status.RelationStatusType] {
	var (
		statusType int
		reason     string
		updated_at *time.Time
	)
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRow(`
SELECT relation_status_type_id, suspended_reason, updated_at
FROM   relation_status
WHERE  relation_uuid = ?
`, relationUUID).Scan(&statusType, &reason, &updated_at)
	})
	c.Assert(err, jc.ErrorIsNil)
	encodedStatus, err := status.DecodeRelationStatus(statusType)
	c.Assert(err, jc.ErrorIsNil)
	return status.StatusInfo[status.RelationStatusType]{
		Status:  encodedStatus,
		Message: reason,
		Since:   updated_at,
	}
}

func (s *stateSuite) TestSetK8sPodStatus(c *gc.C) {
	u1 := application.AddUnitArg{}
	_, unitUUIDs := s.createApplication(c, "foo", life.Alive, false, s.appStatus(time.Now()), u1)
	unitUUID := unitUUIDs[0]

	status := status.StatusInfo[status.K8sPodStatusType]{
		Status:  status.K8sPodStatusRunning,
		Message: "it's running",
		Data:    []byte(`{"foo": "bar"}`),
		Since:   ptr(time.Now()),
	}

	err := s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		return s.state.setK8sPodStatus(ctx, tx, unitUUID, status)
	})
	c.Assert(err, jc.ErrorIsNil)
	s.assertUnitStatus(
		c, "k8s_pod", unitUUID, int(status.Status), status.Message, status.Since, status.Data)
}

func (s *stateSuite) TestSetUnitAgentStatus(c *gc.C) {
	u1 := application.AddUnitArg{}
	_, unitUUIDs := s.createApplication(c, "foo", life.Alive, false, s.appStatus(time.Now()), u1)
	unitUUID := unitUUIDs[0]

	status := status.StatusInfo[status.UnitAgentStatusType]{
		Status:  status.UnitAgentStatusExecuting,
		Message: "it's executing",
		Data:    []byte(`{"foo": "bar"}`),
		Since:   ptr(time.Now()),
	}

	err := s.state.SetUnitAgentStatus(context.Background(), unitUUID, status)
	c.Assert(err, jc.ErrorIsNil)
	s.assertUnitStatus(
		c, "unit_agent", unitUUID, int(status.Status), status.Message, status.Since, status.Data)
}

func (s *stateSuite) TestSetUnitAgentStatusNotFound(c *gc.C) {
	status := status.StatusInfo[status.UnitAgentStatusType]{
		Status:  status.UnitAgentStatusExecuting,
		Message: "it's executing",
		Data:    []byte(`{"foo": "bar"}`),
		Since:   ptr(time.Now()),
	}

	unitUUID := unittesting.GenUnitUUID(c)

	err := s.state.SetUnitAgentStatus(context.Background(), unitUUID, status)
	c.Assert(err, jc.ErrorIs, statuserrors.UnitNotFound)
}

func (s *stateSuite) TestGetUnitAgentStatusUnset(c *gc.C) {
	u1 := application.AddUnitArg{}
	_, unitUUIDs := s.createApplication(c, "foo", life.Alive, false, s.appStatus(time.Now()), u1)
	unitUUID := unitUUIDs[0]

	_, err := s.state.GetUnitAgentStatus(context.Background(), unitUUID)
	c.Assert(err, jc.ErrorIs, statuserrors.UnitStatusNotFound)
}

func (s *stateSuite) TestGetUnitAgentStatusDead(c *gc.C) {
	u1 := application.AddUnitArg{}
	_, unitUUIDs := s.createApplication(c, "foo", life.Dead, false, s.appStatus(time.Now()), u1)
	unitUUID := unitUUIDs[0]

	_, err := s.state.GetUnitAgentStatus(context.Background(), unitUUID)
	c.Assert(err, jc.ErrorIs, statuserrors.UnitIsDead)
}

func (s *stateSuite) TestGetUnitAgentStatus(c *gc.C) {
	u1 := application.AddUnitArg{}
	_, unitUUIDs := s.createApplication(c, "foo", life.Alive, false, s.appStatus(time.Now()), u1)
	unitUUID := unitUUIDs[0]

	status := status.StatusInfo[status.UnitAgentStatusType]{
		Status:  status.UnitAgentStatusExecuting,
		Message: "it's executing",
		Data:    []byte(`{"foo": "bar"}`),
		Since:   ptr(time.Now()),
	}

	err := s.state.SetUnitAgentStatus(context.Background(), unitUUID, status)
	c.Assert(err, jc.ErrorIsNil)

	gotStatus, err := s.state.GetUnitAgentStatus(context.Background(), unitUUID)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(gotStatus.Present, jc.IsFalse)
	assertStatusInfoEqual(c, gotStatus.StatusInfo, status)
}

func (s *stateSuite) TestGetUnitAgentStatusPresent(c *gc.C) {
	u1 := application.AddUnitArg{}
	_, unitUUIDs := s.createApplication(c, "foo", life.Alive, false, s.appStatus(time.Now()), u1)
	unitUUID := unitUUIDs[0]

	status := status.StatusInfo[status.UnitAgentStatusType]{
		Status:  status.UnitAgentStatusExecuting,
		Message: "it's executing",
		Data:    []byte(`{"foo": "bar"}`),
		Since:   ptr(time.Now()),
	}

	err := s.state.SetUnitAgentStatus(context.Background(), unitUUID, status)
	c.Assert(err, jc.ErrorIsNil)

	err = s.state.SetUnitPresence(context.Background(), coreunit.Name("foo/0"))
	c.Assert(err, jc.ErrorIsNil)

	gotStatus, err := s.state.GetUnitAgentStatus(context.Background(), unitUUID)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(gotStatus.Present, jc.IsTrue)
	assertStatusInfoEqual(c, gotStatus.StatusInfo, status)

	err = s.state.DeleteUnitPresence(context.Background(), coreunit.Name("foo/0"))
	c.Assert(err, jc.ErrorIsNil)

	gotStatus, err = s.state.GetUnitAgentStatus(context.Background(), unitUUID)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(gotStatus.Present, jc.IsFalse)
	assertStatusInfoEqual(c, gotStatus.StatusInfo, status)
}

func (s *stateSuite) TestGetUnitWorkloadStatusUnitNotFound(c *gc.C) {
	_, err := s.state.GetUnitWorkloadStatus(context.Background(), "missing-uuid")
	c.Assert(err, jc.ErrorIs, statuserrors.UnitNotFound)
}

func (s *stateSuite) TestGetUnitWorkloadStatusDead(c *gc.C) {
	u1 := application.AddUnitArg{}
	_, unitUUIDs := s.createApplication(c, "foo", life.Dead, false, s.appStatus(time.Now()), u1)
	unitUUID := unitUUIDs[0]

	_, err := s.state.GetUnitWorkloadStatus(context.Background(), unitUUID)
	c.Assert(err, jc.ErrorIs, statuserrors.UnitIsDead)
}

func (s *stateSuite) TestGetUnitWorkloadStatusUnsetStatus(c *gc.C) {
	u1 := application.AddUnitArg{}
	_, unitUUIDs := s.createApplication(c, "foo", life.Alive, false, s.appStatus(time.Now()), u1)
	unitUUID := unitUUIDs[0]

	_, err := s.state.GetUnitWorkloadStatus(context.Background(), unitUUID)
	c.Assert(err, jc.ErrorIs, statuserrors.UnitStatusNotFound)
}

func (s *stateSuite) TestSetWorkloadStatus(c *gc.C) {
	u1 := application.AddUnitArg{}
	_, unitUUIDs := s.createApplication(c, "foo", life.Alive, false, s.appStatus(time.Now()), u1)
	unitUUID := unitUUIDs[0]

	sts := status.StatusInfo[status.WorkloadStatusType]{
		Status:  status.WorkloadStatusActive,
		Message: "it's active!",
		Data:    []byte(`{"foo": "bar"}`),
		Since:   ptr(time.Now()),
	}

	err := s.state.SetUnitWorkloadStatus(context.Background(), unitUUID, sts)
	c.Assert(err, jc.ErrorIsNil)

	gotStatus, err := s.state.GetUnitWorkloadStatus(context.Background(), unitUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(gotStatus.Present, jc.IsFalse)
	assertStatusInfoEqual(c, gotStatus.StatusInfo, sts)

	// Run SetUnitWorkloadStatus followed by GetUnitWorkloadStatus to ensure that
	// the new status overwrites the old one.
	sts = status.StatusInfo[status.WorkloadStatusType]{
		Status:  status.WorkloadStatusTerminated,
		Message: "it's terminated",
		Data:    []byte(`{"bar": "foo"}`),
		Since:   ptr(time.Now()),
	}

	err = s.state.SetUnitWorkloadStatus(context.Background(), unitUUID, sts)
	c.Assert(err, jc.ErrorIsNil)

	gotStatus, err = s.state.GetUnitWorkloadStatus(context.Background(), unitUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(gotStatus.Present, jc.IsFalse)
	assertStatusInfoEqual(c, gotStatus.StatusInfo, sts)
}

func (s *stateSuite) TestSetUnitWorkloadStatusToError(c *gc.C) {
	u1 := application.AddUnitArg{}
	_, unitUUIDs := s.createApplication(c, "foo", life.Alive, false, s.appStatus(time.Now()), u1)
	unitUUID := unitUUIDs[0]

	sts := status.StatusInfo[status.WorkloadStatusType]{
		Status:  status.WorkloadStatusError,
		Message: "it's an error!",
		Data:    []byte("some data"),
		Since:   ptr(time.Now()),
	}

	err := s.state.SetUnitWorkloadStatus(context.Background(), unitUUID, sts)
	c.Assert(err, jc.ErrorIsNil)

	gotStatus, err := s.state.GetUnitWorkloadStatus(context.Background(), unitUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(gotStatus.Present, jc.IsFalse)
	assertStatusInfoEqual(c, gotStatus.StatusInfo, sts)
}

func (s *stateSuite) TestSetWorkloadStatusPresent(c *gc.C) {
	u1 := application.AddUnitArg{}
	_, unitUUIDs := s.createApplication(c, "foo", life.Alive, false, s.appStatus(time.Now()), u1)
	unitUUID := unitUUIDs[0]

	sts := status.StatusInfo[status.WorkloadStatusType]{
		Status:  status.WorkloadStatusActive,
		Message: "it's active!",
		Data:    []byte(`{"foo": "bar"}`),
		Since:   ptr(time.Now()),
	}

	err := s.state.SetUnitWorkloadStatus(context.Background(), unitUUID, sts)
	c.Assert(err, jc.ErrorIsNil)

	err = s.state.SetUnitPresence(context.Background(), coreunit.Name("foo/0"))
	c.Assert(err, jc.ErrorIsNil)

	gotStatus, err := s.state.GetUnitWorkloadStatus(context.Background(), unitUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(gotStatus.Present, jc.IsTrue)
	assertStatusInfoEqual(c, gotStatus.StatusInfo, sts)

	// Run SetUnitWorkloadStatus followed by GetUnitWorkloadStatus to ensure that
	// the new status overwrites the old one.
	sts = status.StatusInfo[status.WorkloadStatusType]{
		Status:  status.WorkloadStatusTerminated,
		Message: "it's terminated",
		Data:    []byte(`{"bar": "foo"}`),
		Since:   ptr(time.Now()),
	}

	err = s.state.SetUnitWorkloadStatus(context.Background(), unitUUID, sts)
	c.Assert(err, jc.ErrorIsNil)

	gotStatus, err = s.state.GetUnitWorkloadStatus(context.Background(), unitUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(gotStatus.Present, jc.IsTrue)
	assertStatusInfoEqual(c, gotStatus.StatusInfo, sts)
}

func (s *stateSuite) TestSetUnitWorkloadStatusNotFound(c *gc.C) {
	status := status.StatusInfo[status.WorkloadStatusType]{
		Status:  status.WorkloadStatusTerminated,
		Message: "it's terminated",
		Data:    []byte(`{"foo": "bar"}`),
		Since:   ptr(time.Now()),
	}

	err := s.state.SetUnitWorkloadStatus(context.Background(), "missing-uuid", status)
	c.Assert(err, jc.ErrorIs, statuserrors.UnitNotFound)
}

func (s *stateSuite) TestGetUnitK8sPodStatusUnset(c *gc.C) {
	u1 := application.AddUnitArg{}
	_, unitUUIDs := s.createApplication(c, "foo", life.Alive, false, s.appStatus(time.Now()), u1)
	unitUUID := unitUUIDs[0]

	sts, err := s.state.GetUnitK8sPodStatus(context.Background(), unitUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(sts, gc.DeepEquals, status.StatusInfo[status.K8sPodStatusType]{
		Status: status.K8sPodStatusUnset,
	})
}

func (s *stateSuite) TestGetUnitK8sPodStatusUnitNotFound(c *gc.C) {
	_, err := s.state.GetUnitK8sPodStatus(context.Background(), "missing-uuid")
	c.Assert(err, jc.ErrorIs, statuserrors.UnitNotFound)
}

func (s *stateSuite) TestGetUnitK8sPodStatusDead(c *gc.C) {
	u1 := application.AddUnitArg{}
	_, unitUUIDs := s.createApplication(c, "foo", life.Dead, false, s.appStatus(time.Now()), u1)
	unitUUID := unitUUIDs[0]

	_, err := s.state.GetUnitK8sPodStatus(context.Background(), unitUUID)
	c.Assert(err, jc.ErrorIs, statuserrors.UnitIsDead)
}

func (s *stateSuite) TestGetUnitK8sPodStatus(c *gc.C) {
	u1 := application.AddUnitArg{}
	_, unitUUIDs := s.createApplication(c, "foo", life.Alive, false, s.appStatus(time.Now()), u1)
	unitUUID := unitUUIDs[0]

	now := time.Now()

	s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		return s.state.setK8sPodStatus(ctx, tx, unitUUID, status.StatusInfo[status.K8sPodStatusType]{
			Status:  status.K8sPodStatusRunning,
			Message: "it's running",
			Data:    []byte(`{"foo": "bar"}`),
			Since:   &now,
		})
	})

	sts, err := s.state.GetUnitK8sPodStatus(context.Background(), unitUUID)
	c.Assert(err, jc.ErrorIsNil)
	assertStatusInfoEqual(c, sts, status.StatusInfo[status.K8sPodStatusType]{
		Status:  status.K8sPodStatusRunning,
		Message: "it's running",
		Data:    []byte(`{"foo": "bar"}`),
		Since:   &now,
	})
}

func (s *stateSuite) TestGetUnitWorkloadStatusesForApplication(c *gc.C) {
	u1 := application.AddUnitArg{}
	appId, unitUUIDs := s.createApplication(c, "foo", life.Alive, false, s.appStatus(time.Now()), u1)
	unitUUID := unitUUIDs[0]

	status := status.StatusInfo[status.WorkloadStatusType]{
		Status:  status.WorkloadStatusActive,
		Message: "it's active!",
		Data:    []byte(`{"foo": "bar"}`),
		Since:   ptr(time.Now()),
	}
	err := s.state.SetUnitWorkloadStatus(context.Background(), unitUUID, status)
	c.Assert(err, jc.ErrorIsNil)

	results, err := s.state.GetUnitWorkloadStatusesForApplication(context.Background(), appId)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(results, gc.HasLen, 1)
	result, ok := results["foo/0"]
	c.Assert(ok, jc.IsTrue)
	c.Check(result.Present, jc.IsFalse)
	assertStatusInfoEqual(c, result.StatusInfo, status)
}

func (s *stateSuite) TestGetUnitWorkloadStatusesForApplicationMultipleUnits(c *gc.C) {
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
	err := s.state.SetUnitWorkloadStatus(context.Background(), unitUUID1, status1)
	c.Assert(err, jc.ErrorIsNil)

	status2 := status.StatusInfo[status.WorkloadStatusType]{
		Status:  status.WorkloadStatusTerminated,
		Message: "it's terminated",
		Data:    []byte(`{"bar": "foo"}`),
		Since:   ptr(time.Now()),
	}
	err = s.state.SetUnitWorkloadStatus(context.Background(), unitUUID2, status2)
	c.Assert(err, jc.ErrorIsNil)

	results, err := s.state.GetUnitWorkloadStatusesForApplication(context.Background(), appId)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, 2, gc.Commentf("expected 2, got %d", len(results)))

	result1, ok := results["foo/0"]
	c.Assert(ok, jc.IsTrue)
	c.Check(result1.Present, jc.IsFalse)
	assertStatusInfoEqual(c, result1.StatusInfo, status1)

	result2, ok := results["foo/1"]
	c.Assert(ok, jc.IsTrue)
	c.Check(result2.Present, jc.IsFalse)
	assertStatusInfoEqual(c, result2.StatusInfo, status2)
}

func (s *stateSuite) TestGetUnitWorkloadStatusesForApplicationMultipleUnitsPresent(c *gc.C) {
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
	err := s.state.SetUnitWorkloadStatus(context.Background(), unitUUID1, status1)
	c.Assert(err, jc.ErrorIsNil)

	status2 := status.StatusInfo[status.WorkloadStatusType]{
		Status:  status.WorkloadStatusTerminated,
		Message: "it's terminated",
		Data:    []byte(`{"bar": "foo"}`),
		Since:   ptr(time.Now()),
	}
	err = s.state.SetUnitWorkloadStatus(context.Background(), unitUUID2, status2)
	c.Assert(err, jc.ErrorIsNil)
	err = s.state.SetUnitPresence(context.Background(), coreunit.Name("foo/1"))
	c.Assert(err, jc.ErrorIsNil)

	results, err := s.state.GetUnitWorkloadStatusesForApplication(context.Background(), appId)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, 2, gc.Commentf("expected 2, got %d", len(results)))

	result1, ok := results["foo/0"]
	c.Assert(ok, jc.IsTrue)
	c.Check(result1.Present, jc.IsFalse)
	assertStatusInfoEqual(c, result1.StatusInfo, status1)

	result2, ok := results["foo/1"]
	c.Assert(ok, jc.IsTrue)
	c.Check(result2.Present, jc.IsTrue)
	assertStatusInfoEqual(c, result2.StatusInfo, status2)
}

func (s *stateSuite) TestGetUnitWorkloadStatusesForApplicationNotFound(c *gc.C) {
	_, err := s.state.GetUnitWorkloadStatusesForApplication(context.Background(), "missing")
	c.Assert(err, jc.ErrorIs, statuserrors.ApplicationNotFound)
}

func (s *stateSuite) TestGetUnitWorkloadStatusesForApplicationNoUnits(c *gc.C) {
	appId, _ := s.createApplication(c, "foo", life.Alive, false, s.appStatus(time.Now()))

	results, err := s.state.GetUnitWorkloadStatusesForApplication(context.Background(), appId)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, 0)
}

func (s *stateSuite) TestGetAllUnitStatusesForApplication(c *gc.C) {
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
	err := s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
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
	c.Assert(err, jc.ErrorIsNil)

	fullStatuses, err := s.state.GetAllFullUnitStatusesForApplication(context.Background(), appId)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(fullStatuses, gc.HasLen, 1)
	fullStatus, ok := fullStatuses["foo/0"]
	c.Assert(ok, jc.IsTrue)

	assertStatusInfoEqual(c, fullStatus.WorkloadStatus, workloadStatus)
	assertStatusInfoEqual(c, fullStatus.AgentStatus, agentStatus)
	assertStatusInfoEqual(c, fullStatus.K8sPodStatus, k8sPodStatus)
}

func (s *stateSuite) TestGetUnitK8sPodStatusForApplicationMultipleUnits(c *gc.C) {
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
	err := s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
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
	c.Assert(err, jc.ErrorIsNil)

	status2 := status.StatusInfo[status.K8sPodStatusType]{
		Status:  status.K8sPodStatusBlocked,
		Message: "it's blocked",
		Data:    []byte(`{"bar": "foo"}`),
		Since:   ptr(time.Now()),
	}
	err = s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
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
	c.Assert(err, jc.ErrorIsNil)

	fullStatuses, err := s.state.GetAllFullUnitStatusesForApplication(context.Background(), appId)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(fullStatuses, gc.HasLen, 2)
	result1, ok := fullStatuses["foo/0"]
	c.Assert(ok, jc.IsTrue)
	assertStatusInfoEqual(c, result1.K8sPodStatus, status1)

	result2, ok := fullStatuses["foo/1"]
	c.Assert(ok, jc.IsTrue)
	assertStatusInfoEqual(c, result2.K8sPodStatus, status2)
}

func (s *stateSuite) TestGetAllUnitStatusesForApplicationNotFound(c *gc.C) {
	_, err := s.state.GetAllFullUnitStatusesForApplication(context.Background(), "missing")
	c.Assert(err, jc.ErrorIs, statuserrors.ApplicationNotFound)
}

func (s *stateSuite) TestGetAllUnitStatusesForApplicationNoUnits(c *gc.C) {
	appId, _ := s.createApplication(c, "foo", life.Alive, false, s.appStatus(time.Now()))

	fullStatuses, err := s.state.GetAllFullUnitStatusesForApplication(context.Background(), appId)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(fullStatuses, gc.HasLen, 0)
}

func (s *stateSuite) TestGetAllUnitStatusesForApplicationUnitsWithoutStatuses(c *gc.C) {
	u1 := application.AddUnitArg{}
	u2 := application.AddUnitArg{}
	appId, _ := s.createApplication(c, "foo", life.Alive, false, s.appStatus(time.Now()), u1, u2)

	_, err := s.state.GetAllFullUnitStatusesForApplication(context.Background(), appId)
	c.Assert(err, jc.ErrorIs, statuserrors.UnitStatusNotFound)
}

func (s *stateSuite) TestGetAllFullUnitStatusesEmptyModel(c *gc.C) {
	res, err := s.state.GetAllUnitWorkloadAgentStatuses(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(res, gc.HasLen, 0)
}

func (s *stateSuite) TestGetAllFullUnitStatusesNotFound(c *gc.C) {
	u1 := application.AddUnitArg{}
	s.createApplication(c, "foo", life.Alive, false, s.appStatus(time.Now()), u1)

	_, err := s.state.GetAllUnitWorkloadAgentStatuses(context.Background())
	c.Assert(err, jc.ErrorIs, statuserrors.UnitStatusNotFound)
}

func (s *stateSuite) TestGetAllFullUnitStatuses(c *gc.C) {
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
	err := s.state.SetUnitWorkloadStatus(context.Background(), u1UUID, u1Workload)
	c.Assert(err, jc.ErrorIsNil)

	u1Agent := status.StatusInfo[status.UnitAgentStatusType]{
		Status:  status.UnitAgentStatusIdle,
		Message: "u1 is idle!",
		Data:    []byte(`{"u1": "agent"}`),
		Since:   ptr(time.Now()),
	}
	err = s.state.SetUnitAgentStatus(context.Background(), u1UUID, u1Agent)
	c.Assert(err, jc.ErrorIsNil)

	err = s.state.SetUnitPresence(context.Background(), "foo/0")
	c.Assert(err, jc.ErrorIsNil)

	u2Workload := status.StatusInfo[status.WorkloadStatusType]{
		Status:  status.WorkloadStatusBlocked,
		Message: "u2 is blocked!",
		Data:    []byte(`{"u2": "workload"}`),
		Since:   ptr(time.Now()),
	}
	err = s.state.SetUnitWorkloadStatus(context.Background(), u2UUID, u2Workload)
	c.Assert(err, jc.ErrorIsNil)

	u2Agent := status.StatusInfo[status.UnitAgentStatusType]{
		Status:  status.UnitAgentStatusAllocating,
		Message: "u2 is allocating!",
		Data:    []byte(`{"u2": "agent"}`),
		Since:   ptr(time.Now()),
	}
	err = s.state.SetUnitAgentStatus(context.Background(), u2UUID, u2Agent)
	c.Assert(err, jc.ErrorIsNil)

	err = s.state.SetUnitPresence(context.Background(), "foo/1")
	c.Assert(err, jc.ErrorIsNil)
	err = s.state.DeleteUnitPresence(context.Background(), "foo/1")
	c.Assert(err, jc.ErrorIsNil)

	u3Workload := status.StatusInfo[status.WorkloadStatusType]{
		Status:  status.WorkloadStatusMaintenance,
		Message: "u3 is maintenance!",
		Data:    []byte(`{"u3": "workload"}`),
		Since:   ptr(time.Now()),
	}
	err = s.state.SetUnitWorkloadStatus(context.Background(), u3UUID, u3Workload)
	c.Assert(err, jc.ErrorIsNil)

	u3Agent := status.StatusInfo[status.UnitAgentStatusType]{
		Status:  status.UnitAgentStatusRebooting,
		Message: "u3 is rebooting!",
		Data:    []byte(`{"u3": "agent"}`),
		Since:   ptr(time.Now()),
	}
	err = s.state.SetUnitAgentStatus(context.Background(), u3UUID, u3Agent)
	c.Assert(err, jc.ErrorIsNil)

	res, err := s.state.GetAllUnitWorkloadAgentStatuses(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(res, gc.HasLen, 3)

	u1Full, ok := res["foo/0"]
	c.Assert(ok, jc.IsTrue)
	c.Check(u1Full.WorkloadStatus.Status, gc.Equals, status.WorkloadStatusActive)
	c.Check(u1Full.WorkloadStatus.Message, gc.Equals, "u1 is active!")
	c.Check(u1Full.WorkloadStatus.Data, gc.DeepEquals, []byte(`{"u1": "workload"}`))
	c.Check(u1Full.AgentStatus.Status, gc.Equals, status.UnitAgentStatusIdle)
	c.Check(u1Full.AgentStatus.Message, gc.Equals, "u1 is idle!")
	c.Check(u1Full.AgentStatus.Data, gc.DeepEquals, []byte(`{"u1": "agent"}`))
	c.Check(u1Full.Present, gc.Equals, true)

	u2Full, ok := res["foo/1"]
	c.Assert(ok, jc.IsTrue)
	c.Check(u2Full.WorkloadStatus.Status, gc.Equals, status.WorkloadStatusBlocked)
	c.Check(u2Full.WorkloadStatus.Message, gc.Equals, "u2 is blocked!")
	c.Check(u2Full.WorkloadStatus.Data, gc.DeepEquals, []byte(`{"u2": "workload"}`))
	c.Check(u2Full.AgentStatus.Status, gc.Equals, status.UnitAgentStatusAllocating)
	c.Check(u2Full.AgentStatus.Message, gc.Equals, "u2 is allocating!")
	c.Check(u2Full.AgentStatus.Data, gc.DeepEquals, []byte(`{"u2": "agent"}`))
	c.Check(u2Full.Present, gc.Equals, false)

	u3Full, ok := res["bar/0"]
	c.Assert(ok, jc.IsTrue)
	c.Check(u3Full.WorkloadStatus.Status, gc.Equals, status.WorkloadStatusMaintenance)
	c.Check(u3Full.WorkloadStatus.Message, gc.Equals, "u3 is maintenance!")
	c.Check(u3Full.WorkloadStatus.Data, gc.DeepEquals, []byte(`{"u3": "workload"}`))
	c.Check(u3Full.AgentStatus.Status, gc.Equals, status.UnitAgentStatusRebooting)
	c.Check(u3Full.AgentStatus.Message, gc.Equals, "u3 is rebooting!")
	c.Check(u3Full.AgentStatus.Data, gc.DeepEquals, []byte(`{"u3": "agent"}`))
	c.Check(u3Full.Present, gc.Equals, false)
}

func (s *stateSuite) TestGetAllApplicationStatusesEmptyModel(c *gc.C) {
	statuses, err := s.state.GetAllApplicationStatuses(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(statuses, gc.HasLen, 0)
}

func (s *stateSuite) TestGetAllApplicationStatusesUnsetStatuses(c *gc.C) {
	u1 := application.AddUnitArg{}
	s.createApplication(c, "foo", life.Alive, false, nil, u1)
	s.createApplication(c, "bar", life.Alive, false, nil)

	statuses, err := s.state.GetAllApplicationStatuses(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(statuses, gc.HasLen, 0)
}

func (s *stateSuite) TestGetAllApplicationStatuses(c *gc.C) {
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
	s.state.SetApplicationStatus(context.Background(), app1ID, app1Status)
	s.state.SetApplicationStatus(context.Background(), app2ID, app2Status)

	statuses, err := s.state.GetAllApplicationStatuses(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	res1, ok := statuses["foo"]
	c.Assert(ok, jc.IsTrue)
	assertStatusInfoEqual(c, res1, app1Status)

	res2, ok := statuses["bar"]
	c.Assert(ok, jc.IsTrue)
	assertStatusInfoEqual(c, res2, app2Status)
}

func (s *stateSuite) TestSetUnitPresence(c *gc.C) {
	u1 := application.AddUnitArg{}
	u2 := application.AddUnitArg{}
	s.createApplication(c, "foo", life.Alive, false, s.appStatus(time.Now()), u1, u2)

	err := s.state.SetUnitPresence(context.Background(), "foo/0")
	c.Assert(err, jc.ErrorIsNil)

	var lastSeen time.Time
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, "SELECT last_seen FROM v_unit_agent_presence WHERE name=?", "foo/0").Scan(&lastSeen); err != nil {
			return err
		}
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(lastSeen.IsZero(), jc.IsFalse)
	c.Check(lastSeen.After(time.Now().Add(-time.Minute)), jc.IsTrue)
}

func (s *stateSuite) TestSetUnitPresenceNotFound(c *gc.C) {
	err := s.state.SetUnitPresence(context.Background(), "foo/665")
	c.Assert(err, jc.ErrorIs, statuserrors.UnitNotFound)
}

func (s *stateSuite) TestDeleteUnitPresenceNotFound(c *gc.C) {
	err := s.state.DeleteUnitPresence(context.Background(), "foo/665")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *stateSuite) TestDeleteUnitPresence(c *gc.C) {
	u1 := application.AddUnitArg{}
	u2 := application.AddUnitArg{}
	s.createApplication(c, "foo", life.Alive, false, s.appStatus(time.Now()), u1, u2)

	err := s.state.SetUnitPresence(context.Background(), "foo/0")
	c.Assert(err, jc.ErrorIsNil)

	var lastSeen time.Time
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, "SELECT last_seen FROM v_unit_agent_presence WHERE name=?", "foo/0").Scan(&lastSeen); err != nil {
			return err
		}
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(lastSeen.IsZero(), jc.IsFalse)
	c.Check(lastSeen.After(time.Now().Add(-time.Minute)), jc.IsTrue)

	err = s.state.DeleteUnitPresence(context.Background(), "foo/0")
	c.Assert(err, jc.ErrorIsNil)

	var count int
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM v_unit_agent_presence WHERE name=?", "foo/0").Scan(&count); err != nil {
			return err
		}
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(count, gc.Equals, 0)
}

func (s *stateSuite) TestGetApplicationAndUnitStatusesNoApplications(c *gc.C) {
	statuses, err := s.state.GetApplicationAndUnitStatuses(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(statuses, jc.DeepEquals, map[string]status.Application{})
}

func (s *stateSuite) TestGetApplicationAndUnitStatusesNoAppStatuses(c *gc.C) {
	u1 := application.AddUnitArg{}
	u2 := application.AddUnitArg{}
	appUUID, _ := s.createApplication(c, "foo", life.Alive, false, nil, u1, u2)

	statuses, err := s.state.GetApplicationAndUnitStatuses(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(statuses, jc.DeepEquals, map[string]status.Application{
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

func (s *stateSuite) TestGetApplicationAndUnitStatuses(c *gc.C) {
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

	statuses, err := s.state.GetApplicationAndUnitStatuses(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(statuses, jc.DeepEquals, map[string]status.Application{
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

func (s *stateSuite) TestGetApplicationAndUnitStatusesSubordinate(c *gc.C) {
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
	c.Assert(units0, gc.HasLen, 1)

	appUUID1, units1 := s.createApplication(c, "sub", life.Alive, true, appStatus, u2, u3)
	c.Assert(units1, gc.HasLen, 2)
	for _, unit := range units1 {
		s.setApplicationSubordinate(c, units0[0], unit)
	}

	statuses, err := s.state.GetApplicationAndUnitStatuses(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(statuses, jc.DeepEquals, map[string]status.Application{
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

func (s *stateSuite) TestGetApplicationAndUnitStatusesLXDProfile(c *gc.C) {
	u1 := application.AddUnitArg{}
	u2 := application.AddUnitArg{}
	now := time.Now()

	appStatus := s.appStatus(now)
	appUUID, _ := s.createApplication(c, "foo", life.Alive, false, appStatus, u1, u2)
	s.setApplicationLXDProfile(c, appUUID, "{}")

	statuses, err := s.state.GetApplicationAndUnitStatuses(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(statuses, jc.DeepEquals, map[string]status.Application{
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

func (s *stateSuite) TestGetApplicationAndUnitStatusesWorkloadVersion(c *gc.C) {
	u1 := application.AddUnitArg{}
	u2 := application.AddUnitArg{}
	now := time.Now()

	appStatus := s.appStatus(now)
	appUUID, unitUUDs := s.createApplication(c, "foo", life.Alive, false, appStatus, u1, u2)
	c.Assert(unitUUDs, gc.HasLen, 2)
	s.setWorkloadVersion(c, appUUID, unitUUDs[0], "blah")

	statuses, err := s.state.GetApplicationAndUnitStatuses(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(statuses, jc.DeepEquals, map[string]status.Application{
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

func (s *stateSuite) setWorkloadVersion(c *gc.C, appUUID coreapplication.ID, unitUUID coreunit.UUID, version string) {
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `UPDATE application_workload_version SET version=? WHERE application_uuid=?`, version, appUUID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE unit_workload_version SET version=? WHERE unit_uuid=?`, version, unitUUID); err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *stateSuite) TestGetApplicationAndUnitStatusesWithRelations(c *gc.C) {
	u1 := application.AddUnitArg{}
	u2 := application.AddUnitArg{}
	now := time.Now()

	appStatus := s.appStatus(now)
	appUUID, _ := s.createApplication(c, "foo", life.Alive, false, appStatus, u1, u2)

	relationUUID := s.addRelationWithLifeAndID(c, corelife.Alive, 7)
	s.addRelationStatusWithMessage(c, relationUUID, corestatus.Active, "this is a test", now)
	s.addRelationToApplication(c, appUUID, relationUUID)

	statuses, err := s.state.GetApplicationAndUnitStatuses(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(statuses, jc.DeepEquals, map[string]status.Application{
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

func (s *stateSuite) TestGetApplicationAndUnitStatusesWithMultipleRelations(c *gc.C) {
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

	statuses, err := s.state.GetApplicationAndUnitStatuses(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(statuses, jc.DeepEquals, map[string]status.Application{
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

func (s *stateSuite) TestGetApplicationAndUnitModelStatuses(c *gc.C) {
	u1 := application.AddUnitArg{}
	u2 := application.AddUnitArg{}
	now := time.Now()

	appStatus := s.appStatus(now)
	s.createApplication(c, "foo", life.Alive, false, appStatus, u1, u2)

	appUnitCount, err := s.state.GetApplicationAndUnitModelStatuses(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	c.Check(appUnitCount, gc.DeepEquals, map[string]int{
		"foo": 2,
	})
}

func (s *stateSuite) TestGetApplicationAndUnitModelStatusesMultiple(c *gc.C) {
	u1 := application.AddUnitArg{}
	u2 := application.AddUnitArg{}
	u3 := application.AddUnitArg{}
	now := time.Now()

	appStatus := s.appStatus(now)
	s.createApplication(c, "foo", life.Alive, false, appStatus, u1, u2)
	s.createApplication(c, "bar", life.Alive, false, appStatus, u3)

	appUnitCount, err := s.state.GetApplicationAndUnitModelStatuses(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	c.Check(appUnitCount, gc.DeepEquals, map[string]int{
		"foo": 2,
		"bar": 1,
	})
}

func (s *stateSuite) TestGetApplicationAndUnitModelStatusesNoApplication(c *gc.C) {
	appUnitCount, err := s.state.GetApplicationAndUnitModelStatuses(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	c.Check(appUnitCount, gc.DeepEquals, map[string]int{})
}

func (s *stateSuite) TestGetApplicationAndUnitModelStatusesNoUnits(c *gc.C) {
	now := time.Now()
	appStatus := s.appStatus(now)
	s.createApplication(c, "foo", life.Alive, false, appStatus)

	appUnitCount, err := s.state.GetApplicationAndUnitModelStatuses(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	c.Check(appUnitCount, gc.DeepEquals, map[string]int{
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
func (s *stateSuite) addRelationWithLifeAndID(c *gc.C, life corelife.Value, relationID int) corerelation.UUID {
	relationUUID := corerelationtesting.GenRelationUUID(c)
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.Exec(`
INSERT INTO relation (uuid, relation_id, life_id)
SELECT ?, ?, id
FROM life
WHERE value = ?
`, relationUUID, relationID, life)
		return err
	})
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Arrange) Failed to insert relation %s, id %d", relationUUID, relationID))
	return relationUUID
}

// addRelationStatusWithMessage inserts a relation status into the relation_status table.
func (s *stateSuite) addRelationStatusWithMessage(c *gc.C, relationUUID corerelation.UUID, status corestatus.Status,
	message string, since time.Time) {
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.Exec(`
INSERT INTO relation_status (relation_uuid, relation_status_type_id, suspended_reason, updated_at)
SELECT ?, rst.id, ?, ?
FROM relation_status_type rst
WHERE rst.name = ?
`, relationUUID, message, since, status)
		return err
	})
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Arrange) Failed to insert relation status %s, status %s, message %q",
		relationUUID, status, message))
}

func (s *stateSuite) addRelationToApplication(c *gc.C, appUUID coreapplication.ID, relationUUID corerelation.UUID) {
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		var charmRelationUUID string
		err := tx.QueryRowContext(ctx, `SELECT uuid FROM charm_relation WHERE name = 'endpoint'`).Scan(&charmRelationUUID)
		if err != nil {
			return err
		}

		endpointUUID := uuid.MustNewUUID().String()
		_, err = tx.ExecContext(ctx, `INSERT INTO application_endpoint (uuid, application_uuid, charm_relation_uuid) VALUES (?, ?, ?);`, endpointUUID, appUUID, charmRelationUUID)
		if err != nil {
			return err
		}

		relationEndpointUUID := uuid.MustNewUUID().String()
		_, err = tx.ExecContext(ctx, `INSERT INTO relation_endpoint (uuid, relation_uuid, endpoint_uuid) VALUES (?, ?, ?);`, relationEndpointUUID, relationUUID, endpointUUID)
		if err != nil {
			return err
		}

		_, err = tx.ExecContext(ctx, `INSERT INTO application_exposed_endpoint_cidr (application_uuid, application_endpoint_uuid, cidr) VALUES (?, ?, "10.0.0.0/24");`, appUUID, endpointUUID)
		if err != nil {
			return err
		}

		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *stateSuite) createApplication(c *gc.C, name string, l life.Life, subordinate bool, appStatus *status.StatusInfo[status.WorkloadStatusType], units ...application.AddUnitArg) (coreapplication.ID, []coreunit.UUID) {
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
	ctx := context.Background()

	appID, err := appState.CreateApplication(ctx, name, application.AddApplicationArg{
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
		Scale:  len(units),
		Status: appStatus,
	}, nil)
	c.Assert(err, jc.ErrorIsNil)

	unitNames, err := appState.AddIAASUnits(ctx, appID, units...)
	c.Assert(err, jc.ErrorIsNil)

	var unitUUIDs = make([]coreunit.UUID, len(units))
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
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
	c.Assert(err, jc.ErrorIsNil)

	return appID, unitUUIDs
}

func (s *stateSuite) setApplicationLXDProfile(c *gc.C, appUUID coreapplication.ID, profile string) {
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
UPDATE charm SET lxd_profile = ? WHERE uuid = (SELECT charm_uuid FROM application WHERE uuid = ?)
`, profile, appUUID)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *stateSuite) setApplicationSubordinate(c *gc.C, principal coreunit.UUID, subordinate coreunit.UUID) {
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO unit_principal (unit_uuid, principal_uuid)
VALUES (?, ?);`, subordinate, principal)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *stateSuite) minimalManifest(c *gc.C) charm.Manifest {
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

func (s *stateSuite) assertUnitStatus(c *gc.C, statusType, unitUUID coreunit.UUID, statusID int, message string, since *time.Time, data []byte) {
	var (
		gotStatusID int
		gotMessage  string
		gotSince    *time.Time
		gotData     []byte
	)
	queryInfo := fmt.Sprintf(`SELECT status_id, message, data, updated_at FROM %s_status WHERE unit_uuid = ?`, statusType)
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, queryInfo, unitUUID).
			Scan(&gotStatusID, &gotMessage, &gotData, &gotSince); err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(gotStatusID, gc.Equals, statusID)
	c.Check(gotMessage, gc.Equals, message)
	c.Check(gotSince, jc.DeepEquals, since)
	c.Check(gotData, jc.DeepEquals, data)
}

func assertStatusInfoEqual[T status.StatusID](c *gc.C, got, want status.StatusInfo[T]) {
	c.Check(got.Status, gc.Equals, want.Status)
	c.Check(got.Message, gc.Equals, want.Message)
	c.Check(got.Data, jc.DeepEquals, want.Data)
	c.Check(got.Since.Sub(*want.Since), gc.Equals, time.Duration(0))
}

func ptr[T any](v T) *T {
	return &v
}
