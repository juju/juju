// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network_test

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/network"
)

type spaceSuite struct {
	testing.IsolationSuite

	spaces network.SpaceInfos
}

var _ = gc.Suite(&spaceSuite{})

func (s *spaceSuite) SetUpTest(c *gc.C) {
	s.spaces = network.SpaceInfos{
		{ID: "1", Name: "space1"},
		{ID: "2", Name: "space2"},
		{ID: "3", Name: "space3"},
	}
}

func (s *spaceSuite) TestString(c *gc.C) {
	result := s.spaces.String()
	c.Assert(result, gc.Equals, `"space1", "space2", "space3"`)
}

func (s *spaceSuite) TestGetByName(c *gc.C) {
	c.Assert(s.spaces.GetByName("space1"), gc.NotNil)
	c.Assert(s.spaces.GetByName("space666"), gc.IsNil)
}

func (s *spaceSuite) TestGetByID(c *gc.C) {
	c.Assert(s.spaces.GetByID("1"), gc.NotNil)
	c.Assert(s.spaces.GetByID("999"), gc.IsNil)
}

func (s *spaceSuite) TestContainsName(c *gc.C) {
	c.Assert(s.spaces.ContainsName("space3"), jc.IsTrue)
	c.Assert(s.spaces.ContainsName("space666"), jc.IsFalse)
}

func (s *spaceSuite) TestDifference(c *gc.C) {
	infos := network.SpaceInfos{
		{ID: "2", Name: "space2"},
		{ID: "3", Name: "space3"},
	}
	result := s.spaces.Difference(infos)
	c.Assert(result, gc.DeepEquals, network.SpaceInfos{{ID: "1", Name: "space1"}})
}

func (s *spaceSuite) TestDifferenceNoDiff(c *gc.C) {
	infos := network.SpaceInfos{
		{ID: "1", Name: "space1"},
		{ID: "2", Name: "space2"},
		{ID: "3", Name: "space3"},
	}
	result := s.spaces.Difference(infos)
	c.Assert(result, gc.DeepEquals, network.SpaceInfos{})
}
