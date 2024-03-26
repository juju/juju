// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/juju/collections/set"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coremodel "github.com/juju/juju/core/model"
	modelcoretesting "github.com/juju/juju/core/model/testing"
	modelerrors "github.com/juju/juju/domain/model/errors"
	modelstatetesting "github.com/juju/juju/domain/model/state/testing"
	schematesting "github.com/juju/juju/domain/schema/testing"
)

type stateSuite struct {
	schematesting.ControllerSuite
}

var _ = gc.Suite(&stateSuite{})

func (s *stateSuite) TestCurateNodes(c *gc.C) {
	db := s.DB()

	_, err := db.ExecContext(context.Background(), "INSERT INTO controller_node (controller_id) VALUES ('1')")
	c.Assert(err, jc.ErrorIsNil)

	err = NewState(s.TxnRunnerFactory()).CurateNodes(
		context.Background(), []string{"2", "3"}, []string{"1"})
	c.Assert(err, jc.ErrorIsNil)

	rows, err := db.QueryContext(context.Background(), "SELECT controller_id FROM controller_node")
	c.Assert(err, jc.ErrorIsNil)
	defer rows.Close()

	ids := set.NewStrings()
	for rows.Next() {
		var addr string
		err := rows.Scan(&addr)
		c.Assert(err, jc.ErrorIsNil)
		ids.Add(addr)
	}
	c.Check(ids.Values(), gc.HasLen, 3)

	// Controller "0" is inserted as part of the bootstrapped schema.
	c.Check(ids.Contains("0"), jc.IsTrue)
	c.Check(ids.Contains("2"), jc.IsTrue)
	c.Check(ids.Contains("3"), jc.IsTrue)
}

func (s *stateSuite) TestUpdateDqliteNode(c *gc.C) {
	// This value would cause a driver error to be emitted if we
	// tried to pass it directly as a uint64 query parameter.
	nodeID := uint64(15237855465837235027)

	err := NewState(s.TxnRunnerFactory()).UpdateDqliteNode(
		context.Background(), "0", nodeID, "192.168.5.60")
	c.Assert(err, jc.ErrorIsNil)

	row := s.DB().QueryRowContext(context.Background(), "SELECT dqlite_node_id, bind_address FROM controller_node WHERE controller_id = '0'")
	c.Assert(row.Err(), jc.ErrorIsNil)

	var (
		id   uint64
		addr string
	)
	err = row.Scan(&id, &addr)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(id, gc.Equals, nodeID)
	c.Check(addr, gc.Equals, "192.168.5.60")
}

func (s *stateSuite) TestSelectModelUUID(c *gc.C) {
	testModelUUID := modelstatetesting.CreateTestModel(c, s.TxnRunnerFactory(), "some-test-model")

	fakeModelUUID := modelcoretesting.GenModelUUID(c)

	st := NewState(s.TxnRunnerFactory())

	uuid, err := st.SelectModelUUID(context.Background(), fakeModelUUID)
	c.Assert(err, jc.ErrorIs, modelerrors.NotFound)
	c.Check(uuid, gc.Equals, coremodel.UUID(""))

	uuid, err = st.SelectModelUUID(context.Background(), testModelUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(uuid, gc.Equals, testModelUUID)
}
