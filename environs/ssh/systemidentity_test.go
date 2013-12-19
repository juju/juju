// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh_test

import (
	"io/ioutil"
	"path"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs/ssh"
	jc "launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/testing/testbase"
)

type SystemIdentitySuite struct {
	testbase.LoggingSuite
}

var _ = gc.Suite(&SystemIdentitySuite{})

func (s *SystemIdentitySuite) TestWrite(c *gc.C) {
	filename := path.Join(c.MkDir(), ssh.SystemIdentity)
	err := ssh.WriteSystemIdentity(filename, "private-key")
	c.Assert(err, gc.IsNil)

	c.Assert(filename, jc.IsNonEmptyFile)

	content, err := ioutil.ReadFile(filename)
	c.Assert(err, gc.IsNil)
	c.Assert(string(content), gc.Equals, "private-key")
}
