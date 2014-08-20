// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"os"
	"path/filepath"

	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/state/backups"
	"github.com/juju/juju/testing"
)

var getFilesToBackup = *backups.GetFilesToBackup

var _ = gc.Suite(&sourcesSuite{})

type sourcesSuite struct {
	testing.BaseSuite
	root string
}

func (s *sourcesSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.root = c.MkDir()

	// Prep the fake FS.
	mkdir := func(path string) string {
		dirname := filepath.Join(s.root, path)
		os.MkdirAll(dirname, 0777)
		return dirname
	}
	touch := func(dirname, name string) {
		path := filepath.Join(dirname, name)
		file, err := os.Create(path)
		c.Assert(err, gc.IsNil)
		file.Close()
	}

	dirname := mkdir("/var/lib/juju")
	touch(dirname, "system-identity")
	touch(dirname, "nonce.txt")
	touch(dirname, "server.pem")
	touch(dirname, "shared-secret")
	mkdir("/var/lib/juju/tools")

	dirname = mkdir("/var/lib/juju/agents")
	touch(dirname, "machine-0.conf")

	dirname = mkdir("/var/log/juju")
	touch(dirname, "all-machines.log")
	touch(dirname, "machine-0.log")

	dirname = mkdir("/etc/init")
	touch(dirname, "jujud-machine-0.conf")
	touch(dirname, "juju-db.conf")

	dirname = mkdir("/etc/rsyslog.d")
	touch(dirname, "spam-juju.conf")

	dirname = mkdir("/home/ubuntu/.ssh")
	touch(dirname, "authorized_keys")
}

func (s *sourcesSuite) TestGetFilesToBackup(c *gc.C) {
	files, err := getFilesToBackup(s.root)
	c.Assert(err, gc.IsNil)

	c.Check(files, jc.SameContents, []string{
		filepath.Join(s.root, "/etc/init/juju-db.conf"),
		filepath.Join(s.root, "/etc/init/jujud-machine-0.conf"),
		filepath.Join(s.root, "/etc/rsyslog.d/spam-juju.conf"),
		filepath.Join(s.root, "/home/ubuntu/.ssh/authorized_keys"),
		filepath.Join(s.root, "/var/lib/juju/agents/machine-0.conf"),
		filepath.Join(s.root, "/var/lib/juju/nonce.txt"),
		filepath.Join(s.root, "/var/lib/juju/server.pem"),
		filepath.Join(s.root, "/var/lib/juju/shared-secret"),
		filepath.Join(s.root, "/var/lib/juju/system-identity"),
		filepath.Join(s.root, "/var/lib/juju/tools"),
		filepath.Join(s.root, "/var/log/juju/all-machines.log"),
		filepath.Join(s.root, "/var/log/juju/machine-0.log"),
	})
}
