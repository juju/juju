// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package container_test

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/worker/uniter/container"
	"github.com/juju/juju/internal/worker/uniter/hook"
	"github.com/juju/juju/internal/worker/uniter/operation"
	"github.com/juju/juju/internal/worker/uniter/remotestate"
	"github.com/juju/juju/internal/worker/uniter/resolver"
)

type workloadSuite struct{}

var _ = gc.Suite(&workloadSuite{})

func (s *workloadSuite) TestWorkloadEventList(c *gc.C) {
	evt := container.WorkloadEvent{
		Type:         container.ReadyEvent,
		WorkloadName: "test",
	}
	cbCalled := false
	expectedErr := errors.Errorf("expected error")
	events := container.NewWorkloadEvents()
	id := events.AddWorkloadEvent(evt, func(err error) {
		c.Assert(err, gc.Equals, expectedErr)
		c.Assert(cbCalled, jc.IsFalse)
		cbCalled = true
	})
	c.Assert(id, gc.Not(gc.Equals), "")
	c.Assert(events.Events(), gc.DeepEquals, []container.WorkloadEvent{evt})
	c.Assert(events.EventIDs(), gc.DeepEquals, []string{id})
	evt2, cb, err := events.GetWorkloadEvent(id)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cb, gc.NotNil)
	c.Assert(evt2, gc.DeepEquals, evt)
	cb(expectedErr)
	c.Assert(cbCalled, jc.IsTrue)
}

func (s *workloadSuite) TestWorkloadEventListFail(c *gc.C) {
	events := container.NewWorkloadEvents()
	evt, cb, err := events.GetWorkloadEvent("nope")
	c.Assert(err, gc.ErrorMatches, "workload event nope not found")
	c.Assert(cb, gc.IsNil)
	c.Assert(evt, gc.DeepEquals, container.WorkloadEvent{})
}

func (s *workloadSuite) TestWorkloadReadyHook(c *gc.C) {
	events := container.NewWorkloadEvents()
	expectedErr := errors.Errorf("expected error")
	handler := func(err error) {
		c.Assert(err, gc.Equals, expectedErr)
	}
	containerResolver := container.NewWorkloadHookResolver(
		loggo.GetLogger("test"),
		events,
		events.RemoveWorkloadEvent)
	localState := resolver.LocalState{
		State: operation.State{
			Kind: operation.Continue,
			Step: operation.Pending,
		},
	}
	remoteState := remotestate.Snapshot{
		WorkloadEvents: []string{
			events.AddWorkloadEvent(container.WorkloadEvent{
				Type:         container.ReadyEvent,
				WorkloadName: "test",
			}, handler),
		},
	}
	opFactory := &mockOperations{}
	op, err := containerResolver.NextOp(context.Background(), localState, remoteState, opFactory)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(op, gc.NotNil)
	op = operation.Unwrap(op)
	hookOp, ok := op.(*mockRunHookOp)
	c.Assert(ok, jc.IsTrue)
	c.Assert(hookOp.hookInfo, gc.DeepEquals, hook.Info{
		Kind:         "pebble-ready",
		WorkloadName: "test",
	})
}

func (s *workloadSuite) TestWorkloadCustomNoticeHook(c *gc.C) {
	events := container.NewWorkloadEvents()
	expectedErr := errors.Errorf("expected error")
	handler := func(err error) {
		c.Assert(err, gc.Equals, expectedErr)
	}
	containerResolver := container.NewWorkloadHookResolver(
		loggo.GetLogger("test"),
		events,
		events.RemoveWorkloadEvent)
	localState := resolver.LocalState{
		State: operation.State{
			Kind: operation.Continue,
			Step: operation.Pending,
		},
	}
	remoteState := remotestate.Snapshot{
		WorkloadEvents: []string{
			events.AddWorkloadEvent(container.WorkloadEvent{
				Type:         container.CustomNoticeEvent,
				WorkloadName: "test",
				NoticeID:     "123",
				NoticeType:   "custom",
				NoticeKey:    "example.com/foo",
			}, handler),
		},
	}
	opFactory := &mockOperations{}
	op, err := containerResolver.NextOp(localState, remoteState, opFactory)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(op, gc.NotNil)
	op = operation.Unwrap(op)
	hookOp, ok := op.(*mockRunHookOp)
	c.Assert(ok, jc.IsTrue)
	c.Assert(hookOp.hookInfo, gc.DeepEquals, hook.Info{
		Kind:         "pebble-custom-notice",
		WorkloadName: "test",
		NoticeID:     "123",
		NoticeType:   "custom",
		NoticeKey:    "example.com/foo",
	})
}
