// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package charm

import (
	"strings"
	stdtesting "testing"

	"github.com/juju/tc"
)

type VersionSuite struct{}

func TestVersionSuite(t *stdtesting.T) { tc.Run(t, &VersionSuite{}) }
func (s *VersionSuite) TestReadVersion(c *tc.C) {
	specs := []struct {
		version string
		expect  string
	}{
		{"7215482", "7215482"},
		{"revision-id: foo@bar.com-20131222180823-abcdefg", "foo@bar.com-20131222180823-abcdefg"},
	}
	for i, t := range specs {
		c.Logf("test %d", i)
		v, err := readVersion(strings.NewReader(t.version))
		c.Check(err, tc.IsNil)
		c.Assert(v, tc.Equals, t.expect)
	}
}
