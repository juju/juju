// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package master_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/worker/master"
	"github.com/juju/juju/worker/workertest"
)

type FlagSuite struct {
	testing.IsolationSuite
	clock *testing.Clock
	conn  *mockConn
}

var _ = gc.Suite(&FlagSuite{})

func (s *FlagSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.clock = testing.NewClock(time.Time{})
	s.conn = &mockConn{master: true}
}

func (s *FlagSuite) newWorker(c *gc.C) *master.FlagWorker {
	worker, err := master.NewFlagWorker(master.FlagConfig{
		Clock:    s.clock,
		Conn:     s.conn,
		Duration: time.Hour,
	})
	c.Assert(err, jc.ErrorIsNil)
	return worker
}

func (s *FlagSuite) TestIsMasterError(c *gc.C) {
	s.conn.SetErrors(errors.New("squish"))
	worker, err := master.NewFlagWorker(master.FlagConfig{
		Clock:    s.clock,
		Conn:     s.conn,
		Duration: time.Hour,
	})
	c.Check(worker, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "squish")
	s.conn.CheckCallNames(c, "IsMaster")
}

func (s *FlagSuite) TestIsMasterFailure(c *gc.C) {
	s.conn.master = false
	worker := s.newWorker(c)
	defer workertest.CleanKill(c, worker)
	c.Check(worker.Check(), jc.IsFalse)
}

func (s *FlagSuite) TestIsMasterSuccess(c *gc.C) {
	worker := s.newWorker(c)
	defer workertest.CleanKill(c, worker)
	c.Check(worker.Check(), jc.IsTrue)
}

func (s *FlagSuite) TestPings(c *gc.C) {
	s.conn.SetErrors(nil, nil, errors.New("bewm"))

	worker := s.newWorker(c)
	defer workertest.DirtyKill(c, worker)

	s.clock.WaitAdvance(time.Hour, time.Second, 1)
	s.clock.WaitAdvance(time.Hour, time.Second, 1)

	err := workertest.CheckKill(c, worker)
	c.Assert(err, gc.ErrorMatches, "ping failed, flag invalidated: bewm")
	s.conn.CheckCallNames(c, "IsMaster", "Ping", "Ping")
}
