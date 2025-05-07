// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package engine_test

import (
	"github.com/juju/tc"
	"github.com/juju/testing"
	"github.com/juju/worker/v4/workertest"

	"github.com/juju/juju/agent/engine"
)

type FlagSuite struct {
	testing.IsolationSuite
}

var _ = tc.Suite(&FlagSuite{})

func (*FlagSuite) TestFlagOutputBadWorker(c *tc.C) {
	in := &stubWorker{}
	var out engine.Flag
	err := engine.FlagOutput(in, &out)
	c.Check(err, tc.ErrorMatches, `expected in to implement Flag; got a .*`)
	c.Check(out, tc.IsNil)
}

func (*FlagSuite) TestFlagOutputBadTarget(c *tc.C) {
	in := &stubFlagWorker{}
	var out interface{}
	err := engine.FlagOutput(in, &out)
	c.Check(err, tc.ErrorMatches, `expected out to be a \*Flag; got a .*`)
	c.Check(out, tc.IsNil)
}

func (*FlagSuite) TestFlagOutputSuccess(c *tc.C) {
	in := &stubFlagWorker{}
	var out engine.Flag
	err := engine.FlagOutput(in, &out)
	c.Check(err, tc.ErrorIsNil)
	c.Check(out, tc.Equals, in)
}

func (*FlagSuite) TestStaticFlagWorker(c *tc.C) {
	testStaticFlagWorker(c, false)
	testStaticFlagWorker(c, true)
}

func testStaticFlagWorker(c *tc.C, value bool) {
	w := engine.NewStaticFlagWorker(value)
	c.Assert(w, tc.NotNil)
	defer workertest.CleanKill(c, w)

	c.Assert(w, tc.Implements, new(engine.Flag))
	c.Assert(w.(engine.Flag).Check(), tc.Equals, value)
}
