// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type bootstrapSuite struct{}

var _ = gc.Suite(&bootstrapSuite{})

func (_ *bootstrapSuite) TestBootstrapModelDefaults(c *gc.C) {
	provider := ModelDefaultsProvider(
		map[string]any{
			"foo":     "default",
			"default": "some value",
		},
		map[string]any{
			"foo":        "controller",
			"controller": "some value",
		},
		map[string]any{
			"foo":    "region",
			"region": "some value",
		},
	)

	defaults, err := provider.ModelDefaults(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(defaults["foo"].Value, gc.Equals, "region")
	c.Assert(defaults["foo"].Source, gc.Equals, "region")
	c.Assert(defaults["default"].Value, gc.Equals, "some value")
	c.Assert(defaults["default"].Source, gc.Equals, "default")
	c.Assert(defaults["controller"].Value, gc.Equals, "some value")
	c.Assert(defaults["controller"].Source, gc.Equals, "controller")
	c.Assert(defaults["region"].Value, gc.Equals, "some value")
	c.Assert(defaults["region"].Source, gc.Equals, "region")
}
