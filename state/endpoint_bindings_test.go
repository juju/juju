// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
)

type BindingsSuite struct {
	ConnSuite
}

var _ = gc.Suite(&BindingsSuite{})

func (s *BindingsSuite) TestValidateEndpointBindingsForCharm(c *gc.C) {
	err := state.ValidateEndpointBindingsForCharm(s.State, nil, nil)
	c.Assert(err, gc.ErrorMatches, "nil bindings not valid")
	c.Assert(err, jc.Satisfies, errors.IsNotValid)
	err = state.ValidateEndpointBindingsForCharm(s.State, map[string]string{}, nil)
	c.Assert(err, gc.ErrorMatches, "nil charm metadata not valid")
	c.Assert(err, jc.Satisfies, errors.IsNotValid)

	// Add some spaces to use in bindings, but notably NOT the default space, as
	// it should be always allowed.
	_, err = s.State.AddSpace("client", nil, true)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddSpace("apps", nil, false)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.Space(network.DefaultSpace)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	meta := s.addTestingCharmGetMeta(c)
	bindingsWithInvalidSpace := s.bindingsWithDefaults(c, meta,
		map[string]string{"foo1": "invalid"}, nil,
	)
	err = state.ValidateEndpointBindingsForCharm(s.State, bindingsWithInvalidSpace, meta)
	c.Assert(err, gc.ErrorMatches, `endpoint "foo1" bound to unknown space "invalid" not valid`)
	c.Assert(err, jc.Satisfies, errors.IsNotValid)

	bindingsWithMissingEndpoint := s.bindingsWithDefaults(c, meta, nil, []string{"foo2"})
	err = state.ValidateEndpointBindingsForCharm(s.State, bindingsWithMissingEndpoint, meta)
	c.Assert(err, gc.ErrorMatches, `endpoint "foo2" not bound to a space not valid`)
	c.Assert(err, jc.Satisfies, errors.IsNotValid)

	bindingsWithEmptySpace := s.bindingsWithDefaults(c, meta, map[string]string{"me": ""}, nil)
	err = state.ValidateEndpointBindingsForCharm(s.State, bindingsWithEmptySpace, meta)
	c.Assert(err, gc.ErrorMatches, `endpoint "me" not bound to a space not valid`)
	c.Assert(err, jc.Satisfies, errors.IsNotValid)

	bindingsWithUnknownEndpoint := s.bindingsWithDefaults(c, meta, map[string]string{"new": "thing"}, nil)
	err = state.ValidateEndpointBindingsForCharm(s.State, bindingsWithUnknownEndpoint, meta)
	c.Assert(err, gc.ErrorMatches, `unknown endpoint "new" binding to space "thing" not valid`)
	c.Assert(err, jc.Satisfies, errors.IsNotValid)

	bindingsWithOnlyDefaults := s.bindingsWithDefaults(c, meta, nil, nil)
	err = state.ValidateEndpointBindingsForCharm(s.State, bindingsWithOnlyDefaults, meta)
	c.Assert(err, jc.ErrorIsNil)

	bindingsWithExplicitSpaces := s.bindingsWithDefaults(c, meta,
		map[string]string{"bar2": "client", "self": "apps"}, nil,
	)
	err = state.ValidateEndpointBindingsForCharm(s.State, bindingsWithExplicitSpaces, meta)
	c.Assert(err, jc.ErrorIsNil)

	// Add the default space and retry the last case - should make no
	// difference.
	_, err = s.State.AddSpace(network.DefaultSpace, nil, true)
	c.Assert(err, jc.ErrorIsNil)
	err = state.ValidateEndpointBindingsForCharm(s.State, bindingsWithExplicitSpaces, meta)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *BindingsSuite) TestDefaultEndpointBindingsForCharm(c *gc.C) {
	bindings, err := state.DefaultEndpointBindingsForCharm(nil)
	c.Assert(err, gc.ErrorMatches, "nil charm metadata")
	c.Assert(bindings, gc.IsNil)

	meta := s.addTestingCharmGetMeta(c)
	bindings, err = state.DefaultEndpointBindingsForCharm(meta)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(bindings, gc.HasLen, len(meta.Provides)+len(meta.Requires)+len(meta.Peers))
	c.Assert(bindings, jc.DeepEquals, map[string]string{
		"foo1": network.DefaultSpace,
		"foo2": network.DefaultSpace,
		"bar1": network.DefaultSpace,
		"bar2": network.DefaultSpace,
		"self": network.DefaultSpace,
		"me":   network.DefaultSpace,
	})
}

func (s *BindingsSuite) TestCombinedCharmRelations(c *gc.C) {
	withNilMeta := func() {
		state.CombinedCharmRelations(nil)
	}
	c.Assert(withNilMeta, gc.PanicMatches, "nil charm metadata")
	meta := s.addTestingCharmGetMeta(c)
	allRelations := state.CombinedCharmRelations(meta)
	c.Assert(allRelations, gc.HasLen, len(meta.Provides)+len(meta.Requires)+len(meta.Peers))
	c.Assert(allRelations, jc.DeepEquals, map[string]charm.Relation{
		"foo1": meta.Provides["foo1"],
		"foo2": meta.Provides["foo2"],
		"bar1": meta.Requires["bar1"],
		"bar2": meta.Requires["bar2"],
		"self": meta.Peers["self"],
		"me":   meta.Peers["me"],
	})
}

func (s *BindingsSuite) addTestingCharmGetMeta(c *gc.C) *charm.Meta {
	const dummyCharmAllRelationTypesMetadata = `
name: dummy
summary: "That's a dummy charm including all relation types."
description: "This is a longer description."
provides:
  foo1:
    interface: phony
  foo2:
    interface: secret
requires:
  bar1:
    interface: fake
  bar2: real
peers:
  self:
    interface: dummy
  me: peer
`
	testCharm := s.AddMetaCharm(c, "dummy", dummyCharmAllRelationTypesMetadata, 0)
	return testCharm.Meta()
}

func (s *BindingsSuite) bindingsWithDefaults(
	c *gc.C,
	charmMeta *charm.Meta,
	updates map[string]string,
	deletes []string,
) map[string]string {
	mergedBindings := make(map[string]string)
	defaultBindings, err := state.DefaultEndpointBindingsForCharm(charmMeta)
	c.Assert(err, jc.ErrorIsNil)
	for key, defaultValue := range defaultBindings {
		mergedBindings[key] = defaultValue
	}
	for key, updatedValue := range updates {
		mergedBindings[key] = updatedValue
	}
	for _, key := range deletes {
		if _, found := mergedBindings[key]; found {
			delete(mergedBindings, key)
		}
	}
	return mergedBindings
}
