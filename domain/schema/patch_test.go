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
	fs *MockReadFileFS
}

func (s *patchesSuite) TestReadPatches(c *tc.C) {
	defer s.setupMocks(c).Finish()

	exp := s.fs.EXPECT()
	exp.ReadFile("testdata/001_initial.sql").Return([]byte("CREATE TABLE test1 (id INTEGER);"), nil)
	exp.ReadFile("testdata/002_add_table.PATCH.sql").Return([]byte("CREATE TABLE test2 (id INTEGER);"), nil)

	entries := []fs.DirEntry{
		dirEntry{name: "001_initial.sql", isDir: false},
		dirEntry{name: "002_add_table.PATCH.sql", isDir: false},
		dirEntry{name: "subdir", isDir: true},
	}

	// Writes tests for readPatches function.
	patches, postPatches := readPatches(entries, s.fs, func(s string) string {
		return "testdata/" + s
	})

	c.Assert(patches, tc.HasLen, 1)
	c.Assert(postPatches, tc.HasLen, 1)

	c.Check(schema.Stmt(patches[0]()), tc.Equals, "CREATE TABLE test1 (id INTEGER);")
	c.Check(schema.Stmt(postPatches[0]()), tc.Equals, "CREATE TABLE test2 (id INTEGER);")
}

func (s *patchesSuite) TestReadPatchesCollision(c *tc.C) {
	defer s.setupMocks(c).Finish()

	exp := s.fs.EXPECT()
	exp.ReadFile("testdata/001_initial.sql").Return([]byte("CREATE TABLE test1 (id INTEGER);"), nil)
	exp.ReadFile("testdata/001_initial.PATCH.sql").Return([]byte("CREATE TABLE test2 (id INTEGER);"), nil)

	entries := []fs.DirEntry{
		dirEntry{name: "001_initial.sql", isDir: false},
		dirEntry{name: "001_initial.PATCH.sql", isDir: false},
		dirEntry{name: "subdir", isDir: true},
	}

	// Writes tests for readPatches function.
	patches, postPatches := readPatches(entries, s.fs, func(s string) string {
		return "testdata/" + s
	})

	c.Assert(patches, tc.HasLen, 1)
	c.Assert(postPatches, tc.HasLen, 1)

	c.Check(schema.Stmt(patches[0]()), tc.Equals, "CREATE TABLE test1 (id INTEGER);")
	c.Check(schema.Stmt(postPatches[0]()), tc.Equals, "CREATE TABLE test2 (id INTEGER);")
}

func (s *patchesSuite) TestReadPatchesOrdering(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Ensure the order is correct regardless of the input order.

	exp := s.fs.EXPECT()
	gomock.InOrder(
		exp.ReadFile("testdata/001_a.sql").Return([]byte("SELECT 1 FROM a"), nil),
		exp.ReadFile("testdata/001_b.sql").Return([]byte("SELECT 1 FROM b"), nil),
		exp.ReadFile("testdata/001_c.sql").Return([]byte("SELECT 1 FROM c"), nil),
		exp.ReadFile("testdata/001_d.sql").Return([]byte("SELECT 1 FROM d"), nil),
		exp.ReadFile("testdata/001_e.sql").Return([]byte("SELECT 1 FROM e"), nil),
		exp.ReadFile("testdata/001_f.sql").Return([]byte("SELECT 1 FROM f"), nil),
		exp.ReadFile("testdata/001_g.sql").Return([]byte("SELECT 1 FROM g"), nil),
		exp.ReadFile("testdata/001_h.sql").Return([]byte("SELECT 1 FROM h"), nil),
	)

	entries := []fs.DirEntry{
		dirEntry{name: "001_a.sql", isDir: false},
		dirEntry{name: "001_b.sql", isDir: false},
		dirEntry{name: "001_c.sql", isDir: false},
		dirEntry{name: "001_d.sql", isDir: false},
		dirEntry{name: "001_e.sql", isDir: false},
		dirEntry{name: "001_f.sql", isDir: false},
		dirEntry{name: "001_g.sql", isDir: false},
		dirEntry{name: "001_h.sql", isDir: false},
	}

	// Shuffle the entries to ensure ordering is done by the function.

	rand.Shuffle(len(entries), func(i, j int) {
		entries[i], entries[j] = entries[j], entries[i]
	})

	// Writes tests for readPatches function.
	patches, _ := readPatches(entries, s.fs, func(s string) string {
		return "testdata/" + s
	})

	c.Assert(len(patches), tc.Equals, len(entries))
	for i, patchFn := range patches {
		expected := fmt.Sprintf("SELECT 1 FROM %c", 'a'+i)
		c.Check(schema.Stmt(patchFn()), tc.Equals, expected)
	}
}

func (s *patchesSuite) TestReadPatchesOrderingOfPatchFiles(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Ordering doesn't matter when concerning patches, it'll will always
	// put the patches at the end.

	exp := s.fs.EXPECT()
	exp.ReadFile("testdata/001_initial.sql").Return([]byte("CREATE TABLE test1 (id INTEGER);"), nil)
	exp.ReadFile("testdata/001_add_table.PATCH.sql").Return([]byte("CREATE TABLE test2 (id INTEGER);"), nil)

	entries := []fs.DirEntry{
		dirEntry{name: "001_add_table.PATCH.sql", isDir: false},
		dirEntry{name: "001_initial.sql", isDir: false},
	}

	// Writes tests for readPatches function.
	patches, postPatches := readPatches(entries, s.fs, func(s string) string {
		return "testdata/" + s
	})

	c.Assert(patches, tc.HasLen, 1)
	c.Assert(postPatches, tc.HasLen, 1)

	c.Check(schema.Stmt(patches[0]()), tc.Equals, "CREATE TABLE test1 (id INTEGER);")
	c.Check(schema.Stmt(postPatches[0]()), tc.Equals, "CREATE TABLE test2 (id INTEGER);")
}

func (s *patchesSuite) TestReadPatchesOrderingWithPatchFiles(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Ensure the order is correct regardless of the input order.

	exp := s.fs.EXPECT()
	gomock.InOrder(
		exp.ReadFile("testdata/001_a.sql").Return([]byte("SELECT 1 FROM a"), nil),
		exp.ReadFile("testdata/001_b.sql").Return([]byte("SELECT 1 FROM b"), nil),
		exp.ReadFile("testdata/001_c.sql").Return([]byte("SELECT 1 FROM c"), nil),
		exp.ReadFile("testdata/001_d.sql").Return([]byte("SELECT 1 FROM d"), nil),
		exp.ReadFile("testdata/001_e.sql").Return([]byte("SELECT 1 FROM e"), nil),
		exp.ReadFile("testdata/001_f.PATCH.sql").Return([]byte("SELECT 1 FROM f"), nil),
		exp.ReadFile("testdata/001_g.PATCH.sql").Return([]byte("SELECT 1 FROM g"), nil),
		exp.ReadFile("testdata/001_h.PATCH.sql").Return([]byte("SELECT 1 FROM h"), nil),
	)

	entries := []fs.DirEntry{
		dirEntry{name: "001_a.sql", isDir: false},
		dirEntry{name: "001_b.sql", isDir: false},
		dirEntry{name: "001_c.sql", isDir: false},
		dirEntry{name: "001_d.sql", isDir: false},
		dirEntry{name: "001_e.sql", isDir: false},
		dirEntry{name: "001_f.PATCH.sql", isDir: false},
		dirEntry{name: "001_g.PATCH.sql", isDir: false},
		dirEntry{name: "001_h.PATCH.sql", isDir: false},
	}

	// Shuffle the entries to ensure ordering is done by the function.

	rand.Shuffle(len(entries), func(i, j int) {
		entries[i], entries[j] = entries[j], entries[i]
	})

	// Writes tests for readPatches function.
	patches, postPatches := readPatches(entries, s.fs, func(s string) string {
		return "testdata/" + s
	})

	c.Assert(patches, tc.HasLen, len(entries)-3)
	c.Assert(postPatches, tc.HasLen, 3)

	for i, patchFn := range patches {
		expected := fmt.Sprintf("SELECT 1 FROM %c", 'a'+i)
		c.Check(schema.Stmt(patchFn()), tc.Equals, expected)
	}

	for i, patchFn := range postPatches {
		expected := fmt.Sprintf("SELECT 1 FROM %c", 'f'+i)
		c.Check(schema.Stmt(patchFn()), tc.Equals, expected)
	}
}

func (s *patchesSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.fs = NewMockReadFileFS(ctrl)

	c.Cleanup(func() {
		s.fs = nil
	})

	return ctrl
}

type dirEntry struct {
	name  string
	isDir bool
}

func (d dirEntry) Name() string               { return d.name }
func (d dirEntry) IsDir() bool                { return d.isDir }
func (d dirEntry) Type() fs.FileMode          { return 0 }
func (d dirEntry) Info() (fs.FileInfo, error) { return nil, nil }
