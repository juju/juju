// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/juju/state"
)

type BindingsSuite struct {
	ConnSuite

	oldMeta     *charm.Meta
	oldDefaults map[string]string
	newMeta     *charm.Meta
	newDefaults map[string]string
}

var _ = gc.Suite(&BindingsSuite{})

func (s *BindingsSuite) SetUpTest(c *gc.C) {
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
		"foo1":      "",
		"bar1":      "",
		"self":      "",
		"one-extra": "",
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
	s.newDefaults = map[string]string{}

	// Add some spaces to use in bindings, but notably NOT the default space, as
	// it should be always allowed.
	_, err := s.State.AddSpace("client", "", nil, true)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddSpace("apps", "", nil, false)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *BindingsSuite) TestMergeBindings(c *gc.C) {
	// The test cases below are not exhaustive, but just check basic
	// functionality. Most of the logic is tested by calling service.SetCharm()
	// in various ways.
	for i, test := range []struct {
		about          string
		newMap, oldMap map[string]string
		meta           *charm.Meta
		updated        map[string]string
		removed        []string
	}{{
		about:  "oldMap used when newMap is nil. Unbound endpoints are removed.",
		newMap: nil,
		oldMap: map[string]string{
			"foo1": "client",
			"self": "db",
		},
		meta: s.oldMeta,
		updated: map[string]string{
			"foo1":      "client",
			"bar1":      "",
			"self":      "db",
			"one-extra": "",
		},
		removed: []string{"bar1"},
	}, {
		about: "newMap overrides oldMap",
		newMap: map[string]string{
			"foo1":      "",
			"self":      "db",
			"bar1":      "client",
			"one-extra": "apps",
		},
		oldMap: map[string]string{
			"foo1": "client",
			"bar1": "db",
		},
		meta: s.oldMeta,
		updated: map[string]string{
			"foo1":      "",
			"bar1":      "client",
			"self":      "db",
			"one-extra": "apps",
		},
		removed: nil,
	}, {
		about: "newMap used when oldMap is nil",
		newMap: map[string]string{
			"self": "db",
		},
		oldMap: nil,
		meta:   s.oldMeta,
		updated: map[string]string{
			"foo1":      "",
			"bar1":      "",
			"self":      "db",
			"one-extra": "",
		},
		removed: []string{"bar1", "foo1"},
	}, {
		about:  "obsolete entries in oldMap are removed",
		newMap: nil,
		oldMap: map[string]string{
			"any-old-thing": "boo",
			"self":          "db",
			"one-extra":     "apps",
		},
		meta: s.oldMeta,
		updated: map[string]string{
			"foo1":      "",
			"bar1":      "",
			"self":      "db",
			"one-extra": "apps",
		},
		removed: []string{"any-old-thing"},
	}, {
		about: "new endpoints use defaults unless specified in newMap, existing ones are kept",
		newMap: map[string]string{
			"foo2": "db",
			"me":   "client",
			"bar3": "db",
		},
		oldMap: s.copyMap(s.oldDefaults),
		meta:   s.newMeta,
		updated: map[string]string{
			"foo1": "",
			"foo2": "db",
			"bar2": "",
			"bar3": "db",
			"self": "",
			"me":   "client",
		},
		removed: []string{"bar1", "one-extra"},
	}} {
		c.Logf("test #%d: %s", i, test.about)

		updated, removed, err := state.MergeBindings(test.newMap, test.oldMap, test.meta)
		c.Check(err, jc.ErrorIsNil)
		c.Check(updated, jc.DeepEquals, test.updated)
		c.Check(removed, jc.DeepEquals, test.removed)
	}
}

func (s *BindingsSuite) copyMap(input map[string]string) map[string]string {
	output := make(map[string]string, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}
