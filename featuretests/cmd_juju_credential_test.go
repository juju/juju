// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	apicloud "github.com/juju/juju/api/client/cloud"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/juju/commands"
	"github.com/juju/juju/core/instance"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/jujuclient"
	_ "github.com/juju/juju/provider/openstack"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/testing/factory"
)

type CmdCredentialSuite struct {
	jujutesting.JujuConnSuite
}

const (
	user       = "fred"
	pass       = "secret"
	tenantName = "hrm"
)

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
    type: openstack
    description: Dummy Test Cloud Metadata
    auth-types: [ userpass ]
`,
	})

	store := jujuclient.NewFileClientStore()
	_ = store.UpdateCredential("dummy", cloud.CloudCredential{
		AuthCredentials: map[string]cloud.Credential{
			"cred": cloud.NewCredential(cloud.UserPassAuthType, map[string]string{
				"username":    user,
				"password":    pass,
				"tenant-name": tenantName,
			}),
		},
	})

	_, err := s.run(c, "show-credential", "dummy", "cred", "-c", "kontroll")
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.run(c, "update-credential", "dummy", "cred", "-c", "kontroll")
	c.Assert(err, gc.Equals, cmd.ErrSilent)
	c.Assert(c.GetTestLog(), jc.Contains, `ERROR juju.cmd.juju.cloud finalizing "cred" credential for cloud "dummy": unknown key "tenant-name" (value "hrm")`)
	_ = store.UpdateCredential("dummy", cloud.CloudCredential{
		AuthCredentials: map[string]cloud.Credential{
			"cred": cloud.NewCredential(cloud.UserPassAuthType, map[string]string{
				"username": user,
				"password": pass,
			}),
		},
	})
	ctx, err := s.run(c, "update-credential", "dummy", "cred", "-c", "kontroll", "--client")
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, `
Local client was updated successfully with provided credential information.
Credential valid for:
  controller
Controller credential "cred" for user "admin" for cloud "dummy" on controller "kontroll" updated.
For more information, see ‘juju show-credential dummy cred’.
`[1:])
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "")

	client := apicloud.NewClient(s.OpenControllerAPI(c))
	defer func() { _ = client.Close() }()

	tag := names.NewCloudCredentialTag("dummy/admin@local/cred")
	result, err := client.Credentials(tag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, []params.CloudCredentialResult{
		{Result: &params.CloudCredential{
			AuthType: "userpass",
			Attributes: map[string]string{
				"username": user,
			},
			Redacted: []string{"password"},
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
    type: openstack
    description: Dummy Test Cloud Metadata
    auth-types: [ userpass ]
`,
	})

	store := jujuclient.NewFileClientStore()
	store.UpdateCredential("dummy", cloud.CloudCredential{
		AuthCredentials: map[string]cloud.Credential{
			"newcred": cloud.NewCredential(cloud.UserPassAuthType, map[string]string{
				"username":    user,
				"password":    pass,
				"tenant-name": tenantName,
			}),
		},
	})
	newCredentialTag := names.NewCloudCredentialTag("dummy/admin@local/newcred")

	client := apicloud.NewClient(s.OpenControllerAPI(c))
	defer func() { _ = client.Close() }()

	// Check new credential does not exist on the controller.
	result, err := client.Credentials(newCredentialTag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.HasLen, 1)
	c.Assert(result[0].Error.Error(), gc.DeepEquals, "credential \"newcred\" not found")

	// Check model references original credential.
	originalCredentialTag, set := s.Model.CloudCredentialTag()
	c.Assert(set, jc.IsTrue)
	c.Assert(originalCredentialTag.String(), jc.DeepEquals, "cloudcred-dummy_admin_cred")

	ctx, err := s.run(c, "set-credential", "dummy", "newcred")
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, `
Did not find credential remotely. Looking locally...
Uploading local credential to the controller.
Changed cloud credential on model "controller" to "newcred".
`[1:])
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "")
	c.Assert(err, jc.ErrorIsNil)

	// Check crdential was uploaded to the controller.
	result2, err := client.Credentials(newCredentialTag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result2, jc.DeepEquals, []params.CloudCredentialResult{
		{Result: &params.CloudCredential{
			AuthType: "userpass",
			Attributes: map[string]string{
				"username":    user,
				"tenant-name": tenantName,
			},
			Redacted: []string{"password"},
		}},
	})
	// Check model reference was updated to a new credential.
	err = s.Model.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	updatedCredentialTag, set := s.Model.CloudCredentialTag()
	c.Assert(set, jc.IsTrue)
	c.Assert(updatedCredentialTag.String(), jc.DeepEquals, "cloudcred-dummy_admin_newcred")

}

func (s *CmdCredentialSuite) TestShowCredentialCommandAll(c *gc.C) {
	ctx, err := s.run(c, "show-credential", "-c", "kontroll")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "")
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, `
controller-credentials:
  dummy:
    cred:
      content:
        auth-type: userpass
        validity-check: valid
        username: dummy
      models:
        controller: admin
`[1:])
}

func (s *CmdCredentialSuite) TestShowCredentialCommandWithName(c *gc.C) {
	ctx, err := s.run(c, "show-credential", "dummy", "cred", "--show-secrets", "-c", "kontroll")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, `
controller-credentials:
  dummy:
    cred:
      content:
        auth-type: userpass
        validity-check: valid
        password: secret
        username: dummy
      models:
        controller: admin
`[1:])
}

func (s *CmdCredentialSuite) run(c *gc.C, args ...string) (*cmd.Context, error) {
	context := cmdtesting.Context(c)
	command := commands.NewJujuCommand(context, "")
	c.Assert(cmdtesting.InitCommand(command, args), jc.ErrorIsNil)
	err := command.Run(context)
	_, _ = loggo.RemoveWriter("warning")
	return context, err
}
