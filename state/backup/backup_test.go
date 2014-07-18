// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backup_test

import (
	"path/filepath"

	gc "launchpad.net/gocheck"

	"github.com/juju/juju/state/backup"
)

func (b *BackupSuite) TestBackup(c *gc.C) {
	b.createTestFiles(c)
	ranCommand := false

	b.PatchValue(backup.GetMongodumpPath, func() (string, error) {
		return "bogusmongodump", nil
	})
	b.PatchValue(backup.GetFilesToBackup, func() ([]string, error) {
		return b.testFiles, nil
	})
	b.PatchValue(backup.DoBackup, func(command string, args ...string) error {
		ranCommand = true
		return nil
	})
	dbinfo := backup.DBConnInfo{"localhost:8080", "bogus-user", "boguspassword"}
	bkpFile, shaSum, err := backup.Backup(&dbinfo, b.cwd)
	c.Check(err, gc.IsNil)
	c.Assert(ranCommand, gc.Equals, true)

	// It is important that the filename uses non-special characters
	// only because it is returned in a header (unencoded) by the
	// backup API call. This also avoids compatibility problems with
	// client side filename conventions.
	c.Check(bkpFile, gc.Matches, `^[a-z0-9_.-]+$`)

	filename := filepath.Join(b.cwd, bkpFile)
	fileShaSum := shaSumFile(c, filename)
	c.Assert(shaSum, gc.Equals, fileShaSum)

	bkpExpectedContents := []expectedTarContents{
		{"juju-backup", ""},
		{"juju-backup/dump", ""},
		{"juju-backup/root.tar", ""},
	}
	b.assertTarContents(c, bkpExpectedContents, filename, true)
}

func (b *BackupSuite) TestStorageName(c *gc.C) {
	c.Assert(backup.StorageName("foo"), gc.Equals, "/backups/foo")
	c.Assert(backup.StorageName("/foo/bar"), gc.Equals, "/backups/bar")
	c.Assert(backup.StorageName("foo/bar"), gc.Equals, "/backups/bar")
}
