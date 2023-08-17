// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"errors"

	"github.com/juju/collections/set"
	jujuerrors "github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/database/testing"
	schematesting "github.com/juju/juju/domain/schema/testing"
)

type stateSuite struct {
	schematesting.ControllerSuite
}

var _ = gc.Suite(&stateSuite{})

func (s *stateSuite) TestCurateNodes(c *gc.C) {
	db := s.DB()

	_, err := db.Exec("INSERT INTO controller_node (controller_id) VALUES ('1')")
	c.Assert(err, jc.ErrorIsNil)

	err = NewState(testing.TxnRunnerFactory(s.TxnRunner())).CurateNodes(
		context.Background(), []string{"2", "3"}, []string{"1"})
	c.Assert(err, jc.ErrorIsNil)

	rows, err := db.Query("SELECT controller_id FROM controller_node")
	c.Assert(err, jc.ErrorIsNil)

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

func (s *stateSuite) TestUpdateUpdateDqliteNode(c *gc.C) {
	err := NewState(testing.TxnRunnerFactory(s.TxnRunner())).UpdateDqliteNode(
		context.Background(), "0", 12345, "192.168.5.60")
	c.Assert(err, jc.ErrorIsNil)

	row := s.DB().QueryRow("SELECT dqlite_node_id, bind_address FROM controller_node WHERE controller_id = '0'")
	c.Assert(row.Err(), jc.ErrorIsNil)

	var (
		id   int
		addr string
	)
	err = row.Scan(&id, &addr)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(id, gc.Equals, 12345)
	c.Check(addr, gc.Equals, "192.168.5.60")
}

func (s *stateSuite) TestSelectModelUUID(c *gc.C) {
	db := s.DB()

	_, err := db.Exec("INSERT INTO model_list (uuid) VALUES ('some-uuid')")
	c.Assert(err, jc.ErrorIsNil)

	st := NewState(testing.TxnRunnerFactory(s.TxnRunner()))

	uuid, err := st.SelectModelUUID(context.Background(), "not-there")
	c.Assert(errors.Is(err, jujuerrors.NotFound), jc.IsTrue)
	c.Check(uuid, gc.Equals, "")

	uuid, err = st.SelectModelUUID(context.Background(), "some-uuid")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(uuid, gc.Equals, "some-uuid")
}
