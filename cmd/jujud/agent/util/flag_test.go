// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package util_test

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/jujud/agent/util"
	"github.com/juju/juju/worker"
)

type FlagSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&FlagSuite{})

func (*FlagSuite) TestFlagOutputBadWorker(c *gc.C) {
	in := &stubWorker{}
	var out util.Flag
	err := util.FlagOutput(in, &out)
	c.Check(err, gc.ErrorMatches, `expected in to implement Flag; got a .*`)
	c.Check(out, gc.IsNil)
}

func (*FlagSuite) TestFlagOutputBadTarget(c *gc.C) {
	in := &stubFlagWorker{}
	var out interface{}
	err := util.FlagOutput(in, &out)
	c.Check(err, gc.ErrorMatches, `expected out to be a \*Flag; got a .*`)
	c.Check(out, gc.IsNil)
}

func (*FlagSuite) TestFlagOutputSuccess(c *gc.C) {
	in := &stubFlagWorker{}
	var out util.Flag
	err := util.FlagOutput(in, &out)
	c.Check(err, jc.ErrorIsNil)
	c.Check(out, gc.Equals, in)
}

type stubWorker struct {
	worker.Worker
}

type stubFlagWorker struct {
	util.Flag
	worker.Worker
}
