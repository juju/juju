// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud_test

import (
	"fmt"
	"strings"
	stdtesting "testing"

	"github.com/juju/tc"

	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/juju/cloud"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	_ "github.com/juju/juju/internal/provider/lxd"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/jujuclient"
)

type defaultCredentialSuite struct {
	testing.FakeJujuXDGDataHomeSuite
}

func TestDefaultCredentialSuite(t *stdtesting.T) {
	tc.Run(t, &defaultCredentialSuite{})
}

func (s *defaultCredentialSuite) TestBadArgs(c *tc.C) {
	cmd := cloud.NewSetDefaultCredentialCommand()
	_, err := cmdtesting.RunCommand(c, cmd)
	c.Assert(err, tc.ErrorMatches, `Usage: juju default-credential <cloud-name> \[<credential-name>\]`)
	_, err = cmdtesting.RunCommand(c, cmd, "cloud", "credential", "extra")
	c.Assert(err, tc.ErrorMatches, `unrecognized args: \["extra"\]`)
}

func (s *defaultCredentialSuite) TestBadCredential(c *tc.C) {
	cmd := cloud.NewSetDefaultCredentialCommand()
	_, err := cmdtesting.RunCommand(c, cmd, "aws", "foo")
	c.Assert(err, tc.ErrorMatches, `credential "foo" for cloud aws not valid`)
}

func (s *defaultCredentialSuite) TestBadCloudName(c *tc.C) {
	cmd := cloud.NewSetDefaultCredentialCommand()
	_, err := cmdtesting.RunCommand(c, cmd, "somecloud", "us-west-1")
	c.Assert(err, tc.ErrorMatches, `cloud somecloud not valid`)
}

func (s *defaultCredentialSuite) assertSetDefaultCredential(c *tc.C, cloudName string) {
	store := jujuclient.NewMemStore()
	store.Credentials[cloudName] = jujucloud.CloudCredential{
		AuthCredentials: map[string]jujucloud.Credential{
			"my-sekrets": {},
		},
	}
	cmd := cloud.NewSetDefaultCredentialCommandForTest(store)
	ctx, err := cmdtesting.RunCommand(c, cmd, cloudName, "my-sekrets")
	c.Assert(err, tc.ErrorIsNil)
	output := cmdtesting.Stderr(ctx)
	output = strings.Replace(output, "\n", "", -1)
	c.Assert(output, tc.Equals, fmt.Sprintf(`Local credential "my-sekrets" is set to be default for %q for this client.`, cloudName))
	c.Assert(store.Credentials[cloudName].DefaultCredential, tc.Equals, "my-sekrets")
}

func (s *defaultCredentialSuite) TestSetDefaultCredential(c *tc.C) {
	s.assertSetDefaultCredential(c, "aws")
}

func (s *defaultCredentialSuite) TestSetDefaultCredentialBuiltIn(c *tc.C) {
	s.assertSetDefaultCredential(c, "localhost")
}

func (s *defaultCredentialSuite) TestReadDefaultCredential(c *tc.C) {
	cloudName := "aws"
	store := jujuclient.NewMemStore()
	store.Credentials[cloudName] = jujucloud.CloudCredential{
		DefaultCredential: "my-sekrets",
	}
	cmd := cloud.NewSetDefaultCredentialCommandForTest(store)
	ctx, err := cmdtesting.RunCommand(c, cmd, cloudName)
	c.Assert(err, tc.ErrorIsNil)
	output := cmdtesting.Stderr(ctx)
	output = strings.Replace(output, "\n", "", -1)
	c.Assert(output, tc.Equals, fmt.Sprintf(`Default credential for cloud %q is "my-sekrets" on this client.`, cloudName))
}

func (s *defaultCredentialSuite) TestReadDefaultCredentialNoneSet(c *tc.C) {
	cloudName := "aws"
	store := jujuclient.NewMemStore()
	store.Credentials[cloudName] = jujucloud.CloudCredential{}
	cmd := cloud.NewSetDefaultCredentialCommandForTest(store)
	ctx, err := cmdtesting.RunCommand(c, cmd, cloudName)
	c.Assert(err, tc.ErrorIsNil)
	output := cmdtesting.Stderr(ctx)
	output = strings.Replace(output, "\n", "", -1)
	c.Assert(output, tc.Equals, fmt.Sprintf(`Default credential for cloud %q is not set on this client.`, cloudName))
}

func (s *defaultCredentialSuite) TestResetDefaultCredential(c *tc.C) {
	cloudName := "aws"
	store := jujuclient.NewMemStore()
	store.Credentials[cloudName] = jujucloud.CloudCredential{
		DefaultCredential: "my-sekrets",
	}
	cmd := cloud.NewSetDefaultCredentialCommandForTest(store)
	ctx, err := cmdtesting.RunCommand(c, cmd, cloudName, "--reset")
	c.Assert(err, tc.ErrorIsNil)
	output := cmdtesting.Stderr(ctx)
	output = strings.Replace(output, "\n", "", -1)
	c.Assert(output, tc.Equals, fmt.Sprintf(`Default credential for cloud %q is no longer set on this client.`, cloudName))
	c.Assert(store.Credentials[cloudName].DefaultCredential, tc.Equals, "")
}
