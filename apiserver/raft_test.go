// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"errors"

	"github.com/juju/clock"
	"github.com/juju/clock/testclock"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/worker/raft"
	"github.com/juju/juju/worker/raft/queue"
)

type raftMediatorSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&raftMediatorSuite{})

func (s *raftMediatorSuite) TestApplyLease(c *gc.C) {
	cmd := []byte("do it")

	queue := queue.NewBlockingOpQueue(testclock.NewClock(clock.WallClock.Now()))

	results := make(chan [][]byte, 1)
	go func() {
		defer close(results)

		for op := range queue.Queue() {
			results <- op.Commands
			queue.Error() <- nil
			break
		}
	}()

	mediator := raftMediator{
		queue:  queue,
		logger: logger,
	}
	err := mediator.ApplyLease(cmd)
	c.Assert(err, jc.ErrorIsNil)

	var commands []string
	for result := range results {
		c.Assert(len(result), gc.Equals, 1)
		commands = append(commands, string(result[0]))
	}
	c.Assert(len(commands), gc.Equals, 1)
	c.Assert(commands, gc.DeepEquals, []string{string(cmd)})
}

func (s *raftMediatorSuite) TestApplyLeaseError(c *gc.C) {
	cmd := []byte("do it")

	queue := queue.NewBlockingOpQueue(testclock.NewClock(clock.WallClock.Now()))

	results := make(chan [][]byte, 1)
	go func() {
		defer close(results)

		for op := range queue.Queue() {
			results <- op.Commands
			queue.Error() <- errors.New("boom")
			break
		}
	}()

	mediator := raftMediator{
		queue:  queue,
		logger: logger,
	}
	err := mediator.ApplyLease(cmd)
	c.Assert(err, gc.ErrorMatches, `boom`)
}

func (s *raftMediatorSuite) TestApplyLeaseNotLeaderError(c *gc.C) {
	cmd := []byte("do it")

	queue := queue.NewBlockingOpQueue(testclock.NewClock(clock.WallClock.Now()))

	results := make(chan [][]byte, 1)
	go func() {
		defer close(results)

		for op := range queue.Queue() {
			results <- op.Commands
			queue.Error() <- raft.NewNotLeaderError("10.0.0.0", "1")
			break
		}
	}()

	mediator := raftMediator{
		queue:  queue,
		logger: logger,
	}
	err := mediator.ApplyLease(cmd)
	c.Assert(err, gc.ErrorMatches, `not currently the leader, try "1"`)
}
