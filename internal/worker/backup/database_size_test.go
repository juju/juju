// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backup

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/juju/tc"

	coretesting "github.com/juju/juju/internal/testing"
)

type databaseSizeSuite struct {
	coretesting.BaseSuite
}

func TestDatabaseSizeSuite(t *testing.T) {
	tc.Run(t, &databaseSizeSuite{})
}

// TestDatabaseSizeSumsRegularFiles verifies that only regular files under
// the Dqlite directory contribute to the recorded size.
func (s *databaseSizeSuite) TestDatabaseSizeSumsRegularFiles(c *tc.C) {
	dataDir := c.MkDir()
	dqliteDir := filepath.Join(dataDir, dqliteDataDir)
	c.Assert(os.MkdirAll(filepath.Join(dqliteDir, "nested"), 0o755), tc.ErrorIsNil)
	c.Assert(os.WriteFile(filepath.Join(dqliteDir, "a.db"), make([]byte, 10), 0o600), tc.ErrorIsNil)
	c.Assert(os.WriteFile(filepath.Join(dqliteDir, "nested", "b.db"), make([]byte, 20), 0o600), tc.ErrorIsNil)
	c.Assert(os.WriteFile(filepath.Join(dataDir, "outside"), make([]byte, 30), 0o600), tc.ErrorIsNil)
	c.Assert(os.Mkdir(filepath.Join(dqliteDir, "empty"), 0o755), tc.ErrorIsNil)
	c.Assert(os.Symlink(filepath.Join(dataDir, "outside"), filepath.Join(dqliteDir, "link")), tc.ErrorIsNil)

	size, err := databaseSize(c.Context(), dataDir)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(size, tc.Equals, int64(30))
}

// TestDatabaseSizeMissingDqliteDir verifies that a controller without a
// Dqlite directory records zero rather than failing the backup.
func (s *databaseSizeSuite) TestDatabaseSizeMissingDqliteDir(c *tc.C) {
	size, err := databaseSize(c.Context(), c.MkDir())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(size, tc.Equals, int64(0))
}

// TestDatabaseSizeContextCancelled verifies that a cancelled backup request
// stops the directory walk.
func (s *databaseSizeSuite) TestDatabaseSizeContextCancelled(c *tc.C) {
	ctx, cancel := context.WithCancel(c.Context())
	cancel()

	_, err := databaseSize(ctx, c.MkDir())
	c.Check(err, tc.ErrorIs, context.Canceled)
}

// TestDatabaseSizeEmptyDqliteDir verifies that an empty Dqlite directory
// records zero.
func (s *databaseSizeSuite) TestDatabaseSizeEmptyDqliteDir(c *tc.C) {
	dataDir := c.MkDir()
	c.Assert(os.Mkdir(filepath.Join(dataDir, dqliteDataDir), 0o755), tc.ErrorIsNil)

	size, err := databaseSize(c.Context(), dataDir)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(size, tc.Equals, int64(0))
}
