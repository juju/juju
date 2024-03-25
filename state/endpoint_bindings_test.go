// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"github.com/juju/charm/v13"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/state"
)

type bindingsSuite struct {
	ConnSuite

	oldMeta     *charm.Meta
	oldDefaults map[string]string
	newMeta     *charm.Meta
	newDefaults map[string]string

	clientSpace *state.Space
	appsSpace   *state.Space
	barbSpace   *state.Space
	dbSpace     *state.Space
}

var _ = gc.Suite(&bindingsSuite{})

func (s *bindingsSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)

	const dummyCharmWithOneOfEachRelationTypeAndExtraBindings = `
name: dummy
summary: "That's a dummy charm with one relation of each type and extra-bindings."
description: "This is a longer description."
provides:
  foo1:
    interface: phony
requires:
  bar1:
    interface: fake
peers:
  self:
    interface: dummy
extra-bindings:
  one-extra:
`
	oldCharm := s.AddMetaCharm(c, "dummy", dummyCharmWithOneOfEachRelationTypeAndExtraBindings, 1)
	s.oldMeta = oldCharm.Meta()
	s.oldDefaults = map[string]string{
		"":          network.AlphaSpaceId,
		"foo1":      network.AlphaSpaceId,
		"bar1":      network.AlphaSpaceId,
		"self":      network.AlphaSpaceId,
		"one-extra": network.AlphaSpaceId,
	}

	const dummyCharmWithTwoOfEachRelationTypeAndNoExtraBindings = `
name: dummy
summary: "That's a dummy charm with 2 relations for each type."
description: "This is a longer description."
provides:
  foo1:
    interface: phony
  foo2:
    interface: secret
requires:
  bar2: real
  bar3:
    interface: cool
peers:
  self:
    interface: dummy
  me: peer
`
	newCharm := s.AddMetaCharm(c, "dummy", dummyCharmWithTwoOfEachRelationTypeAndNoExtraBindings, 2)
	s.newMeta = newCharm.Meta()
	s.newDefaults = map[string]string{
		"foo1": network.AlphaSpaceId,
		"foo2": network.AlphaSpaceId,
		"bar2": network.AlphaSpaceId,
		"bar3": network.AlphaSpaceId,
		"self": network.AlphaSpaceId,
		"me":   network.AlphaSpaceId,
	}

	// Add some spaces to use in bindings, but notably NOT the default space, as
	// it should be always allowed.

	var err error
	s.clientSpace, err = s.State.AddSpace("client", "", nil)
	c.Assert(err, jc.ErrorIsNil)
	s.appsSpace, err = s.State.AddSpace("apps", "", nil)
	c.Assert(err, jc.ErrorIsNil)
	s.dbSpace, err = s.State.AddSpace("db", "", nil)
	c.Assert(err, jc.ErrorIsNil)
	s.barbSpace, err = s.State.AddSpace("barb3", "", nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *bindingsSuite) TestMergeWithModelConfigNonDefaultSpace(c *gc.C) {
	c.Skip("The default space is always alpha due to scaffolding in service of Dqlite migration.")
	err := s.Model.UpdateModelConfig(state.NoopConfigSchemaSource, map[string]interface{}{"default-space": s.appsSpace.Name()}, nil)
	c.Assert(err, jc.ErrorIsNil)

	currentMap := map[string]string{
		"foo1": s.clientSpace.Id(),
		"self": s.dbSpace.Id(),
	}
	updated := map[string]string{
		"":          s.appsSpace.Id(),
		"foo1":      s.clientSpace.Id(),
		"bar1":      s.appsSpace.Id(),
		"self":      s.dbSpace.Id(),
		"one-extra": s.appsSpace.Id(),
	}

	b, err := state.NewBindings(s.State, currentMap)
	c.Assert(err, jc.ErrorIsNil)
	isModified, err := b.Merge(nil, s.oldMeta)
	c.Check(err, jc.ErrorIsNil)
	c.Check(b.Map(), jc.DeepEquals, updated)
	c.Check(isModified, gc.Equals, true)
}
