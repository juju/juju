// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
)

type statusSuite struct {
	statetesting.StateSuite
}

var _ = gc.Suite(&statusSuite{})

func (s *statusSuite) TestPruneStatusHistory(c *gc.C) {
	var oldDoc state.StatusDoc
	var err error
	st := s.State
	globalKey := "BogusKey"
	for changeno := 1; changeno <= 200; changeno++ {
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
			Id:     changeno,
			Insert: hDoc,
		}

		err = state.RunTransaction(st, []txn.Op{h})
		c.Logf("Adding a history entry attempt n: %d", changeno)
		c.Assert(err, jc.ErrorIsNil)
	}
	history, err := state.StatusHistory(500, globalKey, st)
	c.Assert(history, gc.HasLen, 200)
	c.Assert(history[0].Message, gc.Equals, "Status change 200")
	c.Assert(history[199].Message, gc.Equals, "Status change 1")

	err = state.PruneStatusHistory(st, 100)
	c.Assert(err, jc.ErrorIsNil)
	history, err = state.StatusHistory(500, globalKey, st)
	c.Assert(history, gc.HasLen, 100)
	c.Assert(history[0].Message, gc.Equals, "Status change 200")
	c.Assert(history[99].Message, gc.Equals, "Status change 101")
}

func (s *statusSuite) TestTranslateLegacyAgentState(c *gc.C) {
	for i, test := range []struct {
		agentStatus     state.Status
		workloadStatus  state.Status
		workloadMessage string
		expected        state.Status
	}{
		{
			agentStatus: state.StatusAllocating,
			expected:    state.StatusPending,
		}, {
			agentStatus: state.StatusError,
			expected:    state.StatusError,
		}, {
			agentStatus:     state.StatusIdle,
			workloadStatus:  state.StatusMaintenance,
			expected:        state.StatusPending,
			workloadMessage: "installing charm software",
		}, {
			agentStatus:     state.StatusIdle,
			workloadStatus:  state.StatusMaintenance,
			expected:        state.StatusStarted,
			workloadMessage: "backing up",
		}, {
			agentStatus:    state.StatusIdle,
			workloadStatus: state.StatusTerminated,
			expected:       state.StatusStopped,
		}, {
			agentStatus:    state.StatusIdle,
			workloadStatus: state.StatusBlocked,
			expected:       state.StatusStarted,
		},
	} {
		c.Logf("test %d", i)
		legacy, ok := state.TranslateToLegacyAgentState(test.agentStatus, test.workloadStatus, test.workloadMessage)
		c.Check(ok, jc.IsTrue)
		c.Check(legacy, gc.Equals, test.expected)
	}
}

func (s *statusSuite) TestStatusNotFoundError(c *gc.C) {
	err := state.NewStatusNotFound("foo")
	c.Assert(state.IsStatusNotFound(err), jc.IsTrue)
	c.Assert(errors.IsNotFound(err), jc.IsTrue)
	c.Assert(err.Error(), gc.Equals, `status for key "foo" not found`)
	c.Assert(state.IsStatusNotFound(errors.New("foo")), jc.IsFalse)
}

func (s *statusSuite) TestAgentStatusDocValidation(c *gc.C) {
	for i, test := range []struct {
		status   state.Status
		info     string
		expected string
	}{
		{
			status:   state.StatusPending,
			info:     "",
			expected: `status "pending" is deprecated and invalid`,
		},
		{
			status:   state.StatusDown,
			info:     "",
			expected: `cannot set invalid status "down"`,
		},
		{
			status:   state.StatusStarted,
			info:     "",
			expected: `status "started" is deprecated and invalid`,
		},
		{
			status:   state.StatusStopped,
			info:     "",
			expected: `status "stopped" is deprecated and invalid`,
		},
		{
			status:   state.StatusAllocating,
			info:     state.StorageReadyMessage,
			expected: "",
		},
		{
			status:   state.StatusAllocating,
			info:     state.PreparingStorageMessage,
			expected: "",
		},
		{
			status:   state.StatusAllocating,
			info:     "an unexpected or invalid message",
			expected: `cannot set status "allocating"`,
		},
		{
			status:   state.StatusLost,
			info:     state.StorageReadyMessage,
			expected: `cannot set status "lost"`,
		},
		{
			status:   state.StatusLost,
			info:     state.PreparingStorageMessage,
			expected: `cannot set status "lost"`,
		},
		{
			status:   state.StatusError,
			info:     "",
			expected: `cannot set status "error" without info`,
		},
		{
			status:   state.StatusError,
			info:     "some error info",
			expected: "",
		},
		{
			status:   state.Status("bogus"),
			info:     "",
			expected: `cannot set invalid status "bogus"`,
		},
	} {
		c.Logf("test %d", i)
		r := state.ValidateUnitAgentDocDocSet(test.status, test.info)
		if test.expected != "" {
			c.Assert(r, gc.ErrorMatches, test.expected)
		} else {
			c.Assert(r, gc.IsNil)
		}
	}
}
