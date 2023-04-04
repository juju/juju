// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"os"

	"github.com/juju/cmd/v3/cmdtesting"
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
		s.AddTestingApplication(c, app, ch)
	}
}

func (s *CmdRelationSuite) TestAddRelationSuccess(c *gc.C) {
	runCommandExpectSuccess(c, "integrate", s.apps...)
}

func (s *CmdRelationSuite) TestAddRelationSuccessOnAlreadyExists(c *gc.C) {
	runCommandExpectSuccess(c, "integrate", s.apps...)
	context, err := runCommand(c, append([]string{"integrate"}, s.apps...)...)
	c.Assert(err, gc.NotNil)
	// TODO: (jam) 2022-06-29 the error messages need to be updated for 'integrations'
	c.Check(cmdtesting.Stderr(context), jc.Contains, `ERROR cannot add relation "wordpress:db mysql:server"
relation wordpress:db mysql:server (already exists): 

Use 'juju status --relations' to view the current relations.
`)
}

func (s *CmdRelationSuite) TestRemoveRelationSuccess(c *gc.C) {
	runCommandExpectSuccess(c, "integrate", s.apps...)
	runCommandExpectSuccess(c, "remove-relation", s.apps...)
}

func (s *CmdRelationSuite) TestRemoveRelationFail(c *gc.C) {
	runCommandExpectFailure(c, "remove-relation", `ERROR relation matching "wordpress mysql" not found (not found)`, s.apps...)
}
