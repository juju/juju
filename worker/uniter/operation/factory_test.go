// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation_test

import (
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"
)

type FactoryDeployTest struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&FactoryDeployTest{})

func (s *FactoryDeployTest) TestFatal(c *gc.C) {
	c.Fatalf("XXX")
}

type FactoryRunActionsTest struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&FactoryRunActionsTest{})

func (s *FactoryRunActionsTest) TestFatal(c *gc.C) {
	c.Fatalf("XXX")
}

type FactoryRunCommandsTest struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&FactoryRunCommandsTest{})

func (s *FactoryRunCommandsTest) TestFatal(c *gc.C) {
	c.Fatalf("XXX")
}

type FactoryRunHookTest struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&FactoryRunHookTest{})

func (s *FactoryRunHookTest) TestFatal(c *gc.C) {
	c.Fatalf("XXX")
}
