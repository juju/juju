// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh_test

import (
	gc "launchpad.net/gocheck"

	jc "launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/testing/testbase"
	"launchpad.net/juju-core/utils/ssh"
)

type GenerateSuite struct {
	testbase.LoggingSuite
}

var _ = gc.Suite(&GenerateSuite{})

func (s *GenerateSuite) TestGenerate(c *gc.C) {
	private, public, err := ssh.GenerateKey("some-comment")

	c.Check(err, gc.IsNil)
	c.Check(private, jc.HasPrefix, "-----BEGIN RSA PRIVATE KEY-----\n")
	c.Check(private, jc.HasSuffix, "-----END RSA PRIVATE KEY-----\n")
	c.Check(public, jc.HasPrefix, "ssh-rsa ")
	c.Check(public, jc.HasSuffix, " some-comment\n")
}
