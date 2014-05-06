// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dummy_test

import (
	stdtesting "testing"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs/jujutest"
	"launchpad.net/juju-core/provider/dummy"
	"launchpad.net/juju-core/testing"
)

func TestPackage(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}

func init() {
	gc.Suite(&jujutest.LiveTests{
		TestConfig:     dummy.SampleConfig(),
		CanOpenState:   true,
		HasProvisioner: false,
	})
	gc.Suite(&suite{
		Tests: jujutest.Tests{
			TestConfig: dummy.SampleConfig(),
		},
	})
}

type suite struct {
	jujutest.Tests
}

func (s *suite) TearDownTest(c *gc.C) {
	s.Tests.TearDownTest(c)
	dummy.Reset()
}
