// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gate_test

import (
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"
	dt "github.com/juju/worker/v4/dependency/testing"
	"github.com/juju/worker/v4/workertest"

	"github.com/juju/juju/agent/engine"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/gate"
)

type FlagSuite struct {
	testhelpers.IsolationSuite
}

func TestFlagSuite(t *stdtesting.T) {
	tc.Run(t, &FlagSuite{})
}

func (*FlagSuite) TestManifoldInputs(c *tc.C) {
	manifold := gate.FlagManifold(gate.FlagManifoldConfig{
		GateName: "emperrasque",
	})
	c.Check(manifold.Inputs, tc.DeepEquals, []string{"emperrasque"})
}

func (*FlagSuite) TestManifoldOutputBadWorker(c *tc.C) {
	manifold := gate.FlagManifold(gate.FlagManifoldConfig{})
	in := &dummyWorker{}
	var out engine.Flag
	err := manifold.Output(in, &out)
	c.Check(err, tc.ErrorMatches, `expected in to implement Flag; got a .*`)
	c.Check(out, tc.IsNil)
}

func (*FlagSuite) TestManifoldOutputBadTarget(c *tc.C) {
	manifold := gate.FlagManifold(gate.FlagManifoldConfig{})
	in := &gate.Flag{}
	var out interface{}
	err := manifold.Output(in, &out)
	c.Check(err, tc.ErrorMatches, `expected out to be a \*Flag; got a .*`)
	c.Check(out, tc.IsNil)
}

func (*FlagSuite) TestManifoldOutputSuccess(c *tc.C) {
	manifold := gate.FlagManifold(gate.FlagManifoldConfig{})
	in := &gate.Flag{}
	var out engine.Flag
	err := manifold.Output(in, &out)
	c.Check(err, tc.ErrorIsNil)
	c.Check(out, tc.Equals, in)
}

func (*FlagSuite) TestManifoldFilterCatchesErrUnlocked(c *tc.C) {
	manifold := gate.FlagManifold(gate.FlagManifoldConfig{})
	err := manifold.Filter(errors.Trace(gate.ErrUnlocked))
	c.Check(err, tc.Equals, dependency.ErrBounce)
}

func (*FlagSuite) TestManifoldFilterLeavesOtherErrors(c *tc.C) {
	manifold := gate.FlagManifold(gate.FlagManifoldConfig{})
	expect := errors.New("burble")
	actual := manifold.Filter(expect)
	c.Check(actual, tc.Equals, expect)
}

func (*FlagSuite) TestManifoldFilterLeavesNil(c *tc.C) {
	manifold := gate.FlagManifold(gate.FlagManifoldConfig{})
	err := manifold.Filter(nil)
	c.Check(err, tc.ErrorIsNil)
}

func (*FlagSuite) TestManifoldStartGateMissing(c *tc.C) {
	getter := dt.StubGetter(map[string]interface{}{
		"some-gate": dependency.ErrMissing,
	})
	manifold := gate.FlagManifold(gate.FlagManifoldConfig{
		GateName: "some-gate",
	})
	worker, err := manifold.Start(c.Context(), getter)
	c.Check(worker, tc.IsNil)
	c.Check(errors.Cause(err), tc.Equals, dependency.ErrMissing)
}

func (*FlagSuite) TestManifoldStartError(c *tc.C) {
	expect := &dummyWaiter{}
	getter := dt.StubGetter(map[string]interface{}{
		"some-gate": expect,
	})
	manifold := gate.FlagManifold(gate.FlagManifoldConfig{
		GateName: "some-gate",
		NewWorker: func(actual gate.Waiter) (worker.Worker, error) {
			c.Check(actual, tc.Equals, expect)
			return nil, errors.New("gronk")
		},
	})
	worker, err := manifold.Start(c.Context(), getter)
	c.Check(worker, tc.IsNil)
	c.Check(err, tc.ErrorMatches, "gronk")
}

func (*FlagSuite) TestManifoldStartSuccess(c *tc.C) {
	getter := dt.StubGetter(map[string]interface{}{
		"some-gate": &dummyWaiter{},
	})
	expect := &dummyWorker{}
	manifold := gate.FlagManifold(gate.FlagManifoldConfig{
		GateName: "some-gate",
		NewWorker: func(_ gate.Waiter) (worker.Worker, error) {
			return expect, nil
		},
	})
	worker, err := manifold.Start(c.Context(), getter)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(worker, tc.Equals, expect)
}

func (*FlagSuite) TestFlagUnlocked(c *tc.C) {
	lock := gate.AlreadyUnlocked{}
	worker, err := gate.NewFlag(lock)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, worker)
	workertest.CheckAlive(c, worker)
	c.Check(worker.Check(), tc.IsTrue)
}

func (*FlagSuite) TestFlagLocked(c *tc.C) {
	lock := gate.NewLock()
	worker, err := gate.NewFlag(lock)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, worker)
	workertest.CheckAlive(c, worker)
	c.Check(worker.Check(), tc.IsFalse)
}

func (*FlagSuite) TestFlagUnlockError(c *tc.C) {
	lock := gate.NewLock()
	worker, err := gate.NewFlag(lock)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, worker)
	workertest.CheckAlive(c, worker)
	lock.Unlock()
	err = workertest.CheckKilled(c, worker)
	c.Check(err, tc.Equals, gate.ErrUnlocked)
}

type dummyWorker struct{ worker.Worker }
type dummyWaiter struct{ gate.Lock }
