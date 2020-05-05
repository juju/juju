// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package engine_test

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/jujud/agent/engine"
)

type FlagSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&FlagSuite{})

func (*FlagSuite) TestFlagOutputBadWorker(c *gc.C) {
	in := &stubWorker{}
	var out engine.Flag
	err := engine.FlagOutput(in, &out)
	c.Check(err, gc.ErrorMatches, `expected in to implement Flag; got a .*`)
	c.Check(out, gc.IsNil)
}

func (*FlagSuite) TestFlagOutputBadTarget(c *gc.C) {
	in := &stubFlagWorker{}
	var out interface{}
	err := engine.FlagOutput(in, &out)
	c.Check(err, gc.ErrorMatches, `expected out to be a \*Flag; got a .*`)
	c.Check(out, gc.IsNil)
}

func (*FlagSuite) TestFlagOutputSuccess(c *gc.C) {
	in := &stubFlagWorker{}
	var out engine.Flag
	err := engine.FlagOutput(in, &out)
	c.Check(err, jc.ErrorIsNil)
	c.Check(out, gc.Equals, in)
}

func (*FlagSuite) TestStaticFlagWorker(c *gc.C) {
	testStaticFlagWorker(c, false)
	testStaticFlagWorker(c, true)
}

func testStaticFlagWorker(c *gc.C, value bool) {
	w := engine.NewStaticFlagWorker(value)
	c.Assert(w, gc.NotNil)
	defer workertest.CleanKill(c, w)

	c.Assert(w, gc.Implements, new(engine.Flag))
	c.Assert(w.(engine.Flag).Check(), gc.Equals, value)
}

type stubWorker struct {
	worker.Worker
}

type stubFlagWorker struct {
	engine.Flag
	worker.Worker
}
