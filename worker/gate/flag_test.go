// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gate_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"
	dt "github.com/juju/worker/v2/dependency/testing"
	"github.com/juju/worker/v2/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/jujud/agent/engine"
	"github.com/juju/juju/worker/gate"
)

type FlagSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&FlagSuite{})

func (*FlagSuite) TestManifoldInputs(c *gc.C) {
	manifold := gate.FlagManifold(gate.FlagManifoldConfig{
		GateName: "emperrasque",
	})
	c.Check(manifold.Inputs, jc.DeepEquals, []string{"emperrasque"})
}

func (*FlagSuite) TestManifoldOutputBadWorker(c *gc.C) {
	manifold := gate.FlagManifold(gate.FlagManifoldConfig{})
	in := &dummyWorker{}
	var out engine.Flag
	err := manifold.Output(in, &out)
	c.Check(err, gc.ErrorMatches, `expected in to implement Flag; got a .*`)
	c.Check(out, gc.IsNil)
}

func (*FlagSuite) TestManifoldOutputBadTarget(c *gc.C) {
	manifold := gate.FlagManifold(gate.FlagManifoldConfig{})
	in := &gate.Flag{}
	var out interface{}
	err := manifold.Output(in, &out)
	c.Check(err, gc.ErrorMatches, `expected out to be a \*Flag; got a .*`)
	c.Check(out, gc.IsNil)
}

func (*FlagSuite) TestManifoldOutputSuccess(c *gc.C) {
	manifold := gate.FlagManifold(gate.FlagManifoldConfig{})
	in := &gate.Flag{}
	var out engine.Flag
	err := manifold.Output(in, &out)
	c.Check(err, jc.ErrorIsNil)
	c.Check(out, gc.Equals, in)
}

func (*FlagSuite) TestManifoldFilterCatchesErrUnlocked(c *gc.C) {
	manifold := gate.FlagManifold(gate.FlagManifoldConfig{})
	err := manifold.Filter(errors.Trace(gate.ErrUnlocked))
	c.Check(err, gc.Equals, dependency.ErrBounce)
}

func (*FlagSuite) TestManifoldFilterLeavesOtherErrors(c *gc.C) {
	manifold := gate.FlagManifold(gate.FlagManifoldConfig{})
	expect := errors.New("burble")
	actual := manifold.Filter(expect)
	c.Check(actual, gc.Equals, expect)
}

func (*FlagSuite) TestManifoldFilterLeavesNil(c *gc.C) {
	manifold := gate.FlagManifold(gate.FlagManifoldConfig{})
	err := manifold.Filter(nil)
	c.Check(err, jc.ErrorIsNil)
}

func (*FlagSuite) TestManifoldStartGateMissing(c *gc.C) {
	context := dt.StubContext(nil, map[string]interface{}{
		"some-gate": dependency.ErrMissing,
	})
	manifold := gate.FlagManifold(gate.FlagManifoldConfig{
		GateName: "some-gate",
	})
	worker, err := manifold.Start(context)
	c.Check(worker, gc.IsNil)
	c.Check(errors.Cause(err), gc.Equals, dependency.ErrMissing)
}

func (*FlagSuite) TestManifoldStartError(c *gc.C) {
	expect := &dummyWaiter{}
	context := dt.StubContext(nil, map[string]interface{}{
		"some-gate": expect,
	})
	manifold := gate.FlagManifold(gate.FlagManifoldConfig{
		GateName: "some-gate",
		NewWorker: func(actual gate.Waiter) (worker.Worker, error) {
			c.Check(actual, gc.Equals, expect)
			return nil, errors.New("gronk")
		},
	})
	worker, err := manifold.Start(context)
	c.Check(worker, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "gronk")
}

func (*FlagSuite) TestManifoldStartSuccess(c *gc.C) {
	context := dt.StubContext(nil, map[string]interface{}{
		"some-gate": &dummyWaiter{},
	})
	expect := &dummyWorker{}
	manifold := gate.FlagManifold(gate.FlagManifoldConfig{
		GateName: "some-gate",
		NewWorker: func(_ gate.Waiter) (worker.Worker, error) {
			return expect, nil
		},
	})
	worker, err := manifold.Start(context)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(worker, gc.Equals, expect)
}

func (*FlagSuite) TestFlagUnlocked(c *gc.C) {
	lock := gate.AlreadyUnlocked{}
	worker, err := gate.NewFlag(lock)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, worker)
	workertest.CheckAlive(c, worker)
	c.Check(worker.Check(), jc.IsTrue)
}

func (*FlagSuite) TestFlagLocked(c *gc.C) {
	lock := gate.NewLock()
	worker, err := gate.NewFlag(lock)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, worker)
	workertest.CheckAlive(c, worker)
	c.Check(worker.Check(), jc.IsFalse)
}

func (*FlagSuite) TestFlagUnlockError(c *gc.C) {
	lock := gate.NewLock()
	worker, err := gate.NewFlag(lock)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, worker)
	workertest.CheckAlive(c, worker)
	lock.Unlock()
	err = workertest.CheckKilled(c, worker)
	c.Check(err, gc.Equals, gate.ErrUnlocked)
}

type dummyWorker struct{ worker.Worker }
type dummyWaiter struct{ gate.Lock }
