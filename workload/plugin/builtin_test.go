// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package plugin_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/workload"
	"github.com/juju/juju/workload/plugin"
	"github.com/juju/juju/workload/plugin/docker"
)

type builtinSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&builtinSuite{})

func (s *builtinSuite) TestFindBuiltinOkay(c *gc.C) {
	known := map[string]workload.Plugin{
		"docker": &docker.Plugin{},
	}
	for name, expected := range known {
		c.Logf("trying %q", name)
		plugin, err := plugin.FindBuiltin(name)
		c.Assert(err, jc.ErrorIsNil)

		c.Check(plugin, gc.FitsTypeOf, expected)
	}
}

func (s *builtinSuite) TestFindBuiltinNotFound(c *gc.C) {
	_, err := plugin.FindBuiltin("not-a-plugin")

	c.Check(err, jc.Satisfies, errors.IsNotFound)
}
