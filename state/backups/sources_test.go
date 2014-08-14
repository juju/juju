// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/state/backups"
	"github.com/juju/juju/testing"
)

var getFilesToBackup = *backups.GetFilesToBackup

var _ = gc.Suite(&sourcesSuite{})

type sourcesSuite struct {
	testing.BaseSuite
}

func (s *sourcesSuite) TestGetFilesToBackup(c *gc.C) {
	files, err := getFilesToBackup()
	c.Assert(err, gc.IsNil)

	c.Check(files, jc.SameContents, []string{
		"/etc/init/juju-db.conf",
		"/home/ubuntu/.ssh/authorized_keys",
		"/var/lib/juju/nonce.txt",
		"/var/lib/juju/server.pem",
		"/var/lib/juju/shared-secret",
		"/var/lib/juju/system-identity",
		"/var/lib/juju/tools",
		"/var/log/juju/all-machines.log",
		"/var/log/juju/machine-0.log",
	})
}
