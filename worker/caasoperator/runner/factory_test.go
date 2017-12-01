// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package runner_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6/hooks"

	"github.com/juju/juju/worker/caasoperator/hook"
	"github.com/juju/juju/worker/caasoperator/runner"
	"github.com/juju/juju/worker/caasoperator/runner/context"
)

type FactorySuite struct {
	ContextSuite
}

var _ = gc.Suite(&FactorySuite{})

func (s *FactorySuite) AssertPaths(c *gc.C, rnr runner.Runner) {
	c.Assert(runner.RunnerPaths(rnr), gc.DeepEquals, s.paths)
}

func (s *FactorySuite) TestNewCommandRunnerNoRelation(c *gc.C) {
	rnr, err := s.factory.NewCommandRunner(context.CommandInfo{RelationId: -1})
	c.Assert(err, jc.ErrorIsNil)
	s.AssertPaths(c, rnr)
}

func (s *FactorySuite) TestNewCommandRunnerRelationIdDoesNotExist(c *gc.C) {
	for _, value := range []bool{true, false} {
		_, err := s.factory.NewCommandRunner(context.CommandInfo{
			RelationId: 12, ForceRemoteUnit: value,
		})
		c.Check(err, gc.ErrorMatches, `unknown relation id: 12`)
	}
}

func (s *FactorySuite) TestNewCommandRunnerRemoteUnitInvalid(c *gc.C) {
	for _, value := range []bool{true, false} {
		_, err := s.factory.NewCommandRunner(context.CommandInfo{
			RelationId: 0, RemoteUnitName: "blah", ForceRemoteUnit: value,
		})
		c.Check(err, gc.ErrorMatches, `invalid remote unit: blah`)
	}
}

func (s *FactorySuite) TestNewCommandRunnerRemoteUnitInappropriate(c *gc.C) {
	for _, value := range []bool{true, false} {
		_, err := s.factory.NewCommandRunner(context.CommandInfo{
			RelationId: -1, RemoteUnitName: "blah/123", ForceRemoteUnit: value,
		})
		c.Check(err, gc.ErrorMatches, `remote unit provided without a relation: blah/123`)
	}
}

func (s *FactorySuite) TestNewCommandRunnerEmptyRelation(c *gc.C) {
	_, err := s.factory.NewCommandRunner(context.CommandInfo{RelationId: 1})
	c.Check(err, gc.ErrorMatches, `cannot infer remote unit in empty relation 1`)
}

func (s *FactorySuite) TestNewCommandRunnerRemoteUnitAmbiguous(c *gc.C) {
	s.membership[1] = []string{"foo/0", "foo/1"}
	_, err := s.factory.NewCommandRunner(context.CommandInfo{RelationId: 1})
	c.Check(err, gc.ErrorMatches, `ambiguous remote unit; possibilities are \[foo/0 foo/1\]`)
}

func (s *FactorySuite) TestNewCommandRunnerRemoteUnitMissing(c *gc.C) {
	s.membership[0] = []string{"foo/0", "foo/1"}
	_, err := s.factory.NewCommandRunner(context.CommandInfo{
		RelationId: 0, RemoteUnitName: "blah/123",
	})
	c.Check(err, gc.ErrorMatches, `unknown remote unit blah/123; possibilities are \[foo/0 foo/1\]`)
}

func (s *FactorySuite) TestNewCommandRunnerForceNoRemoteUnit(c *gc.C) {
	rnr, err := s.factory.NewCommandRunner(context.CommandInfo{
		RelationId: 0, ForceRemoteUnit: true,
	})
	c.Assert(err, jc.ErrorIsNil)
	s.AssertPaths(c, rnr)
}

func (s *FactorySuite) TestNewCommandRunnerForceRemoteUnitMissing(c *gc.C) {
	_, err := s.factory.NewCommandRunner(context.CommandInfo{
		RelationId: 0, RemoteUnitName: "blah/123", ForceRemoteUnit: true,
	})
	c.Assert(err, gc.IsNil)
}

func (s *FactorySuite) TestNewCommandRunnerInferRemoteUnit(c *gc.C) {
	s.membership[0] = []string{"foo/2"}
	rnr, err := s.factory.NewCommandRunner(context.CommandInfo{RelationId: 0})
	c.Assert(err, jc.ErrorIsNil)
	s.AssertPaths(c, rnr)
}

func (s *FactorySuite) TestNewHookRunner(c *gc.C) {
	rnr, err := s.factory.NewHookRunner(hook.Info{Kind: hooks.ConfigChanged})
	c.Assert(err, jc.ErrorIsNil)
	s.AssertPaths(c, rnr)
}

func (s *FactorySuite) TestNewHookRunnerWithBadHook(c *gc.C) {
	rnr, err := s.factory.NewHookRunner(hook.Info{})
	c.Assert(rnr, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, `unknown hook kind ""`)
}

func (s *FactorySuite) TestNewHookRunnerWithRelation(c *gc.C) {
	rnr, err := s.factory.NewHookRunner(hook.Info{
		Kind:       hooks.RelationChanged,
		RelationId: 1,
		RemoteUnit: "mysql",
	})
	c.Assert(err, jc.ErrorIsNil)
	s.AssertPaths(c, rnr)
}

func (s *FactorySuite) TestNewHookRunnerWithBadRelation(c *gc.C) {
	rnr, err := s.factory.NewHookRunner(hook.Info{
		Kind:       hooks.RelationChanged,
		RelationId: 12345,
		RemoteUnit: "mysql",
	})
	c.Assert(rnr, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, `unknown relation id: 12345`)
}
