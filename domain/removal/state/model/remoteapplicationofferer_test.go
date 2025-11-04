// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/tc"

	coremodel "github.com/juju/juju/core/model"
	crossmodelrelationerrors "github.com/juju/juju/domain/crossmodelrelation/errors"
	crossmodelrelationstate "github.com/juju/juju/domain/crossmodelrelation/state/model"
	"github.com/juju/juju/domain/life"
	"github.com/juju/juju/domain/relation"
	removalerrors "github.com/juju/juju/domain/removal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type remoteApplicationOffererSuite struct {
	baseSuite
}

func TestRemoteApplicationOffererSuite(t *testing.T) {
	tc.Run(t, &remoteApplicationOffererSuite{})
}

func (s *remoteApplicationOffererSuite) TestGetRemoteApplicationOffererUUIDByApplicationUUID(c *tc.C) {
	appUUID, remoteAppUUID := s.createRemoteApplicationOfferer(c, "foo")

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	uuid, err := st.GetRemoteApplicationOffererUUIDByApplicationUUID(c.Context(), appUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(uuid, tc.Equals, remoteAppUUID.String())

	_, err = st.GetRemoteApplicationOffererUUIDByApplicationUUID(c.Context(), "not-today-henry")
	c.Assert(err, tc.ErrorIs, crossmodelrelationerrors.RemoteApplicationNotFound)
}

func (s *remoteApplicationOffererSuite) TestRemoteApplicationOffererExists(c *tc.C) {
	_, remoteAppUUID := s.createRemoteApplicationOfferer(c, "foo")

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	exists, err := st.RemoteApplicationOffererExists(c.Context(), remoteAppUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.Equals, true)

	exists, err = st.RemoteApplicationOffererExists(c.Context(), "not-today-henry")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.Equals, false)
}

func (s *remoteApplicationOffererSuite) TestEnsureRemoteApplicationOffererNotAliveCascadeNormalSuccess(c *tc.C) {
	_, remoteAppUUID := s.createRemoteApplicationOfferer(c, "foo")

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	artifacts, err := st.EnsureRemoteApplicationOffererNotAliveCascade(c.Context(), remoteAppUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	// We don't have any units, so we expect an empty slice for both unit and
	// machine UUIDs.
	c.Check(artifacts.IsEmpty(), tc.Equals, true)
	s.checkRemoteApplicationOffererLife(c, remoteAppUUID.String(), life.Dying)
}

func (s *remoteApplicationOffererSuite) TestEnsureRemoteApplicationOffererNotAliveCascadeNormalSuccessWithCascadedRelations(c *tc.C) {
	_, remoteAppUUID := s.createRemoteApplicationOfferer(c, "foo")
	s.createIAASApplication(c, s.setupApplicationService(c), "bar")

	relSvc := s.setupRelationService(c)
	_, _, err := relSvc.AddRelation(c.Context(), "foo:foo", "bar:bar")
	c.Assert(err, tc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	artifacts, err := st.EnsureRemoteApplicationOffererNotAliveCascade(c.Context(), remoteAppUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	c.Check(artifacts.RelationUUIDs, tc.HasLen, 1)
	s.checkRemoteApplicationOffererLife(c, remoteAppUUID.String(), life.Dying)
}

func (s *remoteApplicationOffererSuite) TestEnsureRemoteApplicationOffererNotAliveCascadeNotExistsSuccess(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	// We don't care if it's already gone.
	_, err := st.EnsureRemoteApplicationOffererNotAliveCascade(c.Context(), "some-application-uuid")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *remoteApplicationOffererSuite) TestRemoteApplicationOffererScheduleRemovalNormalSuccess(c *tc.C) {
	_, remoteAppUUID := s.createRemoteApplicationOfferer(c, "foo")

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	when := time.Now().UTC()
	err := st.RemoteApplicationOffererScheduleRemoval(
		c.Context(), "removal-uuid", remoteAppUUID.String(), false, when,
	)
	c.Assert(err, tc.ErrorIsNil)

	// We should have a removal job scheduled immediately.
	row := s.DB().QueryRow(`
SELECT t.name, r.entity_uuid, r.force, r.scheduled_for
FROM   removal r JOIN removal_type t ON r.removal_type_id = t.id
where  r.uuid = ?`, "removal-uuid",
	)

	var (
		removalType  string
		rUUID        string
		force        bool
		scheduledFor time.Time
	)
	err = row.Scan(&removalType, &rUUID, &force, &scheduledFor)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(removalType, tc.Equals, "remote application offerer")
	c.Check(rUUID, tc.Equals, remoteAppUUID.String())
	c.Check(force, tc.Equals, false)
	c.Check(scheduledFor, tc.Equals, when)
}

func (s *remoteApplicationOffererSuite) TestRemoteApplicationOffererScheduleRemovalNotExistsSuccess(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	when := time.Now().UTC()
	err := st.RemoteApplicationOffererScheduleRemoval(
		c.Context(), "removal-uuid", "some-application-uuid", true, when,
	)
	c.Assert(err, tc.ErrorIsNil)

	// We should have a removal job scheduled immediately.
	// It doesn't matter that the application does not exist.
	// We rely on the worker to handle that fact.
	row := s.DB().QueryRow(`
SELECT t.name, r.entity_uuid, r.force, r.scheduled_for
FROM   removal r JOIN removal_type t ON r.removal_type_id = t.id
where  r.uuid = ?`, "removal-uuid",
	)

	var (
		removalType  string
		rUUID        string
		force        bool
		scheduledFor time.Time
	)
	err = row.Scan(&removalType, &rUUID, &force, &scheduledFor)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(removalType, tc.Equals, "remote application offerer")
	c.Check(rUUID, tc.Equals, "some-application-uuid")
	c.Check(force, tc.Equals, true)
	c.Check(scheduledFor, tc.Equals, when)
}

func (s *remoteApplicationOffererSuite) TestGetRemoteApplicationOffererLifeSuccess(c *tc.C) {
	_, remoteAppUUID := s.createRemoteApplicationOfferer(c, "foo")

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	l, err := st.GetRemoteApplicationOffererLife(c.Context(), remoteAppUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(l, tc.Equals, life.Alive)
}

func (s *remoteApplicationOffererSuite) TestGetRemoteApplicationOffererLifeDying(c *tc.C) {
	_, remoteAppUUID := s.createRemoteApplicationOfferer(c, "foo")

	_, err := s.DB().Exec(`UPDATE application_remote_offerer SET life_id = 1 WHERE uuid = ?`, remoteAppUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	l, err := st.GetRemoteApplicationOffererLife(c.Context(), remoteAppUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(l, tc.Equals, life.Dying)
}

func (s *remoteApplicationOffererSuite) TestGetRemoteApplicationOffererLifeNotFound(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	_, err := st.GetRemoteApplicationOffererLife(c.Context(), "some-application-uuid")
	c.Assert(err, tc.ErrorIs, crossmodelrelationerrors.RemoteApplicationNotFound)
}

func (s *remoteApplicationOffererSuite) TestDeleteRemoteApplicationOffererStillAlive(c *tc.C) {
	_, remoteAppUUID := s.createRemoteApplicationOfferer(c, "foo")

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	err := st.DeleteRemoteApplicationOfferer(c.Context(), remoteAppUUID.String())
	c.Assert(err, tc.ErrorIs, removalerrors.EntityStillAlive)
}

func (s *remoteApplicationOffererSuite) TestDeleteRemoteApplicationOffererSuccess(c *tc.C) {
	appUUID, remoteAppUUID := s.createRemoteApplicationOfferer(c, "foo")

	s.advanceRemoteApplicationOffererLife(c, remoteAppUUID.String(), life.Dying)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	err := st.DeleteRemoteApplicationOfferer(c.Context(), remoteAppUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	// The remote application offerer should be gone.
	exists, err := st.RemoteApplicationOffererExists(c.Context(), remoteAppUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.Equals, false)

	exists, err = st.ApplicationExists(c.Context(), appUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.Equals, false)

	s.checkNoCharmsExist(c)
}

func (s *remoteApplicationOffererSuite) TestDeleteRemoteApplicationOffererWithRelations(c *tc.C) {
	appUUID, remoteAppUUID := s.createRemoteApplicationOfferer(c, "foo")
	s.createIAASApplication(c, s.setupApplicationService(c), "bar")

	relSvc := s.setupRelationService(c)
	ep1, ep2, err := relSvc.AddRelation(c.Context(), "foo:foo", "bar:bar")
	c.Assert(err, tc.ErrorIsNil)
	relUUID, err := relSvc.GetRelationUUIDForRemoval(c.Context(), relation.GetRelationUUIDForRemovalArgs{
		Endpoints: []string{ep1.String(), ep2.String()},
	})
	c.Assert(err, tc.ErrorIsNil)

	s.advanceRemoteApplicationOffererLife(c, remoteAppUUID.String(), life.Dead)
	s.advanceRelationLife(c, relUUID, life.Dead)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	// This should fail because the remote application offerer has a relation.
	err = st.DeleteRemoteApplicationOfferer(c.Context(), remoteAppUUID.String())
	c.Check(err, tc.ErrorIs, removalerrors.RemovalJobIncomplete)
	c.Check(err, tc.ErrorIs, crossmodelrelationerrors.RemoteApplicationHasRelations)

	// Delete any relations associated with the remote application offerer.
	err = st.DeleteRelation(c.Context(), relUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	// Now we can delete the remote application offerer.
	err = st.DeleteRemoteApplicationOfferer(c.Context(), remoteAppUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	exists, err := st.RemoteApplicationOffererExists(c.Context(), remoteAppUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.Equals, false)

	exists, err = st.ApplicationExists(c.Context(), appUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.Equals, false)
}

func (s *remoteApplicationOffererSuite) TestDeleteRemoteApplicationOffererWithUnits(c *tc.C) {
	appUUID, remoteAppUUID := s.createRemoteApplicationOfferer(c, "foo")

	cmrState := crossmodelrelationstate.NewState(
		s.TxnRunnerFactory(), coremodel.UUID(s.ModelUUID()), testclock.NewClock(s.now), loggertesting.WrapCheckLog(c),
	)
	err := cmrState.EnsureUnitsExist(c.Context(), appUUID.String(), []string{"foo/0", "foo/1", "foo/2"})
	c.Assert(err, tc.ErrorIsNil)

	s.advanceRemoteApplicationOffererLife(c, remoteAppUUID.String(), life.Dying)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	err = st.DeleteRemoteApplicationOfferer(c.Context(), remoteAppUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	exists, err := st.RemoteApplicationOffererExists(c.Context(), remoteAppUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.Equals, false)

	exists, err = st.ApplicationExists(c.Context(), appUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.Equals, false)

	var numUnits int
	var numNetNodes int
	s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM unit").Scan(&numUnits)
		if err != nil {
			return err
		}
		err = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM net_node").Scan(&numNetNodes)
		if err != nil {
			return err
		}
		return nil
	})

	c.Check(numUnits, tc.Equals, 0)
	c.Check(numNetNodes, tc.Equals, 0)
}
