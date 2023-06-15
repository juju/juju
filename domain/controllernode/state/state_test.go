// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/juju/collections/set"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/database/testing"
)

type stateSuite struct {
	testing.ControllerSuite
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
