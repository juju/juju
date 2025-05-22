// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"testing"

	"github.com/juju/tc"

	modeltesting "github.com/juju/juju/core/model/testing"
	schematesting "github.com/juju/juju/domain/schema/testing"
)

type migrationSuite struct {
	schematesting.ControllerSuite
}

func TestMigrationSuite(t *testing.T) {
	tc.Run(t, &migrationSuite{})
}

func (s *migrationSuite) TestGetApplicationLeadershipForModelNoLeaders(c *tc.C) {
	modelUUID := modeltesting.GenModelUUID(c)

	state := NewMigrationState(s.TxnRunnerFactory())
	leases, err := state.GetApplicationLeadershipForModel(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(leases, tc.HasLen, 0)
}

func (s *migrationSuite) TestGetApplicationLeadershipForModel(c *tc.C) {
	modelUUID := modeltesting.GenModelUUID(c)

	state := NewMigrationState(s.TxnRunnerFactory())

	s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.Exec(`
INSERT INTO lease (uuid, lease_type_id, model_uuid, name, holder, start, expiry)
VALUES ('1', 1, ?, 'foo', 'unit', date('now'), date('now', '+1 day'))
`, modelUUID)
		return err
	})

	leases, err := state.GetApplicationLeadershipForModel(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(leases, tc.DeepEquals, map[string]string{
		"foo": "unit",
	})
}

func (s *migrationSuite) TestGetApplicationLeadershipForModelSingularControllerType(c *tc.C) {
	modelUUID := modeltesting.GenModelUUID(c)

	state := NewMigrationState(s.TxnRunnerFactory())

	s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.Exec(`
INSERT INTO lease (uuid, lease_type_id, model_uuid, name, holder, start, expiry)
VALUES ('1', 1, ?, 'foo', 'unit', date('now'), date('now', '+1 day'))
`, modelUUID)
		if err != nil {
			return err
		}
		_, err = tx.Exec(`
INSERT INTO lease (uuid, lease_type_id, model_uuid, name, holder, start, expiry)
VALUES ('2', 0, ?, 'controller', 'abc', date('now'), date('now', '+1 day'))
`, modelUUID)
		return err
	})

	leases, err := state.GetApplicationLeadershipForModel(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(leases, tc.DeepEquals, map[string]string{
		"foo": "unit",
	})
}

func (s *migrationSuite) TestGetApplicationLeadershipForModelExpired(c *tc.C) {
	modelUUID := modeltesting.GenModelUUID(c)

	state := NewMigrationState(s.TxnRunnerFactory())

	s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.Exec(`
INSERT INTO lease (uuid, lease_type_id, model_uuid, name, holder, start, expiry)
VALUES ('1', 1, ?, 'foo', 'unit', date('now', '-2 day'), date('now', '-1 day'))
`, modelUUID)
		return err
	})

	leases, err := state.GetApplicationLeadershipForModel(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(leases, tc.HasLen, 0)
}
