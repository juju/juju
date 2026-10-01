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

	// A dangling symlink is skipped rather than failing the backup:
	// hook-tool symlinks dangle when a unit's tools directory is wiped
	// and recreated, and recovery never installs archived tools.
	c.Assert(os.Symlink("gone", filepath.Join(dataDir, "tools", "dangling")),
		tc.ErrorIsNil)

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
	// The dangling symlink is not in the collection.
	c.Check(set.NewStrings(files...).Contains(
		filepath.Join(dataDir, "tools", "dangling")), tc.IsFalse)
}

// TestGetFilesToBackUpUnreadableSSHKeys proves that an authorized_keys
// file the apiserver cannot read — as happens on container controllers,
// where the apiserver runs as a non-host user — is skipped instead of
// failing the backup.
func (s *filesSuite) TestGetFilesToBackUpUnreadableSSHKeys(c *tc.C) {
	if os.Geteuid() == 0 {
		c.Skip("permission checks do not apply to root")
	}
	rootDir := c.MkDir()
	dataDir := filepath.Join(rootDir, relDataDir)
	s.writeFile(c, filepath.Join(dataDir, "objectstore", "blob"), "blob")
	s.writeFile(c, filepath.Join(dataDir, "tools", "jujud"), "binary")

	sshDir := filepath.Join(rootDir, "home", "ubuntu", ".ssh")
	s.writeFile(c, filepath.Join(sshDir, "authorized_keys"), "ssh-rsa key")
	c.Assert(os.Chmod(sshDir, 0o000), tc.ErrorIsNil)
	s.AddCleanup(func(*tc.C) {
		_ = os.Chmod(sshDir, 0o755)
	})

	files, err := backups.GetFilesToBackUp(rootDir, &backups.Paths{
		DataDir: relDataDir,
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(set.NewStrings(files...).Contains(
		filepath.Join(rootDir, backups.SSHDir, "authorized_keys")), tc.IsFalse)
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
