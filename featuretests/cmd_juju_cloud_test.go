// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/commands"
	jujutesting "github.com/juju/juju/juju/testing"
)

type CmdCloudSuite struct {
	jujutesting.JujuConnSuite
}

func (s *CmdCloudSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
}

func (s *CmdCloudSuite) TestAddCloudDuplicate(c *gc.C) {
	s.Home.AddFiles(c, testing.TestFile{
		Name: ".local/share/clouds.yaml",
		Data: `
clouds:
  testcloud:
    type: ec2
    description: Dummy Test Cloud Metadata
    auth-types: [ access-key ]
`,
	})

	ctx, err := s.run(c, "add-cloud", "testcloud", "-c", "kontroll", "--force")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), jc.Contains, `Cloud "testcloud" added to controller "kontroll".`)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "")

	ctx, err = s.run(c, "add-cloud", "testcloud", "-c", "kontroll", "--force")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), jc.Contains, `Cloud "testcloud" already exists on the controller "kontroll".`)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "")
}

func (s *CmdCloudSuite) run(c *gc.C, args ...string) (*cmd.Context, error) {
	context := cmdtesting.Context(c)
	command := commands.NewJujuCommand(context, "")
	c.Assert(cmdtesting.InitCommand(command, args), jc.ErrorIsNil)
	err := command.Run(context)
	_, _ = loggo.RemoveWriter("warning")
	return context, err
}
