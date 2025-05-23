// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network_test

import (
	"testing"

	"github.com/juju/tc"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/internal/testhelpers"
)

type bindingsMockSuite struct {
	testhelpers.IsolationSuite
}

func TestBindingsMockSuite(t *testing.T) {
	tc.Run(t, &bindingsMockSuite{})
}

func (s *bindingsMockSuite) TestMapBindingsWithSpaceNames(c *tc.C) {
	infos := s.expectedSpaceInfos()

	initial := map[string]string{
		"db":      "2",
		"testing": "3",
		"empty":   network.AlphaSpaceId,
	}

	withSpaceNames, err := network.MapBindingsWithSpaceNames(initial, infos)
	c.Assert(err, tc.ErrorIsNil)

	expected := map[string]string{
		"db":      "two",
		"testing": "three",
		"empty":   network.AlphaSpaceName,
	}
	c.Check(withSpaceNames, tc.DeepEquals, expected)
}

func (s *bindingsMockSuite) TestMapBindingsWithSpaceNamesWithNoLookup(c *tc.C) {
	initial := map[string]string{
		"db":      "2",
		"testing": "3",
		"empty":   network.AlphaSpaceId,
	}

	_, err := network.MapBindingsWithSpaceNames(initial, nil)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *bindingsMockSuite) TestMapBindingsWithSpaceNamesWithNoBindings(c *tc.C) {
	initial := map[string]string{}

	withSpaceNames, err := network.MapBindingsWithSpaceNames(initial, make(network.SpaceInfos, 0))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(withSpaceNames, tc.HasLen, 0)
}

func (s *bindingsMockSuite) TestMapBindingsWithSpaceNamesWithEmptyBindings(c *tc.C) {
	initial := map[string]string{
		"db":      "2",
		"testing": "3",
		"empty":   network.AlphaSpaceId,
	}

	_, err := network.MapBindingsWithSpaceNames(initial, make(network.SpaceInfos, 0))
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
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
