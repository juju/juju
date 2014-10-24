// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context_test

import (
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v4/hooks"

	"github.com/juju/juju/worker/uniter/context"
	"github.com/juju/juju/worker/uniter/hook"
)

type FactorySuite struct {
	HookContextSuite
	factory context.Factory
}

var _ = gc.Suite(&FactorySuite{})

func (s *FactorySuite) SetUpTest(c *gc.C) {
	s.HookContextSuite.SetUpTest(c)
	factory, err := context.NewFactory(
		s.uniter,
		s.unit.Tag().(names.UnitTag),
		func() map[int]*context.ContextRelation {
			return s.relctxs
		},
	)
	c.Assert(err, gc.IsNil)
	s.factory = factory
}

func (s *FactorySuite) AssertCoreContext(c *gc.C, ctx *context.HookContext) {
	c.Assert(ctx.UnitName(), gc.Equals, "u/0")
	c.Assert(ctx.OwnerTag(), gc.Equals, s.service.GetOwnerTag())

	expect, expectOK := s.unit.PrivateAddress()
	actual, actualOK := ctx.PrivateAddress()
	c.Assert(actual, gc.Equals, expect)
	c.Assert(actualOK, gc.Equals, expectOK)

	expect, expectOK = s.unit.PublicAddress()
	actual, actualOK = ctx.PublicAddress()
	c.Assert(actual, gc.Equals, expect)
	c.Assert(actualOK, gc.Equals, expectOK)

	env, err := s.State.Environment()
	c.Assert(err, gc.IsNil)
	name, uuid := ctx.EnvInfo()
	c.Assert(name, gc.Equals, env.Name())
	c.Assert(uuid, gc.Equals, env.UUID())

	c.Assert(ctx.RelationIds(), gc.HasLen, 2)

	r, found := ctx.Relation(0)
	c.Assert(found, jc.IsTrue)
	c.Assert(r.Name(), gc.Equals, "db")
	c.Assert(r.FakeId(), gc.Equals, "db:0")

	r, found = ctx.Relation(1)
	c.Assert(found, jc.IsTrue)
	c.Assert(r.Name(), gc.Equals, "db")
	c.Assert(r.FakeId(), gc.Equals, "db:1")
}

func (s *FactorySuite) AssertNotActionContext(c *gc.C, ctx *context.HookContext) {
	c.Assert(ctx.ActionData(), gc.IsNil)
}

func (s *FactorySuite) AssertRelationContext(c *gc.C, ctx *context.HookContext, relId int) {
	rel, found := ctx.HookRelation()
	c.Assert(found, jc.IsTrue)
	c.Assert(rel.Id(), gc.Equals, relId)
}

func (s *FactorySuite) AssertNotRelationContext(c *gc.C, ctx *context.HookContext) {
	rel, found := ctx.HookRelation()
	c.Assert(rel, gc.IsNil)
	c.Assert(found, jc.IsFalse)
}

func (s *FactorySuite) TestNewRunContext(c *gc.C) {
	ctx, err := s.factory.NewRunContext()
	c.Assert(err, gc.IsNil)
	s.AssertCoreContext(c, ctx)
	s.AssertNotActionContext(c, ctx)
	s.AssertNotRelationContext(c, ctx)
}

func (s *FactorySuite) TestNewHookContext(c *gc.C) {
	ctx, err := s.factory.NewHookContext(hook.Info{Kind: hooks.ConfigChanged})
	c.Assert(err, gc.IsNil)
	s.AssertCoreContext(c, ctx)
	s.AssertNotActionContext(c, ctx)
	s.AssertNotRelationContext(c, ctx)
}

func (s *FactorySuite) TestNewHookContextWithBadHook(c *gc.C) {
	ctx, err := s.factory.NewHookContext(hook.Info{})
	c.Assert(ctx, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, `unknown hook kind ""`)
}

func (s *FactorySuite) TestNewHookContextWithRelation(c *gc.C) {
	ctx, err := s.factory.NewHookContext(hook.Info{
		Kind:       hooks.RelationBroken,
		RelationId: 1,
	})
	c.Assert(err, gc.IsNil)
	s.AssertCoreContext(c, ctx)
	s.AssertNotActionContext(c, ctx)
	s.AssertRelationContext(c, ctx, 1)
}

func (s *FactorySuite) TestNewHookContextWithBadRelation(c *gc.C) {
	ctx, err := s.factory.NewHookContext(hook.Info{
		Kind:       hooks.RelationBroken,
		RelationId: 12345,
	})
	c.Assert(ctx, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, `unknown relation id: 12345`)
}

func (s *FactorySuite) TestNewActionContext(c *gc.C) {
	tag := names.NewActionTag("blah_a_1")
	params := map[string]interface{}{"foo": "bar"}
	ctx, err := s.factory.NewActionContext(tag, "blah", params)
	c.Assert(err, gc.IsNil)
	s.AssertCoreContext(c, ctx)
	s.AssertNotRelationContext(c, ctx)
	c.Assert(ctx.ActionData(), jc.DeepEquals, context.NewActionData(
		&tag, params,
	))
}
