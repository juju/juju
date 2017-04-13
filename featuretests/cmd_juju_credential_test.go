// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	apicloud "github.com/juju/juju/api/cloud"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/juju/commands"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/jujuclient"
)

type cmdCredentialSuite struct {
	jujutesting.JujuConnSuite
}

func (s *cmdCredentialSuite) run(c *gc.C, args ...string) *cmd.Context {
	context := cmdtesting.Context(c)
	command := commands.NewJujuCommand(context)
	c.Assert(cmdtesting.InitCommand(command, args), jc.ErrorIsNil)
	c.Assert(command.Run(context), jc.ErrorIsNil)
	loggo.RemoveWriter("warning")
	return context
}

func (s *cmdCredentialSuite) TestUpdateCredentialCommand(c *gc.C) {
	store := jujuclient.NewFileClientStore()
	store.UpdateCredential("dummy", cloud.CloudCredential{
		AuthCredentials: map[string]cloud.Credential{
			"mine": cloud.NewCredential(cloud.UserPassAuthType, map[string]string{"username": "fred", "password": "secret"}),
		},
	})
	s.run(c, "update-credential", "dummy", "mine")

	client := apicloud.NewClient(s.OpenControllerAPI(c))
	defer client.Close()

	tag := names.NewCloudCredentialTag("dummy/admin@local/mine")
	result, err := client.Credentials(tag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, []params.CloudCredentialResult{
		{Result: &params.CloudCredential{
			AuthType:   "userpass",
			Attributes: map[string]string{"username": "fred"},
			Redacted:   []string{"password"},
		}},
	})
}
