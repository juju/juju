// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package container_test

import (
	"context"
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/worker/uniter/container"
	"github.com/juju/juju/worker/uniter/operation"
	"github.com/juju/juju/worker/uniter/remotestate"
	"github.com/juju/juju/worker/uniter/resolver"
)

type containerSuite struct{}

var _ = gc.Suite(&containerSuite{})

func (s *containerSuite) TestNoRemoteInitRequired(c *gc.C) {
	containerResolver := container.NewRemoteContainerInitResolver()
	localState := resolver.LocalState{}
	remoteState := remotestate.Snapshot{}
	_, err := containerResolver.NextOp(context.Background(), localState, remoteState, &mockOperations{})
	c.Assert(err, gc.DeepEquals, resolver.ErrNoOperation)
}

func (s *containerSuite) TestRunningStatusNil(c *gc.C) {
	containerResolver := container.NewRemoteContainerInitResolver()
	localState := resolver.LocalState{
		OutdatedRemoteCharm: true,
	}
	remoteState := remotestate.Snapshot{}
	_, err := containerResolver.NextOp(context.Background(), localState, remoteState, &mockOperations{})
	c.Assert(err, gc.DeepEquals, resolver.ErrNoOperation)
}

func (s *containerSuite) TestRemoteInitRequiredContinue(c *gc.C) {
	containerResolver := container.NewRemoteContainerInitResolver()
	localState := resolver.LocalState{
		State: operation.State{
			Kind: operation.Continue,
		},
		OutdatedRemoteCharm: true,
	}
	remoteState := remotestate.Snapshot{
		ContainerRunningStatus: &remotestate.ContainerRunningStatus{
			Initialising:     true,
			InitialisingTime: time.Now(),
			PodName:          "pod-name",
			Running:          false,
		},
	}
	op, err := containerResolver.NextOp(context.Background(), localState, remoteState, &mockOperations{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(op.String(), gc.Equals, "remote init")
}

func (s *containerSuite) TestRemoteInitRequiredRunHookPending(c *gc.C) {
	containerResolver := container.NewRemoteContainerInitResolver()
	localState := resolver.LocalState{
		State: operation.State{
			Kind: operation.RunHook,
			Step: operation.Pending,
		},
		OutdatedRemoteCharm: true,
	}
	remoteState := remotestate.Snapshot{
		ContainerRunningStatus: &remotestate.ContainerRunningStatus{
			Initialising:     true,
			InitialisingTime: time.Now(),
			PodName:          "pod-name",
			Running:          false,
		},
	}
	op, err := containerResolver.NextOp(context.Background(), localState, remoteState, &mockOperations{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(op.String(), gc.Equals, "remote init")
}

func (s *containerSuite) TestRemoteInitRequiredRunHookNotPending(c *gc.C) {
	containerResolver := container.NewRemoteContainerInitResolver()
	localState := resolver.LocalState{
		State: operation.State{
			Kind: operation.RunHook,
			Step: operation.Done,
		},
		OutdatedRemoteCharm: true,
	}
	remoteState := remotestate.Snapshot{
		ContainerRunningStatus: &remotestate.ContainerRunningStatus{
			Initialising:     true,
			InitialisingTime: time.Now(),
			PodName:          "pod-name",
			Running:          false,
		},
	}
	_, err := containerResolver.NextOp(context.Background(), localState, remoteState, &mockOperations{})
	c.Assert(err, gc.DeepEquals, resolver.ErrNoOperation)
}

func (s *containerSuite) TestRemoteInitRequiredAndPending(c *gc.C) {
	containerResolver := container.NewRemoteContainerInitResolver()
	localState := resolver.LocalState{
		State: operation.State{
			Kind: operation.RemoteInit,
			Step: operation.Pending,
		},
		OutdatedRemoteCharm: true,
	}
	remoteState := remotestate.Snapshot{
		ContainerRunningStatus: &remotestate.ContainerRunningStatus{
			Initialising:     true,
			InitialisingTime: time.Now(),
			PodName:          "pod-name",
			Running:          false,
		},
	}
	op, err := containerResolver.NextOp(context.Background(), localState, remoteState, &mockOperations{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(op.String(), gc.Equals, "remote init")
}

func (s *containerSuite) TestRemoteInitRequiredAndDone(c *gc.C) {
	containerResolver := container.NewRemoteContainerInitResolver()
	localState := resolver.LocalState{
		State: operation.State{
			Kind: operation.RemoteInit,
			Step: operation.Done,
		},
		OutdatedRemoteCharm: true,
	}
	remoteState := remotestate.Snapshot{
		ContainerRunningStatus: &remotestate.ContainerRunningStatus{
			Initialising:     true,
			InitialisingTime: time.Now(),
			PodName:          "pod-name",
			Running:          false,
		},
	}
	op, err := containerResolver.NextOp(context.Background(), localState, remoteState, &mockOperations{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(op.String(), gc.Equals, "skip remote init")
}

func (s *containerSuite) TestReinit(c *gc.C) {
	containerResolver := container.NewRemoteContainerInitResolver()
	t := time.Now()
	localState := resolver.LocalState{
		State: operation.State{
			Kind: operation.Continue,
		},
		ContainerRunningStatus: &remotestate.ContainerRunningStatus{
			Initialising:     false,
			InitialisingTime: t,
			PodName:          "pod-name",
			Running:          true,
		},
	}
	remoteState := remotestate.Snapshot{
		ContainerRunningStatus: &remotestate.ContainerRunningStatus{
			Initialising:     true,
			InitialisingTime: t.Add(time.Second),
			PodName:          "pod-name",
			Running:          false,
		},
	}
	op, err := containerResolver.NextOp(context.Background(), localState, remoteState, &mockOperations{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(op.String(), gc.Equals, "remote init")
}
