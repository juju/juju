// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/juju/tc"

	coremodel "github.com/juju/juju/core/model"
	schematesting "github.com/juju/juju/domain/schema/testing"
)

type migrationSuite struct {
	schematesting.ControllerSuite
}

func TestMigrationSuite(t *testing.T) {
	tc.Run(t, &migrationSuite{})
}

func (s *migrationSuite) TestGetApplicationLeadershipForModelNoLeaders(c *tc.C) {
	modelUUID := tc.Must0(c, coremodel.NewUUID)

	state := NewMigrationState(s.TxnRunnerFactory())
	leases, err := state.GetApplicationLeadershipForModel(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(leases, tc.HasLen, 0)
}

func (s *migrationSuite) TestGetApplicationLeadershipForModel(c *tc.C) {
	modelUUID := tc.Must0(c, coremodel.NewUUID)

	state := NewMigrationState(s.TxnRunnerFactory())

	s.insertLease(c, "1", 1, modelUUID, "foo", "unit", -time.Hour, 24*time.Hour)

	leases, err := state.GetApplicationLeadershipForModel(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(leases, tc.DeepEquals, map[string]string{
		"foo": "unit",
	})
}

func (s *migrationSuite) TestGetApplicationLeadershipForModelSingularControllerType(c *tc.C) {
	modelUUID := tc.Must0(c, coremodel.NewUUID)

	state := NewMigrationState(s.TxnRunnerFactory())

	s.insertLease(c, "1", 1, modelUUID, "foo", "unit", -time.Hour, 24*time.Hour)
	s.insertLease(c, "2", 0, modelUUID, "controller", "abc", -time.Hour, 24*time.Hour)

	leases, err := state.GetApplicationLeadershipForModel(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(leases, tc.DeepEquals, map[string]string{
		"foo": "unit",
	})
}

func (s *migrationSuite) TestGetApplicationLeadershipForModelExpired(c *tc.C) {
	modelUUID := tc.Must0(c, coremodel.NewUUID)

	state := NewMigrationState(s.TxnRunnerFactory())

	s.insertLease(c, "1", 1, modelUUID, "foo", "unit", -48*time.Hour, -24*time.Hour)

	leases, err := state.GetApplicationLeadershipForModel(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(leases, tc.HasLen, 0)
}

func (s *migrationSuite) TestGetApplicationLeadershipForModelHeldAndExpired(c *tc.C) {
	modelUUID := tc.Must0(c, coremodel.NewUUID)
	otherUUID := tc.Must0(c, coremodel.NewUUID)

	state := NewMigrationState(s.TxnRunnerFactory())

	s.insertLease(c, "1", 1, modelUUID, "held", "held/0", -time.Hour, 24*time.Hour)
	s.insertLease(c, "2", 1, modelUUID, "expired", "expired/0", -48*time.Hour, -24*time.Hour)
	s.insertLease(c, "3", 1, otherUUID, "other", "other/0", -time.Hour, 24*time.Hour)

	leases, err := state.GetApplicationLeadershipForModel(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(leases, tc.DeepEquals, map[string]string{
		"held": "held/0",
	})
}

func (s *migrationSuite) insertLease(
	c *tc.C,
	uuid string,
	leaseType int,
	modelUUID coremodel.UUID,
	name, holder string,
	startOffset, expiryOffset time.Duration,
) {
	// SQL timestamp offset syntax differs between database dialects. The test
	// database runs locally, so use the same host clock to calculate fixtures.
	now := time.Now().UTC()
	start := now.Add(startOffset).Format(time.DateTime)
	expiry := now.Add(expiryOffset).Format(time.DateTime)
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO lease (uuid, lease_type_id, model_uuid, name, holder, start, expiry)
VALUES (?, ?, ?, ?, ?, ?, ?)
`, uuid, leaseType, modelUUID, name, holder, start, expiry)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}
