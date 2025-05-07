// Copyright 2014 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package featureflag

import (
	"github.com/juju/tc"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
)

type flagSuite struct {
	testing.IsolationSuite
}

var _ = tc.Suite(&flagSuite{})

func (s *flagSuite) TestEmpty(c *tc.C) {
	s.PatchEnvironment("JUJU_TESTING_FEATURE", "")
	SetFlagsFromEnvironment("JUJU_TESTING_FEATURE")
	c.Assert(All(), tc.HasLen, 0)
	c.Assert(AsEnvironmentValue(), tc.Equals, "")
	c.Assert(String(), tc.Equals, "")
}

func (s *flagSuite) TestParsing(c *tc.C) {
	s.PatchEnvironment("JUJU_TESTING_FEATURE", "MAGIC, test, space ")
	s.PatchEnvironment("JUJU_TESTING_FEATURE2", "magic2")
	SetFlagsFromEnvironment("JUJU_TESTING_FEATURE", "JUJU_TESTING_FEATURE2")
	c.Assert(All(), jc.SameContents, []string{"magic", "space", "test", "magic2"})
	c.Assert(AsEnvironmentValue(), tc.Equals, "magic,magic2,space,test")
	c.Assert(String(), tc.Equals, `"magic", "magic2", "space", "test"`)
}

func (s *flagSuite) TestEnabled(c *tc.C) {
	c.Assert(Enabled(""), jc.IsTrue)
	c.Assert(Enabled(" "), jc.IsTrue)
	c.Assert(Enabled("magic"), jc.IsFalse)
	c.Assert(Enabled("magic2"), jc.IsFalse)

	s.PatchEnvironment("JUJU_TESTING_FEATURE", "MAGIC")
	s.PatchEnvironment("JUJU_TESTING_FEATURE2", "MAGIC2")
	SetFlagsFromEnvironment("JUJU_TESTING_FEATURE", "JUJU_TESTING_FEATURE2")

	c.Assert(Enabled("magic"), jc.IsTrue)
	c.Assert(Enabled("Magic"), jc.IsTrue)
	c.Assert(Enabled(" MAGIC "), jc.IsTrue)
	c.Assert(Enabled("magic2"), jc.IsTrue)
	c.Assert(Enabled("Magic2"), jc.IsTrue)
	c.Assert(Enabled(" MAGIC2 "), jc.IsTrue)
}
