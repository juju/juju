// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils_test

import (
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/mongo/utils"
)

type dataCleansingSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&dataCleansingSuite{})

func (s *dataCleansingSuite) TestEscapeKeys_EscapesPeriods(c *gc.C) {
	before := map[string]interface{}{
		"a.b": "c",
	}
	after := utils.EscapeKeys(before)

	c.Check(after, gc.DeepEquals, map[string]interface{}{
		"a" + "\uff0e" + "b": "c",
	})
}

func (s *dataCleansingSuite) TestEscapeKeys_EscapesDollarSigns(c *gc.C) {
	before := map[string]interface{}{
		"$a": "c",
	}
	after := utils.EscapeKeys(before)

	c.Check(after, gc.DeepEquals, map[string]interface{}{
		"\uff04" + "a": "c",
	})
}

func (s *dataCleansingSuite) TestEscapeKeys_RecursivelyEscapes(c *gc.C) {
	before := map[string]interface{}{
		"$a": "c",
		"b": map[string]interface{}{
			"$foo.bar": "baz",
		},
	}
	after := utils.EscapeKeys(before)

	c.Check(after, gc.DeepEquals, map[string]interface{}{
		"\uff04" + "a": "c",
		"b": map[string]interface{}{
			"\uff04" + "foo" + "\uff0e" + "bar": "baz",
		},
	})
}

func (s *dataCleansingSuite) TestUnescapeKey_UnescapesPeriods(c *gc.C) {
	before := map[string]interface{}{
		"a" + "\uff0e" + "b": "c",
	}
	after := utils.UnescapeKeys(before)

	c.Check(after, gc.DeepEquals, map[string]interface{}{
		"a.b": "c",
	})
}

func (s *dataCleansingSuite) TestUnescapeKeys_UnescapesDollarSigns(c *gc.C) {
	before := map[string]interface{}{
		"\uff04" + "a": "c",
	}
	after := utils.UnescapeKeys(before)

	c.Check(after, gc.DeepEquals, map[string]interface{}{
		"$a": "c",
	})
}

func (s *dataCleansingSuite) TestUnescapeKeys_RecursivelyUnescapes(c *gc.C) {
	before := map[string]interface{}{
		"\uff04" + "a": "c",
		"b": map[string]interface{}{
			"\uff04" + "foo" + "\uff0e" + "bar": "baz",
		},
	}
	after := utils.UnescapeKeys(before)

	c.Check(after, gc.DeepEquals, map[string]interface{}{
		"$a": "c",
		"b": map[string]interface{}{
			"$foo.bar": "baz",
		},
	})
}

func (s *dataCleansingSuite) TestEscapeKey_EscapesPeriods(c *gc.C) {
	c.Check("a"+"\uff0e"+"b", gc.Equals, utils.EscapeKey("a.b"))
}

func (s *dataCleansingSuite) TestEscapeKey_EscapesDollarSigns(c *gc.C) {
	c.Check("\uff04"+"a", gc.Equals, utils.EscapeKey("$a"))
}

func (s *dataCleansingSuite) TestUnescapeKey_UnescapesPeriod(c *gc.C) {
	c.Check(utils.UnescapeKey("a"+"\uff0e"+"b"), gc.Equals, "a.b")
}

func (s *dataCleansingSuite) TestUnescapeKey_UnescapesDollarSigns(c *gc.C) {
	c.Check(utils.UnescapeKey("\uff04"+"a"), gc.Equals, "$a")
}
