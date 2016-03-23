// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud_test

import (
	"strings"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/juju/cloud"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	"github.com/juju/juju/testing"
)

type removeCredentialSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&removeCredentialSuite{})

func (s *removeCredentialSuite) TestBadArgs(c *gc.C) {
	cmd := cloud.NewRemoveCredentialCommand()
	_, err := testing.RunCommand(c, cmd)
	c.Assert(err, gc.ErrorMatches, "Usage: juju remove-credential <cloud-name> <credential-name>")
	_, err = testing.RunCommand(c, cmd, "cloud", "credential", "extra")
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["extra"\]`)
}

func (s *removeCredentialSuite) TestMissingCredential(c *gc.C) {
	store := &jujuclienttesting.MemStore{
		Credentials: map[string]jujucloud.CloudCredential{
			"aws": {
				AuthCredentials: map[string]jujucloud.Credential{
					"my-credential": jujucloud.NewCredential(jujucloud.AccessKeyAuthType, nil),
				},
			},
		},
	}
	cmd := cloud.NewRemoveCredentialCommandForTest(store)
	ctx, err := testing.RunCommand(c, cmd, "aws", "foo")
	c.Assert(err, jc.ErrorIsNil)
	output := testing.Stderr(ctx)
	output = strings.Replace(output, "\n", "", -1)
	c.Assert(output, gc.Equals, `No credential called "foo" exists for cloud "aws"`)
}

func (s *removeCredentialSuite) TestBadCloudName(c *gc.C) {
	cmd := cloud.NewRemoveCredentialCommandForTest(jujuclienttesting.NewMemStore())
	ctx, err := testing.RunCommand(c, cmd, "somecloud", "foo")
	c.Assert(err, jc.ErrorIsNil)
	output := testing.Stderr(ctx)
	output = strings.Replace(output, "\n", "", -1)
	c.Assert(output, gc.Equals, `No credentials exist for cloud "somecloud"`)
}

func (s *removeCredentialSuite) TestRemove(c *gc.C) {
	store := &jujuclienttesting.MemStore{
		Credentials: map[string]jujucloud.CloudCredential{
			"aws": {
				AuthCredentials: map[string]jujucloud.Credential{
					"my-credential":      jujucloud.NewCredential(jujucloud.AccessKeyAuthType, nil),
					"another-credential": jujucloud.NewCredential(jujucloud.AccessKeyAuthType, nil),
				},
			},
		},
	}
	cmd := cloud.NewRemoveCredentialCommandForTest(store)
	ctx, err := testing.RunCommand(c, cmd, "aws", "my-credential")
	c.Assert(err, jc.ErrorIsNil)
	output := testing.Stderr(ctx)
	output = strings.Replace(output, "\n", "", -1)
	c.Assert(output, gc.Equals, `Credential "my-credential" for cloud "aws" has been deleted.`)
	_, stillThere := store.Credentials["aws"].AuthCredentials["my-credential"]
	c.Assert(stillThere, jc.IsFalse)
	c.Assert(store.Credentials["aws"].AuthCredentials, gc.HasLen, 1)
}
