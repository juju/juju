// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/caas/kubernetes/provider"
	"github.com/juju/juju/testing"
)

type templateSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&templateSuite{})

func (t *templateSuite) TestToYaml(c *gc.C) {
	in := struct {
		Command []string `yaml:"command,omitempty"`
	}{
		Command: []string{"sh", "-c", `
set -ex
echo "do some stuff here for gitlab container"
`[1:]},
	}
	out, err := provider.ToYaml(in)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out, jc.DeepEquals, `
command:
- sh
- -c
- |
  set -ex
  echo "do some stuff here for gitlab container"
`[1:])
}

func (t *templateSuite) TestIndent(c *gc.C) {
	out := provider.Indent(6, `
line 1
line 2
line 3`[1:])
	c.Assert(out, jc.DeepEquals, `
      line 1
      line 2
      line 3
`[1:])

	out = provider.Indent(8, `
line 1
line 2
line 3`[1:])
	c.Assert(out, jc.DeepEquals, `
        line 1
        line 2
        line 3
`[1:])
}
