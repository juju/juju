// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/api/agent/uniter"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type settingsSuite struct {
	coretesting.BaseSuite
}

func TestSettingsSuite(t *testing.T) {
	tc.Run(t, &settingsSuite{})
}

func (s *settingsSuite) TestNewSettingsAndMap(c *tc.C) {
	// Make sure newSettings accepts nil settings.
	settings := uniter.NewSettings("blah", "foo", nil)
	theMap := settings.Map()
	c.Assert(theMap, tc.NotNil)
	c.Assert(theMap, tc.HasLen, 0)

	// And also accepts a populated map, and returns a converted map.
	rawSettings := params.Settings{
		"some":  "settings",
		"other": "stuff",
	}
	settings = uniter.NewSettings("blah", "foo", rawSettings)
	theMap = settings.Map()
	c.Assert(theMap, tc.DeepEquals, rawSettings)
}

func (s *settingsSuite) TestSet(c *tc.C) {
	settings := uniter.NewSettings("blah", "foo", nil)

	settings.Set("foo", "bar")
	c.Assert(settings.Map(), tc.DeepEquals, params.Settings{
		"foo": "bar",
	})
	settings.Set("foo", "qaz")
	c.Assert(settings.Map(), tc.DeepEquals, params.Settings{
		"foo": "qaz",
	})
	settings.Set("bar", "Cheers")
	c.Assert(settings.Map(), tc.DeepEquals, params.Settings{
		"foo": "qaz",
		"bar": "Cheers",
	})
}

func (s *settingsSuite) TestDelete(c *tc.C) {
	settings := uniter.NewSettings("blah", "foo", nil)

	settings.Set("foo", "qaz")
	settings.Set("abc", "tink")
	settings.Set("bar", "tonk")
	c.Assert(settings.Map(), tc.DeepEquals, params.Settings{
		"foo": "qaz",
		"abc": "tink",
		"bar": "tonk",
	})
	settings.Delete("abc")
	c.Assert(settings.Map(), tc.DeepEquals, params.Settings{
		"foo": "qaz",
		"bar": "tonk",
	})
	settings.Delete("bar")
	c.Assert(settings.Map(), tc.DeepEquals, params.Settings{
		"foo": "qaz",
	})
	settings.Set("abc", "123")
	c.Assert(settings.Map(), tc.DeepEquals, params.Settings{
		"foo": "qaz",
		"abc": "123",
	})
	settings.Delete("missing")
	c.Assert(settings.Map(), tc.DeepEquals, params.Settings{
		"foo": "qaz",
		"abc": "123",
	})
}
