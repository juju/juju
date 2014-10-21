// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context_test

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/worker/uniter/context"
)

type FactorySuite struct {
	testing.JujuConnSuite
}

var _ = gc.Suite(&FactorySuite{})

func (s *FactorySuite) TestFatal(c *gc.C) {
	c.Fatalf("GFY")
}

func (s *FactorySuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
}

func (s *FactorySuite) AssertCoreContext(c *gc.C, ctx *context.HookContext) {
	c.Fatalf("")
}

func (s *FactorySuite) TestNewRunContext(c *gc.C) {
	c.Fatalf("")
}

func (s *FactorySuite) TestNewHookContext(c *gc.C) {
	c.Fatalf("")
}

func (s *FactorySuite) TestNewHookContextWithRelation(c *gc.C) {
	c.Fatalf("")
}

func (s *FactorySuite) TestNewHookContextWithBadRelation(c *gc.C) {
	c.Fatalf("")
}

func (s *FactorySuite) TestNewActionContext(c *gc.C) {
	c.Fatalf("")
}

func (s *FactorySuite) TestNewActionContextWithBadAction(c *gc.C) {
	c.Fatalf("")
}
