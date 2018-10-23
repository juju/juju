// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	apicloud "github.com/juju/juju/api/cloud"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/juju/commands"
	"github.com/juju/juju/instance"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/testing/factory"
)

type CmdCredentialSuite struct {
	jujutesting.JujuConnSuite
}

func (s *CmdCredentialSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	// JujuConnSuite sets up a cloud instance "localhost".
	// When credential is updated, we check that we can still see
	// all the instances on the provider using the new credential content.
	// This check also verifies that all instances have corresponding machine records
	// in our system and vice versa.
	// This is why we need to add a machine that corresponds to a "localhost" instance,
	// i.e. just making the suite model valid in credential's eyes.
	s.Factory.MakeMachine(c, &factory.MachineParams{
		InstanceId: instance.Id("localhost"),
	})
}

func (s *CmdCredentialSuite) TestUpdateCredentialCommand(c *gc.C) {
	//Add dummy cloud to cloud metadata
	s.Home.AddFiles(c, testing.TestFile{
		Name: ".local/share/clouds.yaml",
		Data: `
clouds:
  dummy:
    type: oracle
    description: Dummy Test Cloud Metadata
    auth-types: [ userpass ]
`,
	})

	store := jujuclient.NewFileClientStore()
	store.UpdateCredential("dummy", cloud.CloudCredential{
		AuthCredentials: map[string]cloud.Credential{
			"cred": cloud.NewCredential(cloud.UserPassAuthType, map[string]string{"username": "fred", "password": "secret", "identity-domain": "domain"}),
		},
	})
	ctx, err := s.run(c, "update-credential", "dummy", "cred")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, `
Credential valid for:
  controller
Controller credential "cred" for user "admin" on cloud "dummy" updated.
For more information, see ‘juju show-credential dummy cred’.
`[1:])
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "")

	client := apicloud.NewClient(s.OpenControllerAPI(c))
	defer client.Close()

	tag := names.NewCloudCredentialTag("dummy/admin@local/cred")
	result, err := client.Credentials(tag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, []params.CloudCredentialResult{
		{Result: &params.CloudCredential{
			AuthType:   "userpass",
			Attributes: map[string]string{"username": "fred", "identity-domain": "domain"},
			Redacted:   []string{"password"},
		}},
	})
}

func (s *CmdCredentialSuite) TestSetModelCredentialCommand(c *gc.C) {
	//Add dummy cloud to cloud metadata
	s.Home.AddFiles(c, testing.TestFile{
		Name: ".local/share/clouds.yaml",
		Data: `
clouds:
  dummy:
    type: oracle
    description: Dummy Test Cloud Metadata
    auth-types: [ userpass ]
`,
	})

	store := jujuclient.NewFileClientStore()
	store.UpdateCredential("dummy", cloud.CloudCredential{
		AuthCredentials: map[string]cloud.Credential{
			"newcred": cloud.NewCredential(cloud.UserPassAuthType, map[string]string{"username": "fred", "password": "secret", "identity-domain": "domain"}),
		},
	})
	newCredentialTag := names.NewCloudCredentialTag("dummy/admin@local/newcred")

	client := apicloud.NewClient(s.OpenControllerAPI(c))
	defer client.Close()

	// Check new credential does not exist on the controller.
	result, err := client.Credentials(newCredentialTag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.HasLen, 1)
	c.Assert(result[0].Error.Error(), gc.DeepEquals, "credential \"newcred\" not found")

	// Check model references original credential.
	originalCredentialTag, set := s.Model.CloudCredential()
	c.Assert(set, jc.IsTrue)
	c.Assert(originalCredentialTag.String(), jc.DeepEquals, "cloudcred-dummy_admin_cred")

	ctx, err := s.run(c, "set-credential", "dummy", "newcred")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, `
Did not find credential remotely. Looking locally...
Uploading local credential to the controller.
Changed cloud credential on model "controller" to "newcred".
`[1:])
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "")

	// Check crdential was uploaded to the controller.
	result2, err := client.Credentials(newCredentialTag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result2, jc.DeepEquals, []params.CloudCredentialResult{
		{Result: &params.CloudCredential{
			AuthType:   "userpass",
			Attributes: map[string]string{"username": "fred", "identity-domain": "domain"},
			Redacted:   []string{"password"},
		}},
	})
	// Check model reference was updated to a new credential.
	err = s.Model.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	updatedCredentialTag, set := s.Model.CloudCredential()
	c.Assert(set, jc.IsTrue)
	c.Assert(updatedCredentialTag.String(), jc.DeepEquals, "cloudcred-dummy_admin_newcred")

}

func (s *CmdCredentialSuite) TestShowCredentialCommandAll(c *gc.C) {
	ctx, err := s.run(c, "show-credential")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "")
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, `
controller-credentials:
  dummy:
    cred:
      content:
        auth-type: userpass
        username: dummy
      models:
        controller: admin
`[1:])
}

func (s *CmdCredentialSuite) TestShowCredentialCommandWithName(c *gc.C) {
	ctx, err := s.run(c, "show-credential", "dummy", "cred", "--show-secrets")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "")
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, `
controller-credentials:
  dummy:
    cred:
      content:
        auth-type: userpass
        password: secret
        username: dummy
      models:
        controller: admin
`[1:])
}

func (s *CmdCredentialSuite) run(c *gc.C, args ...string) (*cmd.Context, error) {
	context := cmdtesting.Context(c)
	command := commands.NewJujuCommand(context)
	c.Assert(cmdtesting.InitCommand(command, args), jc.ErrorIsNil)
	err := command.Run(context)
	loggo.RemoveWriter("warning")
	return context, err
}
