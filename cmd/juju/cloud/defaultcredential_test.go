// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud_test

import (
	"strings"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/juju/cloud"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	"github.com/juju/juju/testing"
)

type defaultCredentialSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&defaultCredentialSuite{})

func (s *defaultCredentialSuite) SetUpTest(c *gc.C) {
	origHome := osenv.SetJujuXDGDataHome(c.MkDir())
	s.AddCleanup(func(*gc.C) { osenv.SetJujuXDGDataHome(origHome) })
}

func (s *defaultCredentialSuite) TestBadArgs(c *gc.C) {
	cmd := cloud.NewSetDefaultCredentialCommand()
	_, err := testing.RunCommand(c, cmd)
	c.Assert(err, gc.ErrorMatches, "Usage: juju set-default-credential <cloud-name> <credential-name>")
	_, err = testing.RunCommand(c, cmd, "cloud", "credential", "extra")
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["extra"\]`)
}

func (s *defaultCredentialSuite) TestBadCredential(c *gc.C) {
	cmd := cloud.NewSetDefaultCredentialCommand()
	_, err := testing.RunCommand(c, cmd, "aws", "foo")
	c.Assert(err, gc.ErrorMatches, `credential "foo" for cloud aws not valid`)
}

func (s *defaultCredentialSuite) TestBadCloudName(c *gc.C) {
	cmd := cloud.NewSetDefaultCredentialCommand()
	_, err := testing.RunCommand(c, cmd, "somecloud", "us-west-1")
	c.Assert(err, gc.ErrorMatches, `cloud somecloud not found`)
}

func (s *defaultCredentialSuite) TestSetDefaultCredential(c *gc.C) {
	store := jujuclienttesting.NewMemStore()
	store.Credentials["aws"] = jujucloud.CloudCredential{
		AuthCredentials: map[string]jujucloud.Credential{
			"my-sekrets": {},
		},
	}
	cmd := cloud.NewSetDefaultCredentialCommandForTest(store)
	ctx, err := testing.RunCommand(c, cmd, "aws", "my-sekrets")
	c.Assert(err, jc.ErrorIsNil)
	output := testing.Stderr(ctx)
	output = strings.Replace(output, "\n", "", -1)
	c.Assert(output, gc.Equals, `Default credential for aws set to "my-sekrets".`)
	c.Assert(store.Credentials["aws"].DefaultCredential, gc.Equals, "my-sekrets")
}
