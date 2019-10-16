// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/errors"
	"github.com/juju/juju/state/mocks"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/state"
)

type bindingsSuite struct {
	ConnSuite

	oldMeta     *charm.Meta
	oldDefaults map[string]string
	newMeta     *charm.Meta
	newDefaults map[string]string

	clientSpaceID string
	appsSpaceID   string
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
		"":          network.DefaultSpaceId,
		"foo1":      network.DefaultSpaceId,
		"bar1":      network.DefaultSpaceId,
		"self":      network.DefaultSpaceId,
		"one-extra": network.DefaultSpaceId,
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
		"foo1": network.DefaultSpaceId,
		"foo2": network.DefaultSpaceId,
		"bar2": network.DefaultSpaceId,
		"bar3": network.DefaultSpaceId,
		"self": network.DefaultSpaceId,
		"me":   network.DefaultSpaceId,
	}

	// Add some spaces to use in bindings, but notably NOT the default space, as
	// it should be always allowed.

	s.clientSpaceID = "1"
	s.appsSpaceID = "2"
}

func (s *bindingsSuite) TestMergeBindings(c *gc.C) {
	// The test cases below are not exhaustive, but just check basic
	// functionality. Most of the logic is tested by calling application.SetCharm()
	// in various ways.

	for i, test := range []struct {
		about          string
		newMap, oldMap map[string]string
		meta           *charm.Meta
		updated        map[string]string
		modified       bool
	}{{
		about:    "defaults used when both newMap and oldMap are nil",
		newMap:   nil,
		oldMap:   nil,
		meta:     s.oldMeta,
		updated:  s.copyMap(s.oldDefaults),
		modified: true,
	}, {
		about:  "oldMap overrides defaults, newMap is nil",
		newMap: nil,
		oldMap: map[string]string{
			"foo1": s.clientSpaceID,
			"self": "db",
		},
		meta: s.oldMeta,
		updated: map[string]string{
			"":          network.DefaultSpaceId,
			"foo1":      s.clientSpaceID,
			"bar1":      network.DefaultSpaceId,
			"self":      "db",
			"one-extra": network.DefaultSpaceId,
		},
		modified: true,
	}, {
		about: "oldMap overrides defaults, newMap overrides oldMap",
		newMap: map[string]string{
			"":          network.DefaultSpaceId,
			"foo1":      network.DefaultSpaceId,
			"self":      "db",
			"bar1":      s.clientSpaceID,
			"one-extra": s.appsSpaceID,
		},
		oldMap: map[string]string{
			"foo1": s.clientSpaceID,
			"bar1": "db",
		},
		meta: s.oldMeta,
		updated: map[string]string{
			"":          network.DefaultSpaceId,
			"foo1":      network.DefaultSpaceId,
			"bar1":      s.clientSpaceID,
			"self":      "db",
			"one-extra": s.appsSpaceID,
		},
		modified: true,
	}, {
		about: "newMap overrides defaults, oldMap is nil",
		newMap: map[string]string{
			"self": "db",
		},
		oldMap: nil,
		meta:   s.oldMeta,
		updated: map[string]string{
			"":          network.DefaultSpaceId,
			"foo1":      network.DefaultSpaceId,
			"bar1":      network.DefaultSpaceId,
			"self":      "db",
			"one-extra": network.DefaultSpaceId,
		},
		modified: true,
	}, {
		about:  "obsolete entries in oldMap missing in defaults are removed",
		newMap: nil,
		oldMap: map[string]string{
			"any-old-thing": "boo",
			"self":          "db",
			"one-extra":     s.appsSpaceID,
		},
		meta: s.oldMeta,
		updated: map[string]string{
			"":          network.DefaultSpaceId,
			"foo1":      network.DefaultSpaceId,
			"bar1":      network.DefaultSpaceId,
			"self":      "db",
			"one-extra": s.appsSpaceID,
		},
		modified: true,
	}, {
		about: "new endpoints use defaults unless specified in newMap, existing ones are kept",
		newMap: map[string]string{
			"foo2": "db",
			"me":   s.clientSpaceID,
			"bar3": "db",
		},
		oldMap: s.copyMap(s.oldDefaults),
		meta:   s.newMeta,
		updated: map[string]string{
			"":     network.DefaultSpaceId,
			"foo1": network.DefaultSpaceId,
			"foo2": "db",
			"bar2": network.DefaultSpaceId,
			"bar3": "db",
			"self": network.DefaultSpaceId,
			"me":   s.clientSpaceID,
		},
		modified: true,
	}, {
		about: "new default supersedes old default",
		newMap: map[string]string{
			"":     "newb",
			"bar3": "barb3",
		},
		oldMap: map[string]string{
			"":          "default",
			"foo1":      "default",
			"bar1":      "db",
			"self":      "",
			"one-extra": "old",
		},
		meta: s.newMeta,
		updated: map[string]string{
			"":     "newb",
			"foo1": "default",
			"foo2": "newb",
			"bar2": "newb",
			"bar3": "barb3",
			"self": "",
			"me":   "newb",
		},
		modified: true,
	}, {
		about: "new map one change",
		newMap: map[string]string{
			"self": "bar",
		},
		oldMap: map[string]string{
			"":          "default",
			"foo1":      "default",
			"bar1":      "db",
			"self":      "",
			"one-extra": "old",
		},
		meta: s.oldMeta,
		updated: map[string]string{
			"":          "default",
			"foo1":      "default",
			"bar1":      "db",
			"self":      "bar",
			"one-extra": "old",
		},
		modified: true,
	}, {
		about:  "old unchanged but different key",
		newMap: nil,
		oldMap: map[string]string{
			"":          "default",
			"bar1":      "db",
			"self":      "",
			"lost":      "old",
			"one-extra": "old",
		},
		meta: s.oldMeta,
		updated: map[string]string{
			"":          "default",
			"foo1":      "default",
			"bar1":      "db",
			"self":      "",
			"one-extra": "old",
		},
		modified: true,
	}} {
		c.Logf("test #%d: %s", i, test.about)
		b := state.NewBindingsForMergeTest(test.newMap)
		isModified, err := b.Merge(test.oldMap, test.meta)
		c.Check(err, jc.ErrorIsNil)
		c.Check(b.Map(), jc.DeepEquals, test.updated)
		c.Check(isModified, gc.Equals, test.modified)
	}
}

func (s *bindingsSuite) copyMap(input map[string]string) map[string]string {
	output := make(map[string]string, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}

var _ = gc.Suite(&bindingsMockSuite{})

type bindingsMockSuite struct {
	testing.IsolationSuite

	endpointBinding *mocks.MockEndpointBinding
}

func (s *bindingsMockSuite) TestNewBindingsNilMap(c *gc.C) {
	defer s.setup(c).Finish()
	s.expectIDsByName()
	s.expectNamesByID()

	binding, err := state.NewBindings(s.endpointBinding, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(binding, gc.NotNil)
	c.Assert(binding.Map(), gc.DeepEquals, map[string]string{})
}

func (s *bindingsMockSuite) TestNewBindingsByID(c *gc.C) {
	defer s.setup(c).Finish()
	s.expectIDsByName()
	s.expectNamesByID()
	initial := map[string]string{
		"db":      "2",
		"testing": "5",
		"empty":   "",
	}

	binding, err := state.NewBindings(s.endpointBinding, initial)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(binding, gc.NotNil)

	c.Assert(binding.Map(), jc.DeepEquals, initial)
}

func (s *bindingsMockSuite) TestNewBindingsByName(c *gc.C) {
	defer s.setup(c).Finish()
	s.expectIDsByName()
	s.expectNamesByID()
	initial := map[string]string{
		"db":      "two",
		"testing": "42",
		"empty":   "",
	}

	binding, err := state.NewBindings(s.endpointBinding, initial)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(binding, gc.NotNil)

	expected := map[string]string{
		"db":      "2",
		"testing": "5",
		"empty":   "",
	}
	c.Logf("%+v", binding.Map())
	c.Assert(binding.Map(), jc.DeepEquals, expected)
}

func (s *bindingsMockSuite) TestNewBindingsInvalid(c *gc.C) {
	defer s.setup(c).Finish()
	s.expectIDsByName()
	s.expectNamesByID()
	initial := map[string]string{
		"db":      "2",
		"testing": "three",
		"empty":   "",
	}

	binding, err := state.NewBindings(s.endpointBinding, initial)
	c.Assert(err, jc.Satisfies, errors.IsNotValid)
	c.Assert(binding, gc.IsNil)
}

func (s *bindingsMockSuite) expectNamesByID() {
	n2i := map[string]string{
		network.DefaultSpaceId: network.DefaultSpaceName,
		"1":                    "one",
		"2":                    "two",
		"3":                    "three",
		"4":                    "four",
		"5":                    "42",
	}
	s.endpointBinding.EXPECT().SpaceNamesByID().Return(n2i, nil).AnyTimes()
}

func (s *bindingsMockSuite) expectIDsByName() {
	i2n := map[string]string{
		network.DefaultSpaceName: network.DefaultSpaceId,
		"one":                    "1",
		"two":                    "2",
		"three":                  "3",
		"four":                   "4",
		"42":                     "5",
	}
	s.endpointBinding.EXPECT().SpaceIDsByName().Return(i2n, nil).AnyTimes()
}

func (s *bindingsMockSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.endpointBinding = mocks.NewMockEndpointBinding(ctrl)
	return ctrl
}
