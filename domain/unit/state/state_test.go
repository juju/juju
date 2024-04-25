// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/domain/life"
	schematesting "github.com/juju/juju/domain/schema/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
)

type stateSuite struct {
	schematesting.ModelSuite

	state *State
}

var _ = gc.Suite(&stateSuite{})

func (s *stateSuite) SetUpTest(c *gc.C) {
	s.ModelSuite.SetUpTest(c)

	s.state = NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
}

func (s *stateSuite) TestDeleteUnit(c *gc.C) {
	s.insertUnit(c, "foo")

	err := s.state.DeleteUnit(context.Background(), "foo/666")
	c.Assert(err, jc.ErrorIsNil)

	var unitCount int
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT count(*) FROM unit WHERE unit_id=?", "foo/666").Scan(&unitCount)
		if err != nil {
			return errors.Trace(err)
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unitCount, gc.Equals, 0)
}

func (s *stateSuite) insertUnit(c *gc.C, appName string) {
	db := s.DB()

	applicationUUID := uuid.MustNewUUID().String()
	_, err := db.ExecContext(context.Background(), `
INSERT INTO application (uuid, name, life_id)
VALUES (?, ?, ?)
`, applicationUUID, appName, life.Alive)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(err, jc.ErrorIsNil)

	netNodeUUID := uuid.MustNewUUID().String()
	_, err = db.ExecContext(context.Background(), "INSERT INTO net_node (uuid) VALUES (?)", netNodeUUID)
	c.Assert(err, jc.ErrorIsNil)
	machineUUID := uuid.MustNewUUID().String()
	_, err = db.ExecContext(context.Background(), `
INSERT INTO unit (uuid, life_id, unit_id, net_node_uuid, application_uuid)
VALUES (?, ?, ?, ?, (SELECT uuid from application WHERE name = ?))
`, machineUUID, life.Alive, appName+"/0", netNodeUUID, appName)
	c.Assert(err, jc.ErrorIsNil)
}
