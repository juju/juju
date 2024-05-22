// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package charm_test

import (
	"strings"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/charm"
)

type VersionSuite struct{}

var _ = gc.Suite(&VersionSuite{})

func (s *VersionSuite) TestReadVersion(c *gc.C) {
	specs := []struct {
		version string
		expect  string
	}{
		{"7215482", "7215482"},
		{"revision-id: foo@bar.com-20131222180823-abcdefg", "foo@bar.com-20131222180823-abcdefg"},
	}
	for i, t := range specs {
		c.Logf("test %d", i)
		v, err := charm.ReadVersion(strings.NewReader(t.version))
		c.Check(err, gc.IsNil)
		c.Assert(v, gc.Equals, t.expect)
	}
}
