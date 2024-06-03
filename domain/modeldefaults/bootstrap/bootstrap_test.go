// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/domain/modeldefaults/state"
)

type bootstrapSuite struct{}

var _ = gc.Suite(&bootstrapSuite{})

func (_ *bootstrapSuite) TestBootstrapModelDefaults(c *gc.C) {
	provider := ModelDefaultsProvider(
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
	c.Check(err, jc.ErrorIsNil)
	c.Check(defaults["foo"].Value, gc.Equals, "region")
	c.Check(defaults["foo"].Source, gc.Equals, "region")
	c.Check(defaults["controller"].Value, gc.Equals, "some value")
	c.Check(defaults["controller"].Source, gc.Equals, "controller")
	c.Check(defaults["region"].Value, gc.Equals, "some value")
	c.Check(defaults["region"].Source, gc.Equals, "region")

	configDefaults := state.ConfigDefaults(context.Background())
	for k, v := range configDefaults {
		c.Check(defaults[k].Value, gc.Equals, v)
		c.Check(defaults[k].Source, gc.Equals, "default")
	}
}
