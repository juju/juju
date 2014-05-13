// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package config_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/testing/testbase"
)

type lxcTest struct {
	testbase.LoggingSuite
}

var _ = gc.Suite(&lxcTest{})

func (t *lxcTest) TestUseFastLXC(c *gc.C) {
	for i, test := range []struct {
		message       string
		releaseSeries string
		expected      bool
	}{{
		message: "missing release file",
	}, {
		message:       "precise release",
		releaseSeries: "precise",
	}, {
		message:       "trusty release",
		releaseSeries: "trusty",
		expected:      true,
	}, {
		message:       "unstable unicorn",
		releaseSeries: "utopic",
		expected:      true,
	}, {
		message:       "lucid",
		releaseSeries: "lucid",
	}} {
		c.Logf("%v: %v", i, test.message)
		value := config.UseFastLXC(test.releaseSeries)
		c.Assert(value, gc.Equals, test.expected)
	}
}
