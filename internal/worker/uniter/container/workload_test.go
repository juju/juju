// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package container_test

import (
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/worker/uniter/container"
	"github.com/juju/juju/internal/worker/uniter/hook"
	"github.com/juju/juju/internal/worker/uniter/operation"
	"github.com/juju/juju/internal/worker/uniter/remotestate"
	"github.com/juju/juju/internal/worker/uniter/resolver"
)

type workloadSuite struct{}

func TestWorkloadSuite(t *stdtesting.T) { tc.Run(t, &workloadSuite{}) }
func (s *workloadSuite) TestWorkloadEventList(c *tc.C) {
	evt := container.WorkloadEvent{
		Type:         container.ReadyEvent,
		WorkloadName: "test",
	}
	cbCalled := false
	expectedErr := errors.Errorf("expected error")
	events := container.NewWorkloadEvents()
	id := events.AddWorkloadEvent(evt, func(err error) {
		c.Assert(err, tc.Equals, expectedErr)
		c.Assert(cbCalled, tc.IsFalse)
		cbCalled = true
	})
	c.Assert(id, tc.Not(tc.Equals), "")
	c.Assert(events.Events(), tc.DeepEquals, []container.WorkloadEvent{evt})
	c.Assert(events.EventIDs(), tc.DeepEquals, []string{id})
	evt2, cb, err := events.GetWorkloadEvent(id)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cb, tc.NotNil)
	c.Assert(evt2, tc.DeepEquals, evt)
	cb(expectedErr)
	c.Assert(cbCalled, tc.IsTrue)
}

func (s *workloadSuite) TestWorkloadEventListFail(c *tc.C) {
	events := container.NewWorkloadEvents()
	evt, cb, err := events.GetWorkloadEvent("nope")
	c.Assert(err, tc.ErrorMatches, "workload event nope not found")
	c.Assert(cb, tc.IsNil)
	c.Assert(evt, tc.DeepEquals, container.WorkloadEvent{})
}

func (s *workloadSuite) TestWorkloadReadyHook(c *tc.C) {
	events := container.NewWorkloadEvents()
	expectedErr := errors.Errorf("expected error")
	handler := func(err error) {
		c.Assert(err, tc.Equals, expectedErr)
	}
	containerResolver := container.NewWorkloadHookResolver(
		loggertesting.WrapCheckLog(c),
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
	op, err := containerResolver.NextOp(c.Context(), localState, remoteState, opFactory)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(op, tc.NotNil)
	op = operation.Unwrap(op)
	hookOp, ok := op.(*mockRunHookOp)
	c.Assert(ok, tc.IsTrue)
	c.Assert(hookOp.hookInfo, tc.DeepEquals, hook.Info{
		Kind:         "pebble-ready",
		WorkloadName: "test",
	})
}

func (s *workloadSuite) TestWorkloadCustomNoticeHook(c *tc.C) {
	events := container.NewWorkloadEvents()
	expectedErr := errors.Errorf("expected error")
	handler := func(err error) {
		c.Assert(err, tc.Equals, expectedErr)
	}
	containerResolver := container.NewWorkloadHookResolver(
		loggertesting.WrapCheckLog(c),
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
	op, err := containerResolver.NextOp(c.Context(), localState, remoteState, opFactory)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(op, tc.NotNil)
	op = operation.Unwrap(op)
	hookOp, ok := op.(*mockRunHookOp)
	c.Assert(ok, tc.IsTrue)
	c.Assert(hookOp.hookInfo, tc.DeepEquals, hook.Info{
		Kind:         "pebble-custom-notice",
		WorkloadName: "test",
		NoticeID:     "123",
		NoticeType:   "custom",
		NoticeKey:    "example.com/foo",
	})
}

// TestWorkloadCheckFailedHook tests that a workload check failed event
// is correctly translated into a hook operation.
func (s *workloadSuite) TestWorkloadCheckFailedHook(c *tc.C) {
	events := container.NewWorkloadEvents()
	expectedErr := errors.Errorf("expected error")
	handler := func(err error) {
		c.Assert(err, tc.Equals, expectedErr)
	}
	containerResolver := container.NewWorkloadHookResolver(
		loggertesting.WrapCheckLog(c),
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
				Type:         container.CheckFailedEvent,
				WorkloadName: "test",
				CheckName:    "http-check",
			}, handler),
		},
	}
	opFactory := &mockOperations{}
	op, err := containerResolver.NextOp(c.Context(), localState, remoteState, opFactory)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(op, tc.NotNil)
	op = operation.Unwrap(op)
	hookOp, ok := op.(*mockRunHookOp)
	c.Assert(ok, tc.IsTrue)
	c.Assert(hookOp.hookInfo, tc.DeepEquals, hook.Info{
		Kind:         "pebble-check-failed",
		WorkloadName: "test",
		CheckName:    "http-check",
	})
}

// TestWorkloadCheckRecoveredHook tests that a workload check recovered event
// is correctly translated into a hook operation.
func (s *workloadSuite) TestWorkloadCheckRecoveredHook(c *tc.C) {
	events := container.NewWorkloadEvents()
	expectedErr := errors.Errorf("expected error")
	handler := func(err error) {
		c.Assert(err, tc.Equals, expectedErr)
	}
	containerResolver := container.NewWorkloadHookResolver(
		loggertesting.WrapCheckLog(c),
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
				Type:         container.CheckRecoveredEvent,
				WorkloadName: "test",
				CheckName:    "http-check",
			}, handler),
		},
	}
	opFactory := &mockOperations{}
	op, err := containerResolver.NextOp(c.Context(), localState, remoteState, opFactory)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(op, tc.NotNil)
	op = operation.Unwrap(op)
	hookOp, ok := op.(*mockRunHookOp)
	c.Assert(ok, tc.IsTrue)
	c.Assert(hookOp.hookInfo, tc.DeepEquals, hook.Info{
		Kind:         "pebble-check-recovered",
		WorkloadName: "test",
		CheckName:    "http-check",
	})
}
