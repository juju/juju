// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	modeltesting "github.com/juju/juju/core/model/testing"
	schematesting "github.com/juju/juju/domain/schema/testing"
)

type leadershipSuite struct {
	schematesting.ControllerSuite
}

var _ = gc.Suite(&leadershipSuite{})

func (s *leadershipSuite) TestGetApplicationLeadershipForModelNoLeaders(c *gc.C) {
	modelUUID := modeltesting.GenModelUUID(c)

	state := NewLeadershipState(s.TxnRunnerFactory())
	leases, err := state.GetApplicationLeadershipForModel(context.Background(), modelUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(leases, gc.HasLen, 0)
}

func (s *leadershipSuite) TestGetApplicationLeadershipForModel(c *gc.C) {
	modelUUID := modeltesting.GenModelUUID(c)

	state := NewLeadershipState(s.TxnRunnerFactory())

	s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.Exec(`
INSERT INTO lease (uuid, lease_type_id, model_uuid, name, holder, start, expiry)
VALUES ('1', 1, ?, 'foo', 'unit', date('now'), date('now', '+1 day'))
`, modelUUID)
		return err
	})

	leases, err := state.GetApplicationLeadershipForModel(context.Background(), modelUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(leases, gc.DeepEquals, map[string]string{
		"foo": "unit",
	})
}

func (s *leadershipSuite) TestGetApplicationLeadershipForModelSingularControllerType(c *gc.C) {
	modelUUID := modeltesting.GenModelUUID(c)

	state := NewLeadershipState(s.TxnRunnerFactory())

	s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
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

	leases, err := state.GetApplicationLeadershipForModel(context.Background(), modelUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(leases, gc.DeepEquals, map[string]string{
		"foo": "unit",
	})
}

func (s *leadershipSuite) TestGetApplicationLeadershipForModelExpired(c *gc.C) {
	modelUUID := modeltesting.GenModelUUID(c)

	state := NewLeadershipState(s.TxnRunnerFactory())

	s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.Exec(`
INSERT INTO lease (uuid, lease_type_id, model_uuid, name, holder, start, expiry)
VALUES ('1', 1, ?, 'foo', 'unit', date('now', '-2 day'), date('now', '-1 day'))
`, modelUUID)
		return err
	})

	leases, err := state.GetApplicationLeadershipForModel(context.Background(), modelUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(leases, gc.HasLen, 0)
}
