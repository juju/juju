// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/canonical/sqlair"
	"github.com/juju/clock"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coreapplication "github.com/juju/juju/core/application"
	corelife "github.com/juju/juju/core/life"
	corerelation "github.com/juju/juju/core/relation"
	corerelationtesting "github.com/juju/juju/core/relation/testing"
	corestatus "github.com/juju/juju/core/status"
	coreunit "github.com/juju/juju/core/unit"
	unittesting "github.com/juju/juju/core/unit/testing"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/architecture"
	"github.com/juju/juju/domain/application/charm"
	applicationstate "github.com/juju/juju/domain/application/state"
	"github.com/juju/juju/domain/life"
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/domain/status"
	statuserrors "github.com/juju/juju/domain/status/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/uuid"
)

type stateSuite struct {
	schematesting.ModelSuite

	state *State
}

var _ = gc.Suite(&stateSuite{})

func (s *stateSuite) SetUpTest(c *gc.C) {
	s.ModelSuite.SetUpTest(c)

	modelUUID := uuid.MustNewUUID()
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO model (uuid, controller_uuid, name, type, cloud, cloud_type)
			VALUES (?, ?, "test", "iaas", "test-model", "ec2")
		`, modelUUID.String(), coretesting.ControllerTag.Id())
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	s.state = NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))
}

func (s *stateSuite) TestGetAllRelationStatuses(c *gc.C) {
	// Arrange: add two relation, one with a status, but not the second one.
	relationUUID1 := corerelationtesting.GenRelationUUID(c)
	relationUUID2 := corerelationtesting.GenRelationUUID(c)
	now := time.Now().Truncate(time.Minute).UTC()

	s.addRelationWithLifeAndID(c, relationUUID1.String(), corelife.Alive, 7)
	s.addRelationWithLifeAndID(c, relationUUID2.String(), corelife.Alive, 8)

	s.addRelationStatusWithMessage(c, relationUUID1.String(), corestatus.Suspended, "this is a test", now)

	// Act
	result, err := s.state.GetAllRelationStatuses(context.Background())

	// Assert:
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, map[corerelation.UUID]status.StatusInfo[status.RelationStatusType]{
		relationUUID1: {
			Status:  status.RelationStatusTypeSuspended,
			Message: "this is a test",
			Since:   &now,
		},
	})

}

func (s *stateSuite) TestGetAllRelationStatusesNone(c *gc.C) {
	// Act
	result, err := s.state.GetAllRelationStatuses(context.Background())

	// Assert:
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.HasLen, 0)
}

func (s *stateSuite) TestGetApplicationIDByName(c *gc.C) {
	id, _ := s.createApplication(c, "foo", life.Alive)

	gotID, err := s.state.GetApplicationIDByName(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(gotID, gc.Equals, id)
}

func (s *stateSuite) TestGetApplicationIDByNameNotFound(c *gc.C) {
	_, err := s.state.GetApplicationIDByName(context.Background(), "foo")
	c.Assert(err, jc.ErrorIs, statuserrors.ApplicationNotFound)
}

func (s *stateSuite) TestGetApplicationIDAndNameByUnitName(c *gc.C) {
	u1 := application.AddUnitArg{
		UnitName: "foo/666",
	}
	expectedAppUUID, _ := s.createApplication(c, "foo", life.Alive, u1)

	appUUID, appName, err := s.state.GetApplicationIDAndNameByUnitName(context.Background(), u1.UnitName)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(appUUID, gc.Equals, expectedAppUUID)
	c.Check(appName, gc.Equals, "foo")
}

func (s *stateSuite) TestGetApplicationIDAndNameByUnitNameNotFound(c *gc.C) {
	_, _, err := s.state.GetApplicationIDAndNameByUnitName(context.Background(), "failme")
	c.Assert(err, jc.ErrorIs, statuserrors.UnitNotFound)
}

func (s *stateSuite) TestSetApplicationStatus(c *gc.C) {
	id, _ := s.createApplication(c, "foo", life.Alive)

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
	id, _ := s.createApplication(c, "foo", life.Alive)

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
	id, _ := s.createApplication(c, "foo", life.Alive)

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
	id, _ := s.createApplication(c, "foo", life.Alive)

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
	id, _ := s.createApplication(c, "foo", life.Alive)

	sts, err := s.state.GetApplicationStatus(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(sts, gc.DeepEquals, status.StatusInfo[status.WorkloadStatusType]{
		Status: status.WorkloadStatusUnset,
	})
}

func (s *stateSuite) TestSetCloudContainerStatus(c *gc.C) {
	u1 := application.AddUnitArg{
		UnitName: "foo/666",
	}
	_, unitUUIDs := s.createApplication(c, "foo", life.Alive, u1)
	unitUUID := unitUUIDs[0]

	status := status.StatusInfo[status.CloudContainerStatusType]{
		Status:  status.CloudContainerStatusRunning,
		Message: "it's running",
		Data:    []byte(`{"foo": "bar"}`),
		Since:   ptr(time.Now()),
	}

	err := s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		return s.state.setCloudContainerStatus(ctx, tx, unitUUID, status)
	})
	c.Assert(err, jc.ErrorIsNil)
	s.assertUnitStatus(
		c, "k8s_pod", unitUUID, int(status.Status), status.Message, status.Since, status.Data)
}

func (s *stateSuite) TestSetUnitAgentStatus(c *gc.C) {
	u1 := application.AddUnitArg{
		UnitName: "foo/666",
	}
	_, unitUUIDs := s.createApplication(c, "foo", life.Alive, u1)
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
	u1 := application.AddUnitArg{
		UnitName: "foo/666",
	}
	_, unitUUIDs := s.createApplication(c, "foo", life.Alive, u1)
	unitUUID := unitUUIDs[0]

	_, err := s.state.GetUnitAgentStatus(context.Background(), unitUUID)
	c.Assert(err, jc.ErrorIs, statuserrors.UnitStatusNotFound)
}

func (s *stateSuite) TestGetUnitAgentStatusDead(c *gc.C) {
	u1 := application.AddUnitArg{
		UnitName: "foo/666",
	}
	_, unitUUIDs := s.createApplication(c, "foo", life.Dead, u1)
	unitUUID := unitUUIDs[0]

	_, err := s.state.GetUnitAgentStatus(context.Background(), unitUUID)
	c.Assert(err, jc.ErrorIs, statuserrors.UnitIsDead)
}

func (s *stateSuite) TestGetUnitAgentStatus(c *gc.C) {
	u1 := application.AddUnitArg{
		UnitName: "foo/666",
	}
	_, unitUUIDs := s.createApplication(c, "foo", life.Alive, u1)
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
	u1 := application.AddUnitArg{
		UnitName: "foo/666",
	}
	_, unitUUIDs := s.createApplication(c, "foo", life.Alive, u1)
	unitUUID := unitUUIDs[0]

	status := status.StatusInfo[status.UnitAgentStatusType]{
		Status:  status.UnitAgentStatusExecuting,
		Message: "it's executing",
		Data:    []byte(`{"foo": "bar"}`),
		Since:   ptr(time.Now()),
	}

	err := s.state.SetUnitAgentStatus(context.Background(), unitUUID, status)
	c.Assert(err, jc.ErrorIsNil)

	err = s.state.SetUnitPresence(context.Background(), coreunit.Name("foo/666"))
	c.Assert(err, jc.ErrorIsNil)

	gotStatus, err := s.state.GetUnitAgentStatus(context.Background(), unitUUID)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(gotStatus.Present, jc.IsTrue)
	assertStatusInfoEqual(c, gotStatus.StatusInfo, status)

	err = s.state.DeleteUnitPresence(context.Background(), coreunit.Name("foo/666"))
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
	u1 := application.AddUnitArg{
		UnitName: "foo/666",
	}
	_, unitUUIDs := s.createApplication(c, "foo", life.Dead, u1)
	unitUUID := unitUUIDs[0]

	_, err := s.state.GetUnitWorkloadStatus(context.Background(), unitUUID)
	c.Assert(err, jc.ErrorIs, statuserrors.UnitIsDead)
}

func (s *stateSuite) TestGetUnitWorkloadStatusUnsetStatus(c *gc.C) {
	u1 := application.AddUnitArg{
		UnitName: "foo/666",
	}
	_, unitUUIDs := s.createApplication(c, "foo", life.Alive, u1)
	unitUUID := unitUUIDs[0]

	_, err := s.state.GetUnitWorkloadStatus(context.Background(), unitUUID)
	c.Assert(err, jc.ErrorIs, statuserrors.UnitStatusNotFound)
}

func (s *stateSuite) TestSetWorkloadStatus(c *gc.C) {
	u1 := application.AddUnitArg{
		UnitName: "foo/666",
	}
	_, unitUUIDs := s.createApplication(c, "foo", life.Alive, u1)
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
	u1 := application.AddUnitArg{
		UnitName: "foo/666",
	}
	_, unitUUIDs := s.createApplication(c, "foo", life.Alive, u1)
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
	u1 := application.AddUnitArg{
		UnitName: "foo/666",
	}
	_, unitUUIDs := s.createApplication(c, "foo", life.Alive, u1)
	unitUUID := unitUUIDs[0]

	sts := status.StatusInfo[status.WorkloadStatusType]{
		Status:  status.WorkloadStatusActive,
		Message: "it's active!",
		Data:    []byte(`{"foo": "bar"}`),
		Since:   ptr(time.Now()),
	}

	err := s.state.SetUnitWorkloadStatus(context.Background(), unitUUID, sts)
	c.Assert(err, jc.ErrorIsNil)

	err = s.state.SetUnitPresence(context.Background(), coreunit.Name("foo/666"))
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

func (s *stateSuite) TestGetUnitCloudContainerStatusUnset(c *gc.C) {
	u1 := application.AddUnitArg{
		UnitName: "foo/666",
	}
	_, unitUUIDs := s.createApplication(c, "foo", life.Alive, u1)
	unitUUID := unitUUIDs[0]

	sts, err := s.state.GetUnitCloudContainerStatus(context.Background(), unitUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(sts, gc.DeepEquals, status.StatusInfo[status.CloudContainerStatusType]{
		Status: status.CloudContainerStatusUnset,
	})
}

func (s *stateSuite) TestGetUnitCloudContainerStatusUnitNotFound(c *gc.C) {
	_, err := s.state.GetUnitCloudContainerStatus(context.Background(), "missing-uuid")
	c.Assert(err, jc.ErrorIs, statuserrors.UnitNotFound)
}

func (s *stateSuite) TestGetUnitCloudContainerStatusDead(c *gc.C) {
	u1 := application.AddUnitArg{
		UnitName: "foo/666",
	}
	_, unitUUIDs := s.createApplication(c, "foo", life.Dead, u1)
	unitUUID := unitUUIDs[0]

	_, err := s.state.GetUnitCloudContainerStatus(context.Background(), unitUUID)
	c.Assert(err, jc.ErrorIs, statuserrors.UnitIsDead)
}

func (s *stateSuite) TestGetUnitCloudContainerStatus(c *gc.C) {
	u1 := application.AddUnitArg{
		UnitName: "foo/666",
	}
	_, unitUUIDs := s.createApplication(c, "foo", life.Alive, u1)
	unitUUID := unitUUIDs[0]

	now := time.Now()

	s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		return s.state.setCloudContainerStatus(ctx, tx, unitUUID, status.StatusInfo[status.CloudContainerStatusType]{
			Status:  status.CloudContainerStatusRunning,
			Message: "it's running",
			Data:    []byte(`{"foo": "bar"}`),
			Since:   &now,
		})
	})

	sts, err := s.state.GetUnitCloudContainerStatus(context.Background(), unitUUID)
	c.Assert(err, jc.ErrorIsNil)
	assertStatusInfoEqual(c, sts, status.StatusInfo[status.CloudContainerStatusType]{
		Status:  status.CloudContainerStatusRunning,
		Message: "it's running",
		Data:    []byte(`{"foo": "bar"}`),
		Since:   &now,
	})
}

func (s *stateSuite) TestGetUnitWorkloadStatusesForApplication(c *gc.C) {
	u1 := application.AddUnitArg{
		UnitName: "foo/666",
	}
	appId, unitUUIDs := s.createApplication(c, "foo", life.Alive, u1)
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
	result, ok := results["foo/666"]
	c.Assert(ok, jc.IsTrue)
	c.Check(result.Present, jc.IsFalse)
	assertStatusInfoEqual(c, result.StatusInfo, status)
}

func (s *stateSuite) TestGetUnitWorkloadStatusesForApplicationMultipleUnits(c *gc.C) {
	u1 := application.AddUnitArg{
		UnitName: "foo/666",
	}
	u2 := application.AddUnitArg{
		UnitName: "foo/667",
	}
	appId, unitUUIDs := s.createApplication(c, "foo", life.Alive, u1, u2)
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

	result1, ok := results["foo/666"]
	c.Assert(ok, jc.IsTrue)
	c.Check(result1.Present, jc.IsFalse)
	assertStatusInfoEqual(c, result1.StatusInfo, status1)

	result2, ok := results["foo/667"]
	c.Assert(ok, jc.IsTrue)
	c.Check(result2.Present, jc.IsFalse)
	assertStatusInfoEqual(c, result2.StatusInfo, status2)
}

func (s *stateSuite) TestGetUnitWorkloadStatusesForApplicationMultipleUnitsPresent(c *gc.C) {
	u1 := application.AddUnitArg{
		UnitName: "foo/666",
	}
	u2 := application.AddUnitArg{
		UnitName: "foo/667",
	}
	appId, unitUUIDs := s.createApplication(c, "foo", life.Alive, u1, u2)
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
	err = s.state.SetUnitPresence(context.Background(), coreunit.Name("foo/667"))
	c.Assert(err, jc.ErrorIsNil)

	results, err := s.state.GetUnitWorkloadStatusesForApplication(context.Background(), appId)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, 2, gc.Commentf("expected 2, got %d", len(results)))

	result1, ok := results["foo/666"]
	c.Assert(ok, jc.IsTrue)
	c.Check(result1.Present, jc.IsFalse)
	assertStatusInfoEqual(c, result1.StatusInfo, status1)

	result2, ok := results["foo/667"]
	c.Assert(ok, jc.IsTrue)
	c.Check(result2.Present, jc.IsTrue)
	assertStatusInfoEqual(c, result2.StatusInfo, status2)
}

func (s *stateSuite) TestGetUnitWorkloadStatusesForApplicationNotFound(c *gc.C) {
	_, err := s.state.GetUnitWorkloadStatusesForApplication(context.Background(), "missing")
	c.Assert(err, jc.ErrorIs, statuserrors.ApplicationNotFound)
}

func (s *stateSuite) TestGetUnitWorkloadStatusesForApplicationNoUnits(c *gc.C) {
	appId, _ := s.createApplication(c, "foo", life.Alive)

	results, err := s.state.GetUnitWorkloadStatusesForApplication(context.Background(), appId)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, 0)
}

func (s *stateSuite) TestGetAllUnitStatusesForApplication(c *gc.C) {
	u1 := application.AddUnitArg{
		UnitName: "foo/666",
	}
	appId, unitUUIDs := s.createApplication(c, "foo", life.Alive, u1)
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
	cloudContainerStatus := status.StatusInfo[status.CloudContainerStatusType]{
		Status:  status.CloudContainerStatusRunning,
		Message: "it's running",
		Data:    []byte(`{"foo": "bar"}`),
		Since:   ptr(time.Now()),
	}
	err := s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		err := s.state.setCloudContainerStatus(ctx, tx, unitUUID, cloudContainerStatus)
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
	fullStatus, ok := fullStatuses["foo/666"]
	c.Assert(ok, jc.IsTrue)

	assertStatusInfoEqual(c, fullStatus.WorkloadStatus, workloadStatus)
	assertStatusInfoEqual(c, fullStatus.AgentStatus, agentStatus)
	assertStatusInfoEqual(c, fullStatus.ContainerStatus, cloudContainerStatus)
}

func (s *stateSuite) TestGetUnitCloudContainerStatusForApplicationMultipleUnits(c *gc.C) {
	u1 := application.AddUnitArg{
		UnitName: "foo/666",
	}
	u2 := application.AddUnitArg{
		UnitName: "foo/667",
	}
	appId, unitUUIDs := s.createApplication(c, "foo", life.Alive, u1, u2)
	unitUUID1 := unitUUIDs[0]
	unitUUID2 := unitUUIDs[1]

	workloadStatus := status.StatusInfo[status.WorkloadStatusType]{
		Status: status.WorkloadStatusActive,
	}
	agentStatus := status.StatusInfo[status.UnitAgentStatusType]{
		Status: status.UnitAgentStatusIdle,
	}

	status1 := status.StatusInfo[status.CloudContainerStatusType]{
		Status:  status.CloudContainerStatusRunning,
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
		return s.state.setCloudContainerStatus(ctx, tx, unitUUID1, status1)
	})
	c.Assert(err, jc.ErrorIsNil)

	status2 := status.StatusInfo[status.CloudContainerStatusType]{
		Status:  status.CloudContainerStatusBlocked,
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
		return s.state.setCloudContainerStatus(ctx, tx, unitUUID2, status2)
	})
	c.Assert(err, jc.ErrorIsNil)

	fullStatuses, err := s.state.GetAllFullUnitStatusesForApplication(context.Background(), appId)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(fullStatuses, gc.HasLen, 2)
	result1, ok := fullStatuses["foo/666"]
	c.Assert(ok, jc.IsTrue)
	assertStatusInfoEqual(c, result1.ContainerStatus, status1)

	result2, ok := fullStatuses["foo/667"]
	c.Assert(ok, jc.IsTrue)
	assertStatusInfoEqual(c, result2.ContainerStatus, status2)
}

func (s *stateSuite) TestGetAllUnitStatusesForApplicationNotFound(c *gc.C) {
	_, err := s.state.GetAllFullUnitStatusesForApplication(context.Background(), "missing")
	c.Assert(err, jc.ErrorIs, statuserrors.ApplicationNotFound)
}

func (s *stateSuite) TestGetAllUnitStatusesForApplicationNoUnits(c *gc.C) {
	appId, _ := s.createApplication(c, "foo", life.Alive)

	fullStatuses, err := s.state.GetAllFullUnitStatusesForApplication(context.Background(), appId)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(fullStatuses, gc.HasLen, 0)
}

func (s *stateSuite) TestGetAllUnitStatusesForApplicationUnitsWithoutStatuses(c *gc.C) {
	u1 := application.AddUnitArg{
		UnitName: "foo/666",
	}
	u2 := application.AddUnitArg{
		UnitName: "foo/667",
	}
	appId, _ := s.createApplication(c, "foo", life.Alive, u1, u2)

	_, err := s.state.GetAllFullUnitStatusesForApplication(context.Background(), appId)
	c.Assert(err, jc.ErrorIs, statuserrors.UnitStatusNotFound)
}

func (s *stateSuite) TestGetAllFullUnitStatusesEmptyModel(c *gc.C) {
	res, err := s.state.GetAllUnitWorkloadAgentStatuses(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(res, gc.HasLen, 0)
}

func (s *stateSuite) TestGetAllFullUnitStatusesNotFound(c *gc.C) {
	u1 := application.AddUnitArg{
		UnitName: "foo/666",
	}
	s.createApplication(c, "foo", life.Alive, u1)

	_, err := s.state.GetAllUnitWorkloadAgentStatuses(context.Background())
	c.Assert(err, jc.ErrorIs, statuserrors.UnitStatusNotFound)
}

func (s *stateSuite) TestGetAllFullUnitStatuses(c *gc.C) {
	u1 := application.AddUnitArg{
		UnitName: "foo/666",
	}
	u2 := application.AddUnitArg{
		UnitName: "foo/667",
	}
	u3 := application.AddUnitArg{
		UnitName: "bar/0",
	}
	_, fooUnitUUIDs := s.createApplication(c, "foo", life.Alive, u1, u2)
	u1UUID := fooUnitUUIDs[0]
	u2UUID := fooUnitUUIDs[1]
	_, barUnitUUIDs := s.createApplication(c, "bar", life.Alive, u3)
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

	err = s.state.SetUnitPresence(context.Background(), "foo/666")
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

	err = s.state.SetUnitPresence(context.Background(), "foo/667")
	c.Assert(err, jc.ErrorIsNil)
	err = s.state.DeleteUnitPresence(context.Background(), "foo/667")
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

	u1Full, ok := res["foo/666"]
	c.Assert(ok, jc.IsTrue)
	c.Check(u1Full.WorkloadStatus.Status, gc.Equals, status.WorkloadStatusActive)
	c.Check(u1Full.WorkloadStatus.Message, gc.Equals, "u1 is active!")
	c.Check(u1Full.WorkloadStatus.Data, gc.DeepEquals, []byte(`{"u1": "workload"}`))
	c.Check(u1Full.AgentStatus.Status, gc.Equals, status.UnitAgentStatusIdle)
	c.Check(u1Full.AgentStatus.Message, gc.Equals, "u1 is idle!")
	c.Check(u1Full.AgentStatus.Data, gc.DeepEquals, []byte(`{"u1": "agent"}`))
	c.Check(u1Full.Present, gc.Equals, true)

	u2Full, ok := res["foo/667"]
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
	u1 := application.AddUnitArg{
		UnitName: "foo/666",
	}
	s.createApplication(c, "foo", life.Alive, u1)
	s.createApplication(c, "bar", life.Alive)

	statuses, err := s.state.GetAllApplicationStatuses(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(statuses, gc.HasLen, 0)
}

func (s *stateSuite) TestGetAllApplicationStatuses(c *gc.C) {
	u1 := application.AddUnitArg{
		UnitName: "foo/666",
	}
	app1ID, _ := s.createApplication(c, "foo", life.Alive, u1)
	app2ID, _ := s.createApplication(c, "bar", life.Alive)
	s.createApplication(c, "goo", life.Alive)

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
	u1 := application.AddUnitArg{
		UnitName: "foo/666",
	}
	u2 := application.AddUnitArg{
		UnitName: "foo/667",
	}
	s.createApplication(c, "foo", life.Alive, u1, u2)

	err := s.state.SetUnitPresence(context.Background(), "foo/666")
	c.Assert(err, jc.ErrorIsNil)

	var lastSeen time.Time
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, "SELECT last_seen FROM v_unit_agent_presence WHERE name=?", "foo/666").Scan(&lastSeen); err != nil {
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
	u1 := application.AddUnitArg{
		UnitName: "foo/666",
	}
	u2 := application.AddUnitArg{
		UnitName: "foo/667",
	}
	s.createApplication(c, "foo", life.Alive, u1, u2)

	err := s.state.SetUnitPresence(context.Background(), "foo/666")
	c.Assert(err, jc.ErrorIsNil)

	var lastSeen time.Time
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, "SELECT last_seen FROM v_unit_agent_presence WHERE name=?", "foo/666").Scan(&lastSeen); err != nil {
			return err
		}
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(lastSeen.IsZero(), jc.IsFalse)
	c.Check(lastSeen.After(time.Now().Add(-time.Minute)), jc.IsTrue)

	err = s.state.DeleteUnitPresence(context.Background(), "foo/666")
	c.Assert(err, jc.ErrorIsNil)

	var count int
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM v_unit_agent_presence WHERE name=?", "foo/666").Scan(&count); err != nil {
			return err
		}
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(count, gc.Equals, 0)
}

// addRelationWithLifeAndID inserts a new relation into the database with the
// given details.
func (s *stateSuite) addRelationWithLifeAndID(c *gc.C, relationUUID string, life corelife.Value, relationID int) {
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.Exec(`
INSERT INTO relation (uuid, relation_id, life_id)
SELECT ?,  ?, id
FROM life
WHERE value = ?
`, relationUUID, relationID, life)
		return err
	})
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Arrange) Failed to insert relation %s, id %d", relationUUID, relationID))
}

// addRelationStatusWithMessage inserts a relation status into the relation_status table.
func (s *stateSuite) addRelationStatusWithMessage(c *gc.C, relationUUID string, status corestatus.Status,
	message string, since time.Time) {
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.Exec(`
INSERT INTO relation_status (relation_uuid, relation_status_type_id, suspended_reason, updated_at)
SELECT ?,rst.id,?,?
FROM relation_status_type rst
WHERE rst.name = ?
`, relationUUID, message, since, status)
		return err
	})
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("(Arrange) Failed to insert relation status %s, status %s, message %q",
		relationUUID, status, message))
}

func (s *stateSuite) createApplication(c *gc.C, name string, l life.Life, units ...application.AddUnitArg) (coreapplication.ID, []coreunit.UUID) {
	appState := applicationstate.NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))

	platform := application.Platform{
		Channel:      "22.04/stable",
		OSType:       application.Ubuntu,
		Architecture: architecture.ARM64,
	}
	channel := &application.Channel{
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
				Name: name,
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
		},
		CharmDownloadInfo: &charm.DownloadInfo{
			Provenance:         charm.ProvenanceDownload,
			CharmhubIdentifier: "ident",
			DownloadURL:        "https://example.com",
			DownloadSize:       42,
		},
		Scale: len(units),
	}, nil)
	c.Assert(err, jc.ErrorIsNil)

	charmUUID, err := appState.GetCharmIDByApplicationName(ctx, "foo")
	c.Assert(err, jc.ErrorIsNil)

	for _, u := range units {
		err := appState.AddIAASUnits(ctx, "", appID, charmUUID, u)
		c.Assert(err, jc.ErrorIsNil)
	}

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

		for i, u := range units {
			var uuid coreunit.UUID
			err := tx.QueryRowContext(ctx, "SELECT uuid FROM unit WHERE name = ?", u.UnitName).Scan(&uuid)
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
