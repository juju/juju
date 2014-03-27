// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package local_test

import (
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/provider/local"
	"launchpad.net/juju-core/testing/testbase"
)

type lxcTest struct {
	testbase.LoggingSuite
}

var _ = gc.Suite(&lxcTest{})

func (*lxcTest) TestUseFastLXCForContainer(c *gc.C) {
	c.Assert(local.UseFastLXC(instance.ContainerType("")), jc.IsFalse)
	c.Assert(local.UseFastLXC(instance.KVM), jc.IsFalse)
}

func (t *lxcTest) TestUseFastLXC(c *gc.C) {
	for i, test := range []struct {
		message        string
		releaseVersion string
		expected       bool
	}{{
		message: "missing release file",
	}, {
		message:        "precise release",
		releaseVersion: "12.04",
	}, {
		message:        "trusty release",
		releaseVersion: "14.04",
		expected:       true,
	}, {
		message:        "unstable unicorn",
		releaseVersion: "14.10",
		expected:       true,
	}, {
		message:        "jaunty",
		releaseVersion: "9.10",
	}} {
		c.Logf("%v: %v", i, test.message)
		t.PatchValue(local.ReleaseVersion, func() string { return test.releaseVersion })
		value := local.UseFastLXC(instance.LXC)
		c.Assert(value, gc.Equals, test.expected)
	}
}
