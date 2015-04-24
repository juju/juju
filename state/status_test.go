// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
)

type HistoryPrunerSuite struct {
	statetesting.StateSuite
}

func (s *HistoryPrunerSuite) SetUpTest(c *gc.C) {
	s.StateSuite.SetUpTest(c)
}

func (s *HistoryPrunerSuite) TestPruneStatusHistory(c *gc.C) {
	var oldDoc state.StatusDoc
	var err error
	st := s.State
	globalKey := "BogusKey"
	for changeno := 1; changeno <= 201; changeno++ {
		oldDoc = state.StatusDoc{
			Status:     "AGivenStatus",
			StatusInfo: fmt.Sprintf("Status change %d", changeno),
			StatusData: nil,
		}
		timestamp := state.NowToTheSecond()
		oldDoc.Updated = &timestamp

		hDoc := state.NewHistoricalStatusDoc(oldDoc, globalKey)

		h := txn.Op{
			C:      state.StatusesHistoryC,
			Id:     fmt.Sprintf("%s%d", globalKey, time.Now().UTC().UnixNano()),
			Insert: hDoc,
		}

		err = state.RunTransaction(st, []txn.Op{h})
		c.Logf("Adding a history entry attempt n: %d", changeno)
		c.Assert(err, jc.ErrorIsNil)
	}
	history, err := state.StatusHistory(500, globalKey, st)
	c.Assert(history, gc.HasLen, 200)
	c.Assert(history[0].Message, gc.Equals, "Status change 1")
	c.Assert(history[199].Message, gc.Equals, "Status change 200")

	err = state.PruneStatusHistory(st, 100)
	c.Assert(err, jc.ErrorIsNil)
	history, err = state.StatusHistory(500, globalKey, st)
	c.Assert(history, gc.HasLen, 100)
	c.Assert(history[0].Message, gc.Equals, "Status change 100")
	c.Assert(history[99].Message, gc.Equals, "Status change 200")

}
