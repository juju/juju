// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh_test

import (
	"io/ioutil"

	gc "launchpad.net/gocheck"

	jc "launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/testing/testbase"
	"launchpad.net/juju-core/utils/ssh"
)

type SystemIdentitySuite struct {
	testbase.LoggingSuite
}

var _ = gc.Suite(&SystemIdentitySuite{})

func (s *SystemIdentitySuite) TestWrite(c *gc.C) {

	dataDir := c.MkDir()
	err := ssh.WriteSystemIdentity(dataDir, "private-key")
	c.Assert(err, gc.IsNil)

	filename := ssh.SystemIdentityFilename(dataDir)
	c.Assert(filename, jc.IsNonEmptyFile)

	content, err := ioutil.ReadFile(filename)
	c.Assert(err, gc.IsNil)
	c.Assert(string(content), gc.Equals, "private-key")
}
