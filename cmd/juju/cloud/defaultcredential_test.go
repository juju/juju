// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud_test

import (
	"fmt"
	"strings"

	"github.com/juju/cmd/v4/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/juju/cloud"
	_ "github.com/juju/juju/internal/provider/lxd"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/testing"
)

type defaultCredentialSuite struct {
	testing.FakeJujuXDGDataHomeSuite
}

var _ = gc.Suite(&defaultCredentialSuite{})

func (s *defaultCredentialSuite) TestBadArgs(c *gc.C) {
	cmd := cloud.NewSetDefaultCredentialCommand()
	_, err := cmdtesting.RunCommand(c, cmd)
	c.Assert(err, gc.ErrorMatches, `Usage: juju default-credential <cloud-name> \[<credential-name>\]`)
	_, err = cmdtesting.RunCommand(c, cmd, "cloud", "credential", "extra")
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["extra"\]`)
}

func (s *defaultCredentialSuite) TestBadCredential(c *gc.C) {
	cmd := cloud.NewSetDefaultCredentialCommand()
	_, err := cmdtesting.RunCommand(c, cmd, "aws", "foo")
	c.Assert(err, gc.ErrorMatches, `credential "foo" for cloud aws not valid`)
}

func (s *defaultCredentialSuite) TestBadCloudName(c *gc.C) {
	cmd := cloud.NewSetDefaultCredentialCommand()
	_, err := cmdtesting.RunCommand(c, cmd, "somecloud", "us-west-1")
	c.Assert(err, gc.ErrorMatches, `cloud somecloud not valid`)
}

func (s *defaultCredentialSuite) assertSetDefaultCredential(c *gc.C, cloudName string) {
	store := jujuclient.NewMemStore()
	store.Credentials[cloudName] = jujucloud.CloudCredential{
		AuthCredentials: map[string]jujucloud.Credential{
			"my-sekrets": {},
		},
	}
	cmd := cloud.NewSetDefaultCredentialCommandForTest(store)
	ctx, err := cmdtesting.RunCommand(c, cmd, cloudName, "my-sekrets")
	c.Assert(err, jc.ErrorIsNil)
	output := cmdtesting.Stderr(ctx)
	output = strings.Replace(output, "\n", "", -1)
	c.Assert(output, gc.Equals, fmt.Sprintf(`Local credential "my-sekrets" is set to be default for %q for this client.`, cloudName))
	c.Assert(store.Credentials[cloudName].DefaultCredential, gc.Equals, "my-sekrets")
}

func (s *defaultCredentialSuite) TestSetDefaultCredential(c *gc.C) {
	s.assertSetDefaultCredential(c, "aws")
}

func (s *defaultCredentialSuite) TestSetDefaultCredentialBuiltIn(c *gc.C) {
	s.assertSetDefaultCredential(c, "localhost")
}

func (s *defaultCredentialSuite) TestReadDefaultCredential(c *gc.C) {
	cloudName := "aws"
	store := jujuclient.NewMemStore()
	store.Credentials[cloudName] = jujucloud.CloudCredential{
		DefaultCredential: "my-sekrets",
	}
	cmd := cloud.NewSetDefaultCredentialCommandForTest(store)
	ctx, err := cmdtesting.RunCommand(c, cmd, cloudName)
	c.Assert(err, jc.ErrorIsNil)
	output := cmdtesting.Stderr(ctx)
	output = strings.Replace(output, "\n", "", -1)
	c.Assert(output, gc.Equals, fmt.Sprintf(`Default credential for cloud %q is "my-sekrets" on this client.`, cloudName))
}

func (s *defaultCredentialSuite) TestReadDefaultCredentialNoneSet(c *gc.C) {
	cloudName := "aws"
	store := jujuclient.NewMemStore()
	store.Credentials[cloudName] = jujucloud.CloudCredential{}
	cmd := cloud.NewSetDefaultCredentialCommandForTest(store)
	ctx, err := cmdtesting.RunCommand(c, cmd, cloudName)
	c.Assert(err, jc.ErrorIsNil)
	output := cmdtesting.Stderr(ctx)
	output = strings.Replace(output, "\n", "", -1)
	c.Assert(output, gc.Equals, fmt.Sprintf(`Default credential for cloud %q is not set on this client.`, cloudName))
}

func (s *defaultCredentialSuite) TestResetDefaultCredential(c *gc.C) {
	cloudName := "aws"
	store := jujuclient.NewMemStore()
	store.Credentials[cloudName] = jujucloud.CloudCredential{
		DefaultCredential: "my-sekrets",
	}
	cmd := cloud.NewSetDefaultCredentialCommandForTest(store)
	ctx, err := cmdtesting.RunCommand(c, cmd, cloudName, "--reset")
	c.Assert(err, jc.ErrorIsNil)
	output := cmdtesting.Stderr(ctx)
	output = strings.Replace(output, "\n", "", -1)
	c.Assert(output, gc.Equals, fmt.Sprintf(`Default credential for cloud %q is no longer set on this client.`, cloudName))
	c.Assert(store.Credentials[cloudName].DefaultCredential, gc.Equals, "")
}
