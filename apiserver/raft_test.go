// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"time"

	"github.com/juju/errors"
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
	timeout := time.Second

	queue := queue.NewBlockingOpQueue()

	var commands []string

	done := make(chan struct{})
	defer close(done)
	go func() {
		for op := range queue.Queue() {
			commands = append(commands, string(op.Command))
			select {
			case <-done:
				return
			case queue.Error() <- nil:
			}
		}
	}()

	mediator := raftMediator{
		queue: queue,
	}
	err := mediator.ApplyLease(cmd, timeout)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(commands, gc.DeepEquals, []string{string(cmd)})
}

func (s *raftMediatorSuite) TestApplyLeaseError(c *gc.C) {
	cmd := []byte("do it")
	timeout := time.Second

	queue := queue.NewBlockingOpQueue()

	done := make(chan struct{})
	defer close(done)
	go func() {
		for range queue.Queue() {
			select {
			case <-done:
				return
			case queue.Error() <- errors.New("boom"):
			}
		}
	}()

	mediator := raftMediator{
		queue: queue,
	}
	err := mediator.ApplyLease(cmd, timeout)
	c.Assert(err, gc.ErrorMatches, `boom`)
}

func (s *raftMediatorSuite) TestApplyLeaseNotLeaderError(c *gc.C) {
	cmd := []byte("do it")
	timeout := time.Second

	queue := queue.NewBlockingOpQueue()

	done := make(chan struct{})
	defer close(done)
	go func() {
		for range queue.Queue() {
			select {
			case <-done:
				return
			case queue.Error() <- raft.NewNotLeaderError("10.0.0.0", "1"):
			}
		}
	}()

	mediator := raftMediator{
		queue: queue,
	}
	err := mediator.ApplyLease(cmd, timeout)
	c.Assert(err, gc.ErrorMatches, `not currently the leader, try "1"`)
}
