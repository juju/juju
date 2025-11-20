// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package schema

import (
	"fmt"
	fs "io/fs"
	"math/rand"
	"testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/database/schema"
)

func TestPatches(t *testing.T) {
	tc.Run(t, &patchesSuite{})
}

type patchesSuite struct {
	fs *MockReadFileDirFS
}

func (s *patchesSuite) TestReadPatches(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	exp := s.fs.EXPECT()

	entry1 := NewMockDirEntry(ctrl)
	entry1.EXPECT().Name().Return("001_initial.sql")
	entry1.EXPECT().IsDir().Return(false)

	entry2 := NewMockDirEntry(ctrl)
	entry2.EXPECT().Name().Return("002_add_table.sql")
	entry2.EXPECT().IsDir().Return(false)

	entry3 := NewMockDirEntry(ctrl)
	entry3.EXPECT().IsDir().Return(true)

	exp.ReadDir("testdata").Return([]fs.DirEntry{entry1, entry2, entry3}, nil)

	exp.ReadFile("testdata/001_initial.sql").Return([]byte("CREATE TABLE test1 (id INTEGER);"), nil)
	exp.ReadFile("testdata/002_add_table.sql").Return([]byte("CREATE TABLE test2 (id INTEGER);"), nil)

	// Writes tests for readPatches function.
	patches, err := readPatches(s.fs, "testdata")
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(patches, tc.HasLen, 2)

	c.Check(schema.Stmt(patches[0]()), tc.Equals, "CREATE TABLE test1 (id INTEGER);")
	c.Check(schema.Stmt(patches[1]()), tc.Equals, "CREATE TABLE test2 (id INTEGER);")
}

func (s *patchesSuite) TestReadPatchesOrdering(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	// Ensure the order is correct regardless of the input order.

	exp := s.fs.EXPECT()

	entry1 := NewMockDirEntry(ctrl)
	entry1.EXPECT().Name().Return("001_a.sql")
	entry1.EXPECT().IsDir().Return(false)

	entry2 := NewMockDirEntry(ctrl)
	entry2.EXPECT().Name().Return("001_b.sql")
	entry2.EXPECT().IsDir().Return(false)

	entry3 := NewMockDirEntry(ctrl)
	entry3.EXPECT().Name().Return("001_c.sql")
	entry3.EXPECT().IsDir().Return(false)

	entry4 := NewMockDirEntry(ctrl)
	entry4.EXPECT().Name().Return("001_d.sql")
	entry4.EXPECT().IsDir().Return(false)

	gomock.InOrder(
		exp.ReadFile("testdata/001_a.sql").Return([]byte("SELECT 1 FROM a"), nil),
		exp.ReadFile("testdata/001_b.sql").Return([]byte("SELECT 1 FROM b"), nil),
		exp.ReadFile("testdata/001_c.sql").Return([]byte("SELECT 1 FROM c"), nil),
		exp.ReadFile("testdata/001_d.sql").Return([]byte("SELECT 1 FROM d"), nil),
	)

	entries := []fs.DirEntry{
		entry1,
		entry2,
		entry3,
		entry4,
	}

	// Shuffle the entries to ensure ordering is done by the function.

	rand.Shuffle(len(entries), func(i, j int) {
		entries[i], entries[j] = entries[j], entries[i]
	})

	exp.ReadDir("testdata").Return(entries, nil)

	// Writes tests for readPatches function.
	patches, _ := readPatches(s.fs, "testdata")

	c.Assert(len(patches), tc.Equals, len(entries))
	for i, patchFn := range patches {
		expected := fmt.Sprintf("SELECT 1 FROM %c", 'a'+i)
		c.Check(schema.Stmt(patchFn()), tc.Equals, expected)
	}
}

func (s *patchesSuite) TestReadPatchesSkipsPostPatches(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	exp := s.fs.EXPECT()

	entry1 := NewMockDirEntry(ctrl)
	entry1.EXPECT().Name().Return("001_initial.sql")
	entry1.EXPECT().IsDir().Return(false)

	entry2 := NewMockDirEntry(ctrl)
	entry2.EXPECT().Name().Return("002_add_table.PATCH.sql")
	entry2.EXPECT().IsDir().Return(false)

	exp.ReadDir("testdata").Return([]fs.DirEntry{entry1, entry2}, nil)

	exp.ReadFile("testdata/001_initial.sql").Return([]byte("CREATE TABLE test1 (id INTEGER);"), nil)

	// Writes tests for readPatches function.
	patches, err := readPatches(s.fs, "testdata")
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(patches, tc.HasLen, 1)

	c.Check(schema.Stmt(patches[0]()), tc.Equals, "CREATE TABLE test1 (id INTEGER);")
}

func (s *patchesSuite) TestReadPostPatches(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	exp := s.fs.EXPECT()

	exp.ReadFile("testdata/001_post_patch.PATCH.sql").Return([]byte("ALTER TABLE test1 ADD COLUMN name TEXT;"), nil)
	exp.ReadFile("testdata/002_post_patch.PATCH.sql").Return([]byte("ALTER TABLE test2 ADD COLUMN value INTEGER;"), nil)

	patchFiles := []string{
		"001_post_patch.PATCH.sql",
		"002_post_patch.PATCH.sql",
	}

	patches, err := readPostPatches(s.fs, "testdata", patchFiles)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(patches, tc.HasLen, 2)

	c.Check(schema.Stmt(patches[0]()), tc.Equals, "ALTER TABLE test1 ADD COLUMN name TEXT;")
	c.Check(schema.Stmt(patches[1]()), tc.Equals, "ALTER TABLE test2 ADD COLUMN value INTEGER;")
}

func (s *patchesSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.fs = NewMockReadFileDirFS(ctrl)

	c.Cleanup(func() {
		s.fs = nil
	})

	return ctrl
}
