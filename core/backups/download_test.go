// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/juju/tc"

	"github.com/juju/juju/core/backups"
	"github.com/juju/juju/internal/uuid"
)

type oneShotSuite struct{}

func TestOneShotSuite(t *testing.T) {
	tc.Run(t, &oneShotSuite{})
}

func (s *oneShotSuite) TestOneShotDir(c *tc.C) {
	c.Check(backups.OneShotDir("/backup"), tc.Equals,
		filepath.Join("/backup", backups.OneShotDirName))
}

func (s *oneShotSuite) TestOneShotArchivePath(c *tc.C) {
	id, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	path, err := backups.OneShotArchivePath("/backup", id.String())
	c.Check(err, tc.ErrorIsNil)
	c.Check(path, tc.Equals,
		filepath.Join("/backup", backups.OneShotDirName, id.String()+".tar.gz"))
}

func (s *oneShotSuite) TestOneShotArchivePathInvalidID(c *tc.C) {
	// The id is the only client-controlled input to the path: anything
	// that is not a UUID is rejected, so ids can never traverse the
	// filesystem.
	for _, id := range []string{
		"",
		"not-a-uuid",
		"../backup-dir",
		"../../etc/passwd",
		idWithNul,
	} {
		c.Logf("id %q", id)
		path, err := backups.OneShotArchivePath("/backup", id)
		c.Check(err, tc.ErrorMatches, "invalid backup id.*")
		c.Check(path, tc.Equals, "")
	}
}

const idWithNul = "00000000-0000-4000-8000-0000000000\x00"

func (s *oneShotSuite) TestCleanExpiredOneShotArchives(c *tc.C) {
	backupDir := c.MkDir()
	now := time.Now()

	expiredID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)
	freshID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	expired, err := backups.OneShotArchivePath(backupDir, expiredID.String())
	c.Assert(err, tc.ErrorIsNil)
	fresh, err := backups.OneShotArchivePath(backupDir, freshID.String())
	c.Assert(err, tc.ErrorIsNil)
	for _, path := range []string{expired, fresh} {
		c.Assert(os.MkdirAll(filepath.Dir(path), 0755), tc.ErrorIsNil)
		c.Assert(os.WriteFile(path, []byte("archive data"), 0600), tc.ErrorIsNil)
	}
	old := now.Add(-2 * time.Hour)
	c.Assert(os.Chtimes(expired, old, old), tc.ErrorIsNil)
	err = backups.CleanExpiredOneShotArchives(backupDir, time.Hour, now)
	c.Check(err, tc.ErrorIsNil)

	_, err = os.Stat(expired)
	c.Assert(err, tc.Satisfies, os.IsNotExist)
	_, err = os.Stat(fresh)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *oneShotSuite) TestCleanExpiredOneShotArchivesEmptyDir(c *tc.C) {
	// A backup dir without a one-shot dir is not an error.
	backupDir := c.MkDir()
	err := backups.CleanExpiredOneShotArchives(backupDir, time.Hour, time.Now())
	c.Check(err, tc.ErrorIsNil)
}

func (s *oneShotSuite) TestCleanExpiredOneShotArchivesNonRegular(c *tc.C) {
	// Directories inside the one-shot dir are not ours to remove.
	backupDir := c.MkDir()
	oneShotDir := backups.OneShotDir(backupDir)
	sub := filepath.Join(oneShotDir, "sub")
	c.Assert(os.MkdirAll(sub, 0755), tc.ErrorIsNil)
	old := time.Now().Add(-2 * time.Hour)
	c.Assert(os.Chtimes(sub, old, old), tc.ErrorIsNil)

	err := backups.CleanExpiredOneShotArchives(backupDir, time.Hour, time.Now())
	c.Check(err, tc.ErrorIsNil)

	_, err = os.Stat(sub)
	c.Assert(err, tc.ErrorIsNil)
}
