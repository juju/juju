// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/domain/modeldefaults"
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
	c.Assert(defaults, jc.DeepEquals, modeldefaults.Defaults{
		"foo": modeldefaults.DefaultAttributeValue{
			Default:    "default",
			Controller: "controller",
			Region:     "region",
		},
		"default": modeldefaults.DefaultAttributeValue{
			Default: "some value",
		},
		"controller": modeldefaults.DefaultAttributeValue{
			Controller: "some value",
		},
		"region": modeldefaults.DefaultAttributeValue{
			Region: "some value",
		},
	})
}
