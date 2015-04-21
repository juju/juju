// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/system"
	"github.com/juju/juju/feature"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/testing"
)

type cmdSystemSuite struct {
	jujutesting.RepoSuite
}

func (s *cmdSystemSuite) SetUpTest(c *gc.C) {
	s.RepoSuite.SetUpTest(c)
	s.SetFeatureFlags(feature.JES)
}

func (s *cmdSystemSuite) TestEnvironmentShareCmdStack(c *gc.C) {
	context, err := testing.RunCommand(c, &system.ListCommand{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stdout(context), gc.Equals, "dummyenv\n")
}
