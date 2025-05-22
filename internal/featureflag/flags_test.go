// Copyright 2014 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package featureflag

import (
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/testhelpers"
)

type flagSuite struct {
	testhelpers.IsolationSuite
}

func TestFlagSuite(t *stdtesting.T) {
	tc.Run(t, &flagSuite{})
}

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
	c.Assert(All(), tc.SameContents, []string{"magic", "space", "test", "magic2"})
	c.Assert(AsEnvironmentValue(), tc.Equals, "magic,magic2,space,test")
	c.Assert(String(), tc.Equals, `"magic", "magic2", "space", "test"`)
}

func (s *flagSuite) TestEnabled(c *tc.C) {
	c.Assert(Enabled(""), tc.IsTrue)
	c.Assert(Enabled(" "), tc.IsTrue)
	c.Assert(Enabled("magic"), tc.IsFalse)
	c.Assert(Enabled("magic2"), tc.IsFalse)

	s.PatchEnvironment("JUJU_TESTING_FEATURE", "MAGIC")
	s.PatchEnvironment("JUJU_TESTING_FEATURE2", "MAGIC2")
	SetFlagsFromEnvironment("JUJU_TESTING_FEATURE", "JUJU_TESTING_FEATURE2")

	c.Assert(Enabled("magic"), tc.IsTrue)
	c.Assert(Enabled("Magic"), tc.IsTrue)
	c.Assert(Enabled(" MAGIC "), tc.IsTrue)
	c.Assert(Enabled("magic2"), tc.IsTrue)
	c.Assert(Enabled("Magic2"), tc.IsTrue)
	c.Assert(Enabled(" MAGIC2 "), tc.IsTrue)
}
