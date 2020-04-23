// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/operation"
	"github.com/juju/juju/worker/uniter/remotestate"
)

type RemoteInitSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&RemoteInitSuite{})

func (s *RemoteInitSuite) TestRemoteInit(c *gc.C) {
	callbacks := &RemoteInitCallbacks{
		MockRemoteInit: &MockRemoteInit{
			err: nil,
		},
	}
	var abort <-chan struct{} = make(chan struct{})
	factory := operation.NewFactory(operation.FactoryParams{
		Callbacks: callbacks,
		Abort:     abort,
	})
	runningStatus := remotestate.ContainerRunningStatus{
		PodName: "test",
	}
	op, err := factory.NewRemoteInit(runningStatus)
	c.Assert(err, jc.ErrorIsNil)

	newState, err := op.Prepare(operation.State{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(newState, gc.DeepEquals, &operation.State{
		Kind: operation.RemoteInit,
		Step: operation.Pending,
	})
	c.Assert(callbacks.MockRemoteInit.gotRunningStatus, gc.IsNil)
	c.Assert(callbacks.MockRemoteInit.gotAbort, gc.IsNil)

	newState, err = op.Execute(*newState)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(newState, gc.DeepEquals, &operation.State{
		Kind: operation.RemoteInit,
		Step: operation.Done,
	})
	c.Assert(callbacks.MockRemoteInit.gotRunningStatus, gc.DeepEquals, &runningStatus)
	c.Assert(callbacks.MockRemoteInit.gotAbort, gc.Equals, abort)

	newState, err = op.Commit(*newState)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(newState, gc.DeepEquals, &operation.State{
		Kind: operation.Continue,
		Step: operation.Pending,
	})
}

func (s *RemoteInitSuite) TestRemoteInitWithHook(c *gc.C) {
	callbacks := &RemoteInitCallbacks{
		MockRemoteInit: &MockRemoteInit{
			err: nil,
		},
	}
	var abort <-chan struct{} = make(chan struct{})
	factory := operation.NewFactory(operation.FactoryParams{
		Callbacks: callbacks,
		Abort:     abort,
	})
	runningStatus := remotestate.ContainerRunningStatus{
		PodName: "test",
	}
	op, err := factory.NewRemoteInit(runningStatus)
	c.Assert(err, jc.ErrorIsNil)

	newState, err := op.Prepare(operation.State{
		Kind: operation.RunHook,
		Step: operation.Pending,
		Hook: &hook.Info{
			Kind: hook.LeaderElected,
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(newState, gc.DeepEquals, &operation.State{
		Kind: operation.RemoteInit,
		Step: operation.Pending,
		Hook: &hook.Info{
			Kind: hook.LeaderElected,
		},
	})
	c.Assert(callbacks.MockRemoteInit.gotRunningStatus, gc.IsNil)
	c.Assert(callbacks.MockRemoteInit.gotAbort, gc.IsNil)

	newState, err = op.Execute(*newState)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(newState, gc.DeepEquals, &operation.State{
		Kind: operation.RemoteInit,
		Step: operation.Done,
		Hook: &hook.Info{
			Kind: hook.LeaderElected,
		},
	})
	c.Assert(callbacks.MockRemoteInit.gotRunningStatus, gc.DeepEquals, &runningStatus)
	c.Assert(callbacks.MockRemoteInit.gotAbort, gc.Equals, abort)

	newState, err = op.Commit(*newState)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(newState, gc.DeepEquals, &operation.State{
		Kind: operation.RunHook,
		Step: operation.Pending,
		Hook: &hook.Info{
			Kind: hook.LeaderElected,
		},
	})
}

func (s *RemoteInitSuite) TestRemoteInitFail(c *gc.C) {
	callbacks := &RemoteInitCallbacks{
		MockRemoteInit: &MockRemoteInit{
			err: errors.New("ooops"),
		},
	}
	var abort <-chan struct{} = make(chan struct{})
	factory := operation.NewFactory(operation.FactoryParams{
		Callbacks: callbacks,
		Abort:     abort,
	})
	runningStatus := remotestate.ContainerRunningStatus{
		PodName: "test",
	}
	op, err := factory.NewRemoteInit(runningStatus)
	c.Assert(err, jc.ErrorIsNil)

	newState, err := op.Prepare(operation.State{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(newState, gc.DeepEquals, &operation.State{
		Kind: operation.RemoteInit,
		Step: operation.Pending,
	})
	c.Assert(callbacks.MockRemoteInit.gotRunningStatus, gc.IsNil)
	c.Assert(callbacks.MockRemoteInit.gotAbort, gc.IsNil)

	newState, err = op.Execute(*newState)
	c.Assert(err, gc.ErrorMatches, "ooops")
	c.Assert(newState, gc.IsNil)
	c.Assert(callbacks.MockRemoteInit.gotRunningStatus, gc.DeepEquals, &runningStatus)
	c.Assert(callbacks.MockRemoteInit.gotAbort, gc.Equals, abort)
}

func (s *RemoteInitSuite) TestSkipRemoteInit(c *gc.C) {
	callbacks := &RemoteInitCallbacks{
		MockRemoteInit: &MockRemoteInit{
			err: nil,
		},
	}
	var abort <-chan struct{} = make(chan struct{})
	factory := operation.NewFactory(operation.FactoryParams{
		Callbacks: callbacks,
		Abort:     abort,
	})
	op, err := factory.NewSkipRemoteInit(false)
	c.Assert(err, jc.ErrorIsNil)

	newState, err := op.Prepare(operation.State{})
	c.Assert(err, gc.Equals, operation.ErrSkipExecute)
	c.Assert(newState, gc.IsNil)
	c.Assert(callbacks.MockRemoteInit.gotRunningStatus, gc.IsNil)
	c.Assert(callbacks.MockRemoteInit.gotAbort, gc.IsNil)

	newState, err = op.Execute(operation.State{})
	c.Assert(err, gc.Equals, operation.ErrSkipExecute)
	c.Assert(newState, gc.IsNil)
	c.Assert(callbacks.MockRemoteInit.gotRunningStatus, gc.IsNil)
	c.Assert(callbacks.MockRemoteInit.gotAbort, gc.IsNil)

	newState, err = op.Commit(operation.State{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(newState, gc.DeepEquals, &operation.State{
		Kind: operation.Continue,
		Step: operation.Pending,
	})
}

func (s *RemoteInitSuite) TestSkipRemoteInitWithHook(c *gc.C) {
	callbacks := &RemoteInitCallbacks{
		MockRemoteInit: &MockRemoteInit{
			err: nil,
		},
	}
	var abort <-chan struct{} = make(chan struct{})
	factory := operation.NewFactory(operation.FactoryParams{
		Callbacks: callbacks,
		Abort:     abort,
	})
	op, err := factory.NewSkipRemoteInit(false)
	c.Assert(err, jc.ErrorIsNil)

	newState, err := op.Prepare(operation.State{
		Kind: operation.RemoteInit,
		Step: operation.Pending,
		Hook: &hook.Info{
			Kind: hook.LeaderElected,
		},
	})
	c.Assert(err, gc.Equals, operation.ErrSkipExecute)
	c.Assert(newState, gc.IsNil)
	c.Assert(callbacks.MockRemoteInit.gotRunningStatus, gc.IsNil)
	c.Assert(callbacks.MockRemoteInit.gotAbort, gc.IsNil)

	newState, err = op.Execute(operation.State{
		Kind: operation.RemoteInit,
		Step: operation.Pending,
		Hook: &hook.Info{
			Kind: hook.LeaderElected,
		},
	})
	c.Assert(err, gc.Equals, operation.ErrSkipExecute)
	c.Assert(newState, gc.IsNil)
	c.Assert(callbacks.MockRemoteInit.gotRunningStatus, gc.IsNil)
	c.Assert(callbacks.MockRemoteInit.gotAbort, gc.IsNil)

	newState, err = op.Commit(operation.State{
		Kind: operation.RemoteInit,
		Step: operation.Pending,
		Hook: &hook.Info{
			Kind: hook.LeaderElected,
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(newState, gc.DeepEquals, &operation.State{
		Kind: operation.RunHook,
		Step: operation.Pending,
		Hook: &hook.Info{
			Kind: hook.LeaderElected,
		},
	})
}

func (s *RemoteInitSuite) TestSkipRemoteInitRetry(c *gc.C) {
	callbacks := &RemoteInitCallbacks{
		MockRemoteInit: &MockRemoteInit{
			err: nil,
		},
	}
	var abort <-chan struct{} = make(chan struct{})
	factory := operation.NewFactory(operation.FactoryParams{
		Callbacks: callbacks,
		Abort:     abort,
	})
	op, err := factory.NewSkipRemoteInit(true)
	c.Assert(err, jc.ErrorIsNil)

	newState, err := op.Prepare(operation.State{})
	c.Assert(err, gc.Equals, operation.ErrSkipExecute)
	c.Assert(newState, gc.IsNil)
	c.Assert(callbacks.MockRemoteInit.gotRunningStatus, gc.IsNil)
	c.Assert(callbacks.MockRemoteInit.gotAbort, gc.IsNil)

	newState, err = op.Execute(operation.State{})
	c.Assert(err, gc.Equals, operation.ErrSkipExecute)
	c.Assert(newState, gc.IsNil)
	c.Assert(callbacks.MockRemoteInit.gotRunningStatus, gc.IsNil)
	c.Assert(callbacks.MockRemoteInit.gotAbort, gc.IsNil)

	newState, err = op.Commit(operation.State{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(newState, gc.DeepEquals, &operation.State{
		Kind: operation.RemoteInit,
		Step: operation.Pending,
	})
}

func (s *RemoteInitSuite) TestSkipRemoteInitRetryWithHook(c *gc.C) {
	callbacks := &RemoteInitCallbacks{
		MockRemoteInit: &MockRemoteInit{
			err: nil,
		},
	}
	var abort <-chan struct{} = make(chan struct{})
	factory := operation.NewFactory(operation.FactoryParams{
		Callbacks: callbacks,
		Abort:     abort,
	})
	op, err := factory.NewSkipRemoteInit(true)
	c.Assert(err, jc.ErrorIsNil)

	newState, err := op.Prepare(operation.State{
		Kind: operation.RemoteInit,
		Step: operation.Done,
		Hook: &hook.Info{
			Kind: hook.LeaderElected,
		},
	})
	c.Assert(err, gc.Equals, operation.ErrSkipExecute)
	c.Assert(newState, gc.IsNil)
	c.Assert(callbacks.MockRemoteInit.gotRunningStatus, gc.IsNil)
	c.Assert(callbacks.MockRemoteInit.gotAbort, gc.IsNil)

	newState, err = op.Execute(operation.State{
		Kind: operation.RemoteInit,
		Step: operation.Done,
		Hook: &hook.Info{
			Kind: hook.LeaderElected,
		},
	})
	c.Assert(err, gc.Equals, operation.ErrSkipExecute)
	c.Assert(newState, gc.IsNil)
	c.Assert(callbacks.MockRemoteInit.gotRunningStatus, gc.IsNil)
	c.Assert(callbacks.MockRemoteInit.gotAbort, gc.IsNil)

	newState, err = op.Commit(operation.State{
		Kind: operation.RemoteInit,
		Step: operation.Done,
		Hook: &hook.Info{
			Kind: hook.LeaderElected,
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(newState, gc.DeepEquals, &operation.State{
		Kind: operation.RemoteInit,
		Step: operation.Pending,
		Hook: &hook.Info{
			Kind: hook.LeaderElected,
		},
	})
}
