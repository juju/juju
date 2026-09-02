// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"os"
	"path/filepath"
	stdtesting "testing"

	"github.com/juju/collections/set"
	"github.com/juju/tc"

	"github.com/juju/juju/core/backups"
	"github.com/juju/juju/internal/testing"
)

type filesSuite struct {
	testing.BaseSuite
}

// relDataDir is the data dir relative to the root dir handed to
// GetFilesToBackUp. Tests always pass a root dir so that the optional
// /home/ubuntu/.ssh/authorized_keys lookup stays inside the test's own
// tree: on the host it may exist but be unreadable, which is fatal to
// GetFilesToBackUp and would make results depend on the test runner.
var relDataDir = filepath.Join("var", "lib", "juju")

func TestFilesSuite(t *stdtesting.T) {
	tc.Run(t, &filesSuite{})
}

func (s *filesSuite) writeFile(c *tc.C, path, content string) {
	err := os.MkdirAll(filepath.Dir(path), 0755)
	c.Assert(err, tc.ErrorIsNil)
	err = os.WriteFile(path, []byte(content), 0644)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *filesSuite) TestGetFilesToBackUpMissingObjectstore(c *tc.C) {
	rootDir := c.MkDir()
	dataDir := filepath.Join(rootDir, relDataDir)
	s.writeFile(c, filepath.Join(dataDir, "tools", "jujud"), "binary")

	_, err := backups.GetFilesToBackUp(rootDir,
		&backups.Paths{DataDir: relDataDir})
	c.Assert(err, tc.ErrorMatches,
		`cannot walk ".*objectstore.*`)
}

func (s *filesSuite) TestGetFilesToBackUpMissingTools(c *tc.C) {
	rootDir := c.MkDir()
	dataDir := filepath.Join(rootDir, relDataDir)
	err := os.MkdirAll(filepath.Join(dataDir, "objectstore"), 0755)
	c.Assert(err, tc.ErrorIsNil)

	_, err = backups.GetFilesToBackUp(rootDir,
		&backups.Paths{DataDir: relDataDir})
	c.Assert(err, tc.ErrorMatches,
		`cannot walk ".*tools.*`)
}

func (s *filesSuite) TestGetFilesToBackUp(c *tc.C) {
	rootDir := c.MkDir()
	dataDir := filepath.Join(rootDir, relDataDir)
	s.writeFile(c, filepath.Join(dataDir, "objectstore", "ns1", "blob"),
		"blob")
	s.writeFile(c, filepath.Join(dataDir, "tools", "jujud"), "binary")
	s.writeFile(c,
		filepath.Join(dataDir, "agents", "machine-0", "agent.conf"), "conf")
	s.writeFile(c, filepath.Join(dataDir, "init", "jujud-machine-0.conf"),
		"init conf")
	s.writeFile(c, filepath.Join(dataDir, "system-identity"), "ssh key")
	authKeys := filepath.Join(rootDir, "home", "ubuntu", ".ssh",
		"authorized_keys")
	s.writeFile(c, authKeys, "ssh-rsa key")

	// A symlink is collected, but the directory it sits in is not.
	err := os.Symlink("jujud",
		filepath.Join(dataDir, "tools", "jujud-link"))
	c.Assert(err, tc.ErrorIsNil)

	// server.pem, shared-secret and nonce.txt are absent and must be
	// tolerated.
	files, err := backups.GetFilesToBackUp(rootDir, &backups.Paths{
		DataDir: relDataDir,
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(set.NewStrings(files...), tc.DeepEquals, set.NewStrings(
		filepath.Join(dataDir, "objectstore", "ns1", "blob"),
		filepath.Join(dataDir, "tools", "jujud"),
		filepath.Join(dataDir, "tools", "jujud-link"),
		filepath.Join(dataDir, "agents", "machine-0", "agent.conf"),
		filepath.Join(dataDir, "init", "jujud-machine-0.conf"),
		filepath.Join(dataDir, "system-identity"),
		authKeys,
	))
}

func (s *filesSuite) TestGetFilesToBackUpWithRootDir(c *tc.C) {
	rootDir := c.MkDir()
	dataDir := filepath.Join(rootDir, relDataDir)
	s.writeFile(c, filepath.Join(dataDir, "objectstore", "blob"), "blob")
	s.writeFile(c, filepath.Join(dataDir, "tools", "jujud"), "binary")

	files, err := backups.GetFilesToBackUp(rootDir, &backups.Paths{
		DataDir: relDataDir,
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(set.NewStrings(files...), tc.DeepEquals, set.NewStrings(
		filepath.Join(dataDir, "objectstore", "blob"),
		filepath.Join(dataDir, "tools", "jujud"),
	))
}

func (s *filesSuite) TestGetFilesToBackUpSkipsObjectstoreTmp(c *tc.C) {
	rootDir := c.MkDir()
	dataDir := filepath.Join(rootDir, relDataDir)
	// In-flight object store uploads are staged under
	// <objectstore>/<namespace>/tmp; they never made it into the
	// object store.
	s.writeFile(c,
		filepath.Join(dataDir, "objectstore", "ns1", "tmp", "tmp1234"),
		"partial upload")
	s.writeFile(c, filepath.Join(dataDir, "objectstore", "ns1", "blob"),
		"blob")
	// A "tmp" directory deeper in a namespace holds object data, not
	// staging files, and is still backed up.
	s.writeFile(c, filepath.Join(dataDir, "objectstore", "ns1",
		"applications", "tmp", "resource"), "resource")
	s.writeFile(c, filepath.Join(dataDir, "tools", "jujud"), "binary")

	files, err := backups.GetFilesToBackUp(rootDir, &backups.Paths{
		DataDir: relDataDir,
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(set.NewStrings(files...), tc.DeepEquals, set.NewStrings(
		filepath.Join(dataDir, "objectstore", "ns1", "blob"),
		filepath.Join(dataDir, "objectstore", "ns1", "applications",
			"tmp", "resource"),
		filepath.Join(dataDir, "tools", "jujud"),
	))
}

func (s *filesSuite) TestBackupDirToUse(c *tc.C) {
	c.Check(backups.BackupDirToUse("/some/dir"), tc.Equals, "/some/dir")
	c.Check(backups.BackupDirToUse(""), tc.Equals, os.TempDir())
}

func (s *filesSuite) TestIsValidBackupFilepath(c *tc.C) {
	dir := c.MkDir()
	valid := filepath.Join(dir, backups.FilenamePrefix+"2020.tar.gz")
	s.writeFile(c, valid, "archive data")

	ok, err := backups.IsValidBackupFilepath(dir, valid)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(ok, tc.IsTrue)

	// A backup file in a subdirectory of root is rejected: only files
	// directly under root are served.
	nested := filepath.Join(dir, "sub", backups.FilenamePrefix+"nested.tar.gz")
	s.writeFile(c, nested, "archive data")

	for _, invalid := range []struct {
		name  string
		path_ string
	}{
		{"relative", "juju-backup-foo.tar.gz"},
		{"absolute missing file", filepath.Join(dir, "juju-backup-missing.tar.gz")},
		{"prefix-refusing", filepath.Join(dir, "other.tar.gz")},
		{"nested", nested},
	} {
		ok, err = backups.IsValidBackupFilepath(dir, invalid.path_)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(ok, tc.IsFalse, tc.Commentf("case %q", invalid.name))
	}
}
