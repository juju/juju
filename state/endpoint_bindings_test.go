// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/charm/v7"
	"github.com/juju/errors"
	"github.com/juju/juju/state/mocks"
	"github.com/juju/testing"
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
	s.clientSpace, err = s.State.AddSpace("client", "", nil, true)
	c.Assert(err, jc.ErrorIsNil)
	s.appsSpace, err = s.State.AddSpace("apps", "", nil, true)
	c.Assert(err, jc.ErrorIsNil)
	s.dbSpace, err = s.State.AddSpace("db", "", nil, true)
	c.Assert(err, jc.ErrorIsNil)
	s.barbSpace, err = s.State.AddSpace("barb3", "", nil, true)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *bindingsSuite) TestMergeBindings(c *gc.C) {
	// The test cases below are not exhaustive, but just check basic
	// functionality. Most of the logic is tested by calling application.SetCharm()
	// in various ways.

	for i, test := range []struct {
		about                    string
		mergeWithMap, currentMap map[string]string
		meta                     *charm.Meta
		updated                  map[string]string
		modified                 bool
	}{{
		about:        "defaults used when both mergeWithMap and currentMap are nil",
		mergeWithMap: nil,
		currentMap:   nil,
		meta:         s.oldMeta,
		updated:      s.copyMap(s.oldDefaults),
		modified:     true,
	}, {
		about:        "currentMap overrides defaults, mergeWithMap is nil",
		mergeWithMap: nil,
		currentMap: map[string]string{
			"foo1": s.clientSpace.Id(),
			"self": s.dbSpace.Id(),
		},
		meta: s.oldMeta,
		updated: map[string]string{
			"":          network.AlphaSpaceId,
			"foo1":      s.clientSpace.Id(),
			"bar1":      network.AlphaSpaceId,
			"self":      s.dbSpace.Id(),
			"one-extra": network.AlphaSpaceId,
		},
		modified: true,
	}, {
		about: "currentMap overrides defaults, mergeWithMap overrides currentMap",
		mergeWithMap: map[string]string{
			"":          network.AlphaSpaceId,
			"foo1":      network.AlphaSpaceId,
			"self":      s.dbSpace.Id(),
			"bar1":      s.clientSpace.Id(),
			"one-extra": s.appsSpace.Id(),
		},
		currentMap: map[string]string{
			"foo1": s.clientSpace.Id(),
			"bar1": s.dbSpace.Id(),
		},
		meta: s.oldMeta,
		updated: map[string]string{
			"":          network.AlphaSpaceId,
			"foo1":      network.AlphaSpaceId,
			"bar1":      s.clientSpace.Id(),
			"self":      s.dbSpace.Id(),
			"one-extra": s.appsSpace.Id(),
		},
		modified: true,
	}, {
		about: "mergeWithMap overrides defaults, currentMap is nil",
		mergeWithMap: map[string]string{
			"self": s.dbSpace.Id(),
		},
		currentMap: nil,
		meta:       s.oldMeta,
		updated: map[string]string{
			"":          network.AlphaSpaceId,
			"foo1":      network.AlphaSpaceId,
			"bar1":      network.AlphaSpaceId,
			"self":      s.dbSpace.Id(),
			"one-extra": network.AlphaSpaceId,
		},
		modified: true,
	}, {
		about:        "obsolete entries in currentMap missing in defaults are removed",
		mergeWithMap: nil,
		currentMap: map[string]string{
			"any-old-thing": s.dbSpace.Id(),
			"self":          s.dbSpace.Id(),
			"one-extra":     s.appsSpace.Id(),
		},
		meta: s.oldMeta,
		updated: map[string]string{
			"":          network.AlphaSpaceId,
			"foo1":      network.AlphaSpaceId,
			"bar1":      network.AlphaSpaceId,
			"self":      s.dbSpace.Id(),
			"one-extra": s.appsSpace.Id(),
		},
		modified: true,
	}, {
		about: "new endpoints use defaults unless specified in mergeWithMap, existing ones are kept",
		mergeWithMap: map[string]string{
			"foo2": s.dbSpace.Id(),
			"me":   s.clientSpace.Id(),
			"bar3": s.dbSpace.Id(),
		},
		currentMap: s.copyMap(s.oldDefaults),
		meta:       s.newMeta,
		updated: map[string]string{
			"":     network.AlphaSpaceId,
			"foo1": network.AlphaSpaceId,
			"foo2": s.dbSpace.Id(),
			"bar2": network.AlphaSpaceId,
			"bar3": s.dbSpace.Id(),
			"self": network.AlphaSpaceId,
			"me":   s.clientSpace.Id(),
		},
		modified: true,
	}, {
		about: "new default supersedes old default",
		mergeWithMap: map[string]string{
			"":     s.clientSpace.Name(),
			"bar3": s.barbSpace.Name(),
		},
		currentMap: map[string]string{
			"":          s.appsSpace.Id(),
			"foo1":      s.appsSpace.Id(),
			"bar1":      s.dbSpace.Id(),
			"self":      network.AlphaSpaceId,
			"one-extra": s.barbSpace.Id(),
		},
		meta: s.newMeta,
		updated: map[string]string{
			"":     s.clientSpace.Id(),
			"foo1": s.appsSpace.Id(),
			"foo2": s.clientSpace.Id(),
			"bar2": s.clientSpace.Id(),
			"bar3": s.barbSpace.Id(),
			"self": network.AlphaSpaceId,
			"me":   s.clientSpace.Id(),
		},
		modified: true,
	}, {
		about: "new map one change",
		mergeWithMap: map[string]string{
			"self": s.barbSpace.Name(),
		},
		currentMap: map[string]string{
			"":          s.appsSpace.Id(),
			"foo1":      s.appsSpace.Id(),
			"bar1":      s.dbSpace.Id(),
			"self":      network.AlphaSpaceId,
			"one-extra": s.clientSpace.Id(),
		},
		meta: s.oldMeta,
		updated: map[string]string{
			"":          s.appsSpace.Id(),
			"foo1":      s.appsSpace.Id(),
			"bar1":      s.dbSpace.Id(),
			"self":      s.barbSpace.Id(),
			"one-extra": s.clientSpace.Id(),
		},
		modified: true,
	}, {
		about:        "old unchanged but different key",
		mergeWithMap: nil,
		currentMap: map[string]string{
			"":          s.appsSpace.Id(),
			"bar1":      s.dbSpace.Id(),
			"self":      network.AlphaSpaceId,
			"lost":      s.clientSpace.Id(),
			"one-extra": s.clientSpace.Id(),
		},
		meta: s.oldMeta,
		updated: map[string]string{
			"":          s.appsSpace.Id(),
			"foo1":      s.appsSpace.Id(),
			"bar1":      s.dbSpace.Id(),
			"self":      network.AlphaSpaceId,
			"one-extra": s.clientSpace.Id(),
		},
		modified: true,
	}} {
		c.Logf("test #%d: %s", i, test.about)
		b, err := state.NewBindings(s.State, test.currentMap)
		c.Assert(err, jc.ErrorIsNil)
		isModified, err := b.Merge(test.mergeWithMap, test.meta)
		c.Check(err, jc.ErrorIsNil)
		c.Check(b.Map(), jc.DeepEquals, test.updated)
		c.Check(isModified, gc.Equals, test.modified)
	}
}

func (s *bindingsSuite) TestMergeWithModelConfigNonDefaultSpace(c *gc.C) {
	err := s.Model.UpdateModelConfig(map[string]interface{}{"default-space": s.appsSpace.Name()}, nil)
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

func (s *bindingsSuite) TestDefaultEndpointBindingSpaceNotDefault(c *gc.C) {
	err := s.Model.UpdateModelConfig(map[string]interface{}{"default-space": s.clientSpace.Name()}, nil)
	c.Assert(err, jc.ErrorIsNil)
	id, err := s.State.DefaultEndpointBindingSpace()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(id, gc.Equals, s.clientSpace.Id())
}

func (s *bindingsSuite) TestDefaultEndpointBindingSpaceDefault(c *gc.C) {
	id, err := s.State.DefaultEndpointBindingSpace()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(id, gc.Equals, network.AlphaSpaceId)
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
	s.expectAllSpaceInfos()

	binding, err := state.NewBindings(s.endpointBinding, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(binding, gc.NotNil)
	c.Assert(binding.Map(), gc.DeepEquals, map[string]string{})
}

func (s *bindingsMockSuite) TestNewBindingsByID(c *gc.C) {
	defer s.setup(c).Finish()
	s.expectAllSpaceInfos()
	initial := map[string]string{
		"db":      "2",
		"testing": "5",
		"empty":   network.AlphaSpaceId,
	}

	binding, err := state.NewBindings(s.endpointBinding, initial)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(binding, gc.NotNil)

	c.Assert(binding.Map(), jc.DeepEquals, initial)
}

func (s *bindingsMockSuite) TestNewBindingsByName(c *gc.C) {
	defer s.setup(c).Finish()
	s.expectAllSpaceInfos()
	initial := map[string]string{
		"db":      "two",
		"testing": "42",
		"empty":   network.AlphaSpaceName,
	}

	binding, err := state.NewBindings(s.endpointBinding, initial)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(binding, gc.NotNil)

	expected := map[string]string{
		"db":      "2",
		"testing": "5",
		"empty":   network.AlphaSpaceId,
	}
	c.Logf("%+v", binding.Map())
	c.Assert(binding.Map(), jc.DeepEquals, expected)
}

func (s *bindingsMockSuite) TestNewBindingsNotFound(c *gc.C) {
	defer s.setup(c).Finish()
	s.expectAllSpaceInfos()
	initial := map[string]string{
		"db":      "2",
		"testing": "three",
		"empty":   network.AlphaSpaceId,
	}

	binding, err := state.NewBindings(s.endpointBinding, initial)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(binding, gc.IsNil)
}

func (s *bindingsMockSuite) TestMapWithSpaceNames(c *gc.C) {
	defer s.setup(c).Finish()
	s.expectAllSpaceInfos()
	initial := map[string]string{
		"db":      "2",
		"testing": "3",
		"empty":   network.AlphaSpaceId,
	}

	binding, err := state.NewBindings(s.endpointBinding, initial)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(binding, gc.NotNil)
	withSpaceNames, err := binding.MapWithSpaceNames()
	c.Assert(err, jc.ErrorIsNil)

	expected := map[string]string{
		"db":      "two",
		"testing": "three",
		"empty":   network.AlphaSpaceName,
	}
	c.Assert(withSpaceNames, jc.DeepEquals, expected)
}

func (s *bindingsMockSuite) expectAllSpaceInfos() {
	infos := network.SpaceInfos{
		{ID: network.AlphaSpaceId, Name: network.AlphaSpaceName},
		{ID: "1", Name: "one"},
		{ID: "2", Name: "two"},
		{ID: "3", Name: "three"},
		{ID: "4", Name: "four"},
		{ID: "5", Name: "42"},
	}
	s.endpointBinding.EXPECT().AllSpaceInfos().Return(infos, nil).AnyTimes()
}
func (s *bindingsMockSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.endpointBinding = mocks.NewMockEndpointBinding(ctrl)
	return ctrl
}
