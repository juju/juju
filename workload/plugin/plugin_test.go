// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package plugin_test

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/workload"
	"github.com/juju/juju/workload/plugin"
)

type pluginSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&pluginSuite{})

func (s *pluginSuite) TestFindOkay(c *gc.C) {
	known := map[string]workload.Plugin{
		"docker": &plugin.DockerPlugin{},
	}
	for name, expected := range known {
		c.Logf("trying %q", name)
		plugin, err := plugin.Find(name)
		c.Assert(err, jc.ErrorIsNil)

		c.Check(plugin, gc.FitsTypeOf, expected)
	}
}
