// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network_test

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/network"
)

var _ = gc.Suite(&bindingsMockSuite{})

type bindingsMockSuite struct {
	testing.IsolationSuite
}

func (s *bindingsMockSuite) TestMapBindingsWithSpaceNames(c *gc.C) {
	infos := s.expectedSpaceInfos()

	initial := map[string]string{
		"db":      "2",
		"testing": "3",
		"empty":   network.AlphaSpaceId,
	}

	withSpaceNames, err := network.MapBindingsWithSpaceNames(initial, infos)
	c.Assert(err, jc.ErrorIsNil)

	expected := map[string]string{
		"db":      "two",
		"testing": "three",
		"empty":   network.AlphaSpaceName,
	}
	c.Check(withSpaceNames, jc.DeepEquals, expected)
}

func (s *bindingsMockSuite) TestMapBindingsWithSpaceNamesWithNoLookup(c *gc.C) {
	initial := map[string]string{
		"db":      "2",
		"testing": "3",
		"empty":   network.AlphaSpaceId,
	}

	_, err := network.MapBindingsWithSpaceNames(initial, nil)
	c.Assert(err, jc.ErrorIs, coreerrors.NotValid)
}

func (s *bindingsMockSuite) TestMapBindingsWithSpaceNamesWithNoBindings(c *gc.C) {
	initial := map[string]string{}

	withSpaceNames, err := network.MapBindingsWithSpaceNames(initial, make(network.SpaceInfos, 0))
	c.Assert(err, jc.ErrorIsNil)
	c.Check(withSpaceNames, gc.HasLen, 0)
}

func (s *bindingsMockSuite) TestMapBindingsWithSpaceNamesWithEmptyBindings(c *gc.C) {
	initial := map[string]string{
		"db":      "2",
		"testing": "3",
		"empty":   network.AlphaSpaceId,
	}

	_, err := network.MapBindingsWithSpaceNames(initial, make(network.SpaceInfos, 0))
	c.Assert(err, jc.ErrorIs, coreerrors.NotValid)
}

func (s *bindingsMockSuite) expectedSpaceInfos() network.SpaceInfos {
	return network.SpaceInfos{
		{ID: network.AlphaSpaceId, Name: network.AlphaSpaceName},
		{ID: "1", Name: "one"},
		{ID: "2", Name: "two"},
		{ID: "3", Name: "three"},
		{ID: "4", Name: "four"},
		{ID: "5", Name: "42"},
	}
}
