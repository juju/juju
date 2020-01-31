// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network_test

import (
	"github.com/juju/collections/set"
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/network"
)

type spaceSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&spaceSuite{})

func (*spaceSuite) TestString(c *gc.C) {
	result := network.SpaceInfos{
		{Name: "space1"},
		{Name: "space2"},
		{Name: "space3"},
	}.String()

	c.Assert(result, gc.Equals, "space1, space2, space3")
}

func (*spaceSuite) TestGetByName(c *gc.C) {
	spaces := network.SpaceInfos{
		{Name: "space1"},
		{Name: "space2"},
		{Name: "space3"},
	}

	c.Assert(spaces.GetByName("space1"), gc.NotNil)
	c.Assert(spaces.GetByName("space666"), gc.IsNil)
}

func (*spaceSuite) TestGetByID(c *gc.C) {
	spaces := network.SpaceInfos{
		{ID: "space1"},
		{ID: "space2"},
		{ID: "space3"},
	}

	c.Assert(spaces.GetByID("space1"), gc.NotNil)
	c.Assert(spaces.GetByID("space666"), gc.IsNil)
}

func (s *spaceSuite) TestConvertSpaceName(c *gc.C) {
	empty := set.Strings{}
	nameTests := []struct {
		name     string
		existing set.Strings
		expected string
	}{
		{"foo", empty, "foo"},
		{"foo1", empty, "foo1"},
		{"Foo Thing", empty, "foo-thing"},
		{"foo^9*//++!!!!", empty, "foo9"},
		{"--Foo", empty, "foo"},
		{"---^^&*()!", empty, "empty"},
		{" ", empty, "empty"},
		{"", empty, "empty"},
		{"foo\u2318", empty, "foo"},
		{"foo--", empty, "foo"},
		{"-foo--foo----bar-", empty, "foo-foo-bar"},
		{"foo-", set.NewStrings("foo", "bar", "baz"), "foo-2"},
		{"foo", set.NewStrings("foo", "foo-2"), "foo-3"},
		{"---", set.NewStrings("empty"), "empty-2"},
	}
	for _, test := range nameTests {
		result := network.ConvertSpaceName(test.name, test.existing)
		c.Check(result, gc.Equals, test.expected)
	}
}
