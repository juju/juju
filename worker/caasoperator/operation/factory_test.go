// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation_test

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6/hooks"

	"github.com/juju/juju/worker/caasoperator/hook"
	"github.com/juju/juju/worker/caasoperator/operation"
)

type FactorySuite struct {
	testing.IsolationSuite
	factory operation.Factory
}

var _ = gc.Suite(&FactorySuite{})

func (s *FactorySuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	// Yes, this factory will produce useless ops; this suite is just for
	// verifying that inadequate args to the factory methods will produce
	// the expected errors; and that the results of same get a string
	// representation that does not depend on the factory attributes.
	s.factory = operation.NewFactory(operation.FactoryParams{})
}

func (s *FactorySuite) testNewHookError(c *gc.C, newHook newHook) {
	op, err := newHook(s.factory, hook.Info{Kind: hooks.Kind("gibberish")})
	c.Check(op, gc.IsNil)
	c.Check(err, gc.ErrorMatches, `unknown hook kind "gibberish"`)
}

func (s *FactorySuite) TestNewHookError_Run(c *gc.C) {
	s.testNewHookError(c, (operation.Factory).NewRunHook)
}

func (s *FactorySuite) TestNewHookError_Skip(c *gc.C) {
	s.testNewHookError(c, (operation.Factory).NewSkipHook)
}

func (s *FactorySuite) TestNewHookString_Run(c *gc.C) {
	op, err := s.factory.NewRunHook(hook.Info{Kind: hooks.ConfigChanged})
	c.Check(err, jc.ErrorIsNil)
	c.Check(op.String(), gc.Equals, "run config-changed hook")
}

func (s *FactorySuite) TestNewHookString_Skip(c *gc.C) {
	op, err := s.factory.NewSkipHook(hook.Info{
		Kind:       hooks.RelationChanged,
		RemoteUnit: "foo/22",
		RelationId: 123,
	})
	c.Check(err, jc.ErrorIsNil)
	c.Check(op.String(), gc.Equals, "skip run relation-changed (123; foo/22) hook")
}
