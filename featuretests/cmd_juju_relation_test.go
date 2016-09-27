// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"bytes"
	"os"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/juju/osenv"
	jujutesting "github.com/juju/juju/juju/testing"
)

type CmdRelationSuite struct {
	jujutesting.JujuConnSuite
	apps []string
}

func (s *CmdRelationSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	os.Setenv(osenv.JujuModelEnvKey, "")

	s.apps = []string{"wordpress", "mysql"}
	for _, app := range s.apps {
		ch := s.AddTestingCharm(c, app)
		s.AddTestingService(c, app, ch)
	}
}

func run(c *gc.C, command, expectedError string, args ...string) {
	cmdArgs := append([]string{command}, args...)
	context, err := runCommand(c, cmdArgs...)
	if expectedError == "" {
		c.Assert(err, jc.ErrorIsNil)
	} else {
		c.Assert(err, gc.ErrorMatches, "cmd: error out silently")
		buff, ok := context.GetStderr().(*bytes.Buffer)
		if !ok {
			c.Log("could not get stderr content")
			c.Fail()
		}
		c.Assert(buff.String(), jc.Contains, expectedError)
	}
}

func (s *CmdRelationSuite) TestAddRelationSuccess(c *gc.C) {
	run(c, "add-relation", "", s.apps...)
}

func (s *CmdRelationSuite) TestAddRelationFail(c *gc.C) {
	run(c, "add-relation", "", s.apps...)
	run(c, "add-relation", `cannot add relation "wordpress:db mysql:server": relation already exists`, s.apps...)
}

func (s *CmdRelationSuite) TestRemoveRelationSuccess(c *gc.C) {
	run(c, "add-relation", "", s.apps...)
	run(c, "remove-relation", "", s.apps...)
}

func (s *CmdRelationSuite) TestRemoveRelationFail(c *gc.C) {
	run(c, "remove-relation", `relation "wordpress:db mysql:server" not found`, s.apps...)
}
