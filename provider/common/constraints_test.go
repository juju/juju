// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/provider/common"
	"launchpad.net/juju-core/testing/testbase"
)

type constraintsSuite struct {
	testbase.LoggingSuite
}

var _ = gc.Suite(&constraintsSuite{})

func (s *constraintsSuite) TestValidateConstraints(c *gc.C) {
	defer loggo.ResetWriters()
	logger := loggo.GetLogger("test")
	logger.SetLogLevel(loggo.DEBUG)
	tw := &loggo.TestWriter{}
	c.Assert(loggo.RegisterWriter("constraints-tester", tw, loggo.DEBUG), gc.IsNil)
	cons := constraints.MustParse("arch=amd64 instance-type=foo")
	envCons := constraints.MustParse("cpu-cores=2")
	env := &mockEnviron{
		config: configGetter(c),
	}
	combined, err := common.ValidateConstraints(logger, env, cons, envCons)
	c.Assert(err, gc.IsNil)
	c.Assert(combined, gc.DeepEquals, constraints.MustParse("arch=amd64 instance-type=foo"))
	c.Assert(tw.Log, jc.LogMatches, jc.SimpleMessages{{
		loggo.WARNING,
		`instance-type constraint "foo" not supported ` +
			`for anything, really provider "mock environment"`},
	})
}

func (s *constraintsSuite) TestValidateConstraintsNoMessage(c *gc.C) {
	defer loggo.ResetWriters()
	logger := loggo.GetLogger("test")
	logger.SetLogLevel(loggo.DEBUG)
	tw := &loggo.TestWriter{}
	c.Assert(loggo.RegisterWriter("constraints-tester", tw, loggo.DEBUG), gc.IsNil)
	cons := constraints.MustParse("arch=amd64")
	envCons := constraints.MustParse("cpu-cores=2")
	env := &mockEnviron{
		config: configGetter(c),
	}
	combined, err := common.ValidateConstraints(logger, env, cons, envCons)
	c.Assert(err, gc.IsNil)
	c.Assert(combined, gc.DeepEquals, constraints.MustParse("arch=amd64 cpu-cores=2"))
	c.Assert(tw.Log, jc.LogMatches, jc.SimpleMessages{})
}

var imageMatchConstraintTests = []struct{ in, out string }{
	{"arch=amd64", "arch=amd64"},
	{"arch=amd64 instance-type=foo", "arch=amd64"},
	{"instance-type=foo", "instance-type=foo"},
}

func (s *constraintsSuite) TestImageMatchConstraint(c *gc.C) {
	for _, test := range imageMatchConstraintTests {
		inCons := constraints.MustParse(test.in)
		outCons := constraints.MustParse(test.out)
		c.Check(common.ImageMatchConstraint(inCons), jc.DeepEquals, outCons)
	}
}
