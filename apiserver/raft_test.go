// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"context"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/yaml.v3"

	apiservererrors "github.com/juju/juju/v2/apiserver/errors"
	"github.com/juju/juju/v2/core/raft/queue"
	"github.com/juju/juju/v2/core/raftlease"
	"github.com/juju/juju/v2/worker/raft"
)

type raftMediatorSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&raftMediatorSuite{})

func (s *raftMediatorSuite) TestApplyLease(c *gc.C) {
	cmd := raftlease.Command{
		Operation: "claim",
		Lease:     "singular-worker",
	}

	q := queue.NewOpQueue(clock.WallClock)

	results := s.consume(c, q, 1, nil)

	mediator := raftMediator{
		queue:  q,
		logger: logger,
		clock:  clock.WallClock,
	}
	err := mediator.ApplyLease(context.Background(), cmd)
	c.Assert(err, jc.ErrorIsNil)

	s.matcheOne(c, results, cmd)
}

func (s *raftMediatorSuite) TestApplyLeaseError(c *gc.C) {
	cmd := raftlease.Command{
		Operation: "claim",
		Lease:     "singular-worker",
	}

	q := queue.NewOpQueue(clock.WallClock)
	results := s.consume(c, q, 1, errors.New("boom"))

	mediator := raftMediator{
		queue:  q,
		logger: logger,
		clock:  clock.WallClock,
	}
	err := mediator.ApplyLease(context.Background(), cmd)
	c.Assert(err, gc.ErrorMatches, `boom`)

	s.matcheOne(c, results, cmd)
}

func (s *raftMediatorSuite) TestApplyLeaseNotLeaderError(c *gc.C) {
	cmd := raftlease.Command{
		Operation: "claim",
		Lease:     "singular-worker",
	}

	q := queue.NewOpQueue(clock.WallClock)
	results := s.consume(c, q, 1, raft.NewNotLeaderError("10.0.0.0", "1"))

	mediator := raftMediator{
		queue:  q,
		logger: logger,
		clock:  clock.WallClock,
	}
	err := mediator.ApplyLease(context.Background(), cmd)
	c.Assert(err, gc.ErrorMatches, `not currently the leader, try "1"`)

	s.matcheOne(c, results, cmd)
}

func (s *raftMediatorSuite) TestApplyLeaseDeadlineExceededError(c *gc.C) {
	cmd := raftlease.Command{
		Operation: "claim",
		Lease:     "singular-worker",
	}

	deadLineErr := queue.ErrDeadlineExceeded
	q := queue.NewOpQueue(clock.WallClock)

	results := s.consume(c, q, 1, deadLineErr)

	mediator := raftMediator{
		queue:  q,
		logger: logger,
		clock:  clock.WallClock,
	}
	err := mediator.ApplyLease(context.Background(), cmd)
	c.Assert(err, gc.ErrorMatches, `enqueueing deadline exceeded`)
	c.Assert(apiservererrors.IsDeadlineExceededError(err), jc.IsTrue)

	s.matcheOne(c, results, cmd)
}

func (s *raftMediatorSuite) TestApplyLeaseContextDoneError(c *gc.C) {
	cmd := raftlease.Command{
		Operation: "claim",
		Lease:     "singular-worker",
	}

	q := queue.NewOpQueue(clock.WallClock)

	mediator := raftMediator{
		queue:  q,
		logger: logger,
		clock:  clock.WallClock,
	}

	ctx, cancel := context.WithCancel(context.Background())
	// Force the context to be canceled before we request the apply leases to
	// exercise the done path.
	cancel()

	err := mediator.ApplyLease(ctx, cmd)
	c.Assert(err, gc.ErrorMatches, `enqueueing canceled`)
	c.Assert(apiservererrors.IsDeadlineExceededError(err), jc.IsTrue)
}

func (s *raftMediatorSuite) consume(c *gc.C, q *queue.OpQueue, n int, err error) chan queue.OutOperation {
	results := make(chan queue.OutOperation, n)
	go func() {
		defer close(results)

		for {
			select {
			case ops := <-q.Queue():
				for _, op := range ops {
					results <- op
					op.Done(err)
				}

				return
			case <-time.After(testing.LongWait):
				c.Fatal("timed out waiting for operations")
			}
		}
	}()

	return results
}

func (s *raftMediatorSuite) matcheOne(c *gc.C, results chan queue.OutOperation, cmd raftlease.Command) {
	var commands []raftlease.Command
	for result := range results {
		var got raftlease.Command
		err := yaml.Unmarshal(result.Command, &got)
		c.Assert(err, jc.ErrorIsNil)

		commands = append(commands, got)
	}
	c.Assert(len(commands), gc.Equals, 1)
	c.Assert(commands, gc.DeepEquals, []raftlease.Command{cmd})
}
