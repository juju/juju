// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package config_test

import (
	"os"
	"path/filepath"

	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/state/backups/config"
	"github.com/juju/juju/testing"
)

var _ = gc.Suite(&sourcesSuite{})

type sourcesSuite struct {
	testing.BaseSuite
	root   string
	paths  config.Paths
	dbInfo config.DBInfo
}

func (s *sourcesSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	root := c.MkDir()
	paths := config.ReRoot(&config.DefaultPaths, root)
	dbConnInfo := config.NewDBConnInfo(
		"some-host.com:8080",
		"awesome",
		"bad-pw",
	)
	dbInfo, err := config.NewDBInfo(dbConnInfo)
	c.Assert(err, gc.IsNil)
	s.root = root
	s.paths = paths
	s.dbInfo = dbInfo

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
	touch(dirname, "tools")
	touch(dirname, "system-identity")
	touch(dirname, "nonce.txt")
	touch(dirname, "server.pem")
	touch(dirname, "shared-secret")

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

func (s *sourcesSuite) TestBackupsConfigNewBackupsConfigOkay(c *gc.C) {
	conf, err := config.NewBackupsConfig(s.dbInfo, s.paths)
	c.Assert(err, gc.IsNil)
	dbInfo, paths := config.BackupsConfigValues(conf)

	c.Check(dbInfo, gc.DeepEquals, s.dbInfo)
	c.Check(paths, gc.DeepEquals, s.paths)
}

func (s *sourcesSuite) TestBackupsConfigNewBackupsConfigDefaults(c *gc.C) {
	conf, err := config.NewBackupsConfig(s.dbInfo, nil)
	c.Assert(err, gc.IsNil)
	dbInfo, paths := config.BackupsConfigValues(conf)
	expectedPaths := config.DefaultPaths

	c.Check(dbInfo, gc.DeepEquals, s.dbInfo)
	c.Check(paths, gc.DeepEquals, &expectedPaths)
}

func (s *sourcesSuite) TestBackupsConfigNewBackupsConfigMissingDBInfo(c *gc.C) {
	_, err := config.NewBackupsConfig(nil, s.paths)
	c.Check(err, gc.ErrorMatches, "missing dbInfo")
}

func (s *sourcesSuite) TestBackupsConfigNewBackupsConfigRaw(c *gc.C) {
	conf, err := config.NewBackupsConfigRaw(
		"some-host.com:8080",
		"awesome",
		"bad-pw",
		"spam",
		s.paths,
	)
	c.Assert(err, gc.IsNil)
	dbInfo, paths := config.BackupsConfigValues(conf)

	c.Check(dbInfo.ConnInfo().Address(), gc.Equals, "some-host.com:8080")
	c.Check(dbInfo.ConnInfo().Username(), gc.Equals, "awesome")
	c.Check(dbInfo.ConnInfo().Password(), gc.Equals, "bad-pw")
	c.Check(dbInfo.BinDir(), gc.Equals, "spam")
	c.Check(paths, gc.DeepEquals, s.paths)
}

func (s *sourcesSuite) TestBackupsConfigFilesToBackUpOkay(c *gc.C) {
	conf, err := config.NewBackupsConfigRaw("", "", "", "", s.paths)
	c.Assert(err, gc.IsNil)
	files, err := conf.FilesToBackUp()
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

func (s *sourcesSuite) TestBackupsConfigDBDumpOkay(c *gc.C) {
	dumpBinary := filepath.Join(s.root, "mongodump")
	file, err := os.Create(dumpBinary)
	c.Assert(err, gc.IsNil)
	file.Close()

	conf, err := config.NewBackupsConfigRaw(
		"some-host.com:8080",
		"awesome",
		"bad-pw",
		s.root,
		nil,
	)
	c.Assert(err, gc.IsNil)
	bin, args, err := conf.DBDump("/tmp/dbdump")

	c.Check(err, gc.IsNil)
	c.Check(bin, gc.Equals, dumpBinary)
	c.Check(args, gc.DeepEquals, []string{
		"--oplog",
		"--ssl",
		"--host", "some-host.com:8080",
		"--username", "awesome",
		"--password", "bad-pw",
		"--out", "/tmp/dbdump",
	})
}

func (s *sourcesSuite) TestBackupsConfigDBDumpMissing(c *gc.C) {
	conf, err := config.NewBackupsConfigRaw(
		"some-host.com:8080",
		"awesome",
		"bad-pw",
		"spam",
		nil,
	)
	c.Assert(err, gc.IsNil)
	_, _, err = conf.DBDump("/tmp/dbdump")
	c.Check(err, gc.ErrorMatches, `missing "spam/mongodump".*`)
}
