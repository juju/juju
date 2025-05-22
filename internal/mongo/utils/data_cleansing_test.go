// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils_test

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/mongo/utils"
	"github.com/juju/juju/internal/testhelpers"
)

type dataCleansingSuite struct {
	testhelpers.IsolationSuite
}

func TestDataCleansingSuite(t *testing.T) {
	tc.Run(t, &dataCleansingSuite{})
}

func (s *dataCleansingSuite) TestEscapeKeys_EscapesPeriods(c *tc.C) {
	before := map[string]interface{}{
		"a.b": "c",
	}
	after := utils.EscapeKeys(before)

	c.Check(after, tc.DeepEquals, map[string]interface{}{
		"a" + "\uff0e" + "b": "c",
	})
}

func (s *dataCleansingSuite) TestEscapeKeys_EscapesDollarSigns(c *tc.C) {
	before := map[string]interface{}{
		"$a": "c",
	}
	after := utils.EscapeKeys(before)

	c.Check(after, tc.DeepEquals, map[string]interface{}{
		"\uff04" + "a": "c",
	})
}

func (s *dataCleansingSuite) TestEscapeKeys_RecursivelyEscapes(c *tc.C) {
	before := map[string]interface{}{
		"$a": "c",
		"b": map[string]interface{}{
			"$foo.bar": "baz",
		},
	}
	after := utils.EscapeKeys(before)

	c.Check(after, tc.DeepEquals, map[string]interface{}{
		"\uff04" + "a": "c",
		"b": map[string]interface{}{
			"\uff04" + "foo" + "\uff0e" + "bar": "baz",
		},
	})
}

func (s *dataCleansingSuite) TestUnescapeKey_UnescapesPeriods(c *tc.C) {
	before := map[string]interface{}{
		"a" + "\uff0e" + "b": "c",
	}
	after := utils.UnescapeKeys(before)

	c.Check(after, tc.DeepEquals, map[string]interface{}{
		"a.b": "c",
	})
}

func (s *dataCleansingSuite) TestUnescapeKeys_UnescapesDollarSigns(c *tc.C) {
	before := map[string]interface{}{
		"\uff04" + "a": "c",
	}
	after := utils.UnescapeKeys(before)

	c.Check(after, tc.DeepEquals, map[string]interface{}{
		"$a": "c",
	})
}

func (s *dataCleansingSuite) TestUnescapeKeys_RecursivelyUnescapes(c *tc.C) {
	before := map[string]interface{}{
		"\uff04" + "a": "c",
		"b": map[string]interface{}{
			"\uff04" + "foo" + "\uff0e" + "bar": "baz",
		},
	}
	after := utils.UnescapeKeys(before)

	c.Check(after, tc.DeepEquals, map[string]interface{}{
		"$a": "c",
		"b": map[string]interface{}{
			"$foo.bar": "baz",
		},
	})
}

func (s *dataCleansingSuite) TestEscapeKey_EscapesPeriods(c *tc.C) {
	c.Check("a"+"\uff0e"+"b", tc.Equals, utils.EscapeKey("a.b"))
}

func (s *dataCleansingSuite) TestEscapeKey_EscapesDollarSigns(c *tc.C) {
	c.Check("\uff04"+"a", tc.Equals, utils.EscapeKey("$a"))
}

func (s *dataCleansingSuite) TestUnescapeKey_UnescapesPeriod(c *tc.C) {
	c.Check(utils.UnescapeKey("a"+"\uff0e"+"b"), tc.Equals, "a.b")
}

func (s *dataCleansingSuite) TestUnescapeKey_UnescapesDollarSigns(c *tc.C) {
	c.Check(utils.UnescapeKey("\uff04"+"a"), tc.Equals, "$a")
}
