// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/juju/tc"

	coretesting "github.com/juju/juju/internal/testing"
)

type discoverSuite struct {
	coretesting.BaseSuite
}

func TestDiscoverSuite(t *testing.T) {
	tc.Run(t, &discoverSuite{})
}

func (s *discoverSuite) TestFindRepoRootNested(c *tc.C) {
	root := c.MkDir()
	c.Assert(os.WriteFile(filepath.Join(root, "go.mod"), nil, 0o644), tc.ErrorIsNil)

	nested := filepath.Join(root, "a", "b")
	c.Assert(os.MkdirAll(nested, 0o755), tc.ErrorIsNil)

	found, err := findRepoRoot(nested)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(found, tc.Equals, root)

	// The root itself resolves to itself.
	found, err = findRepoRoot(root)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(found, tc.Equals, root)
}

func (s *discoverSuite) TestFindRepoRootNotFound(c *tc.C) {
	// The temp directory must not live inside a Go module for this
	// test to be meaningful; if it does, skip rather than fail.
	tmp := c.MkDir()
	if _, err := findRepoRoot(tmp); err == nil {
		c.Skip("temporary directory lies inside a Go module")
	}

	deep := filepath.Join(tmp, "x", "y")
	c.Assert(os.MkdirAll(deep, 0o755), tc.ErrorIsNil)

	_, err := findRepoRoot(deep)
	c.Check(err, tc.ErrorMatches, "repository root not found from .*")
}

// makeTree creates the named files (slash-separated relative paths)
// under root, including parent directories.
func makeTree(c *tc.C, root string, files ...string) {
	c.Helper()
	for _, rel := range files {
		path := filepath.Join(root, filepath.FromSlash(rel))
		c.Assert(os.MkdirAll(filepath.Dir(path), 0o755), tc.ErrorIsNil)
		c.Assert(os.WriteFile(path, []byte("package "+filepath.Base(rel)+"\n"), 0o644), tc.ErrorIsNil)
	}
}

// outsideModule skips the test when the temp directory happens to lie
// inside a Go module, which would make git-based discovery succeed and
// defeat walk-fallback coverage.
func outsideModule(c *tc.C, dir string) {
	c.Helper()
	if _, err := findRepoRoot(dir); err == nil {
		c.Skip("temporary directory lies inside a Go module")
	}
}

func (s *discoverSuite) TestDiscoverGoFilesWalkFallback(c *tc.C) {
	root := c.MkDir()
	outsideModule(c, root)
	makeTree(c, root,
		"a/a.go",
		"b/b.go",
		"sub/sub.go",
		"sub/deep/deep.go",
		"vendor/v.go",
		"sub/vendor/v.go",
		"node_modules/n.go",
		"_build/x.go",
		".git/g.go",
		"scripts/sqlcheck/s.go",
		"a/notgo.txt",
	)

	// No .git in root: git ls-files fails and discovery falls back to
	// walking the tree.
	paths, err := discoverGoFiles(c.Context(), root, nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(paths, tc.DeepEquals, []string{
		"a/a.go",
		"b/b.go",
		"sub/deep/deep.go",
		"sub/sub.go",
	})
}

func (s *discoverSuite) TestDiscoverGoFilesCancelledContext(c *tc.C) {
	root := c.MkDir()
	outsideModule(c, root)
	makeTree(c, root, "a/a.go")

	ctx, cancel := context.WithCancel(c.Context())
	cancel()
	_, err := discoverGoFiles(ctx, root, nil)
	c.Check(err, tc.ErrorMatches, "context canceled")
}

func (s *discoverSuite) TestDiscoverFromArgsDirectory(c *tc.C) {
	root := c.MkDir()
	makeTree(c, root,
		"a/a.go",
		"a/b.go",
		"a/vendor/skip.go",
		"a/node_modules/skip.go",
		"a/_build/skip.go",
		"a/scripts/sqlcheck/skip.go",
		"a/.git/skip.go",
		"a/notgo.txt",
	)

	// Note: only the repository-top-level scripts/sqlcheck directory is
	// excluded, so a/scripts/sqlcheck/skip.go is legitimately included.
	paths, err := discoverFromArgs(c.Context(), root, []string{filepath.Join(root, "a")})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(paths, tc.DeepEquals, []string{
		"a/a.go",
		"a/b.go",
		"a/scripts/sqlcheck/skip.go",
	})
}

func (s *discoverSuite) TestDiscoverFromArgsSingleFile(c *tc.C) {
	root := c.MkDir()
	makeTree(c, root, "a/a.go", "a/b.go", "a/notgo.txt")

	paths, err := discoverFromArgs(c.Context(), root, []string{filepath.Join(root, "a", "a.go")})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(paths, tc.DeepEquals, []string{"a/a.go"})

	// Non-Go files are silently ignored.
	paths, err = discoverFromArgs(c.Context(), root, []string{filepath.Join(root, "a", "notgo.txt")})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(paths, tc.IsNil)

	// Repeated arguments collapse to a single entry.
	paths, err = discoverFromArgs(c.Context(), root, []string{
		filepath.Join(root, "a", "a.go"),
		filepath.Join(root, "a", "a.go"),
		filepath.Join(root, "a"),
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(paths, tc.DeepEquals, []string{"a/a.go", "a/b.go"})
}

func (s *discoverSuite) TestDiscoverFromArgsMissingPath(c *tc.C) {
	root := c.MkDir()
	_, err := discoverFromArgs(c.Context(), root, []string{filepath.Join(root, "nope", "gone.go")})
	c.Check(err, tc.ErrorMatches, ".*no such file or directory.*")
}

func (s *discoverSuite) TestGlobGoFiles(c *tc.C) {
	root := c.MkDir()
	outsideModule(c, root)
	makeTree(c, root,
		"domain/cloud/state/state.go",
		"domain/cloud/service/service.go",
		"domain/secret/state/state.go",
	)

	paths, err := globGoFiles(c.Context(), root, "domain/*/state/**")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(paths, tc.DeepEquals, []string{
		"domain/cloud/state/state.go",
		"domain/secret/state/state.go",
	})

	paths, err = globGoFiles(c.Context(), root, "domain/**/state/state.go")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(paths, tc.DeepEquals, []string{
		"domain/cloud/state/state.go",
		"domain/secret/state/state.go",
	})
}

func (s *discoverSuite) TestGlobGoFilesNoMatch(c *tc.C) {
	root := c.MkDir()
	outsideModule(c, root)
	makeTree(c, root, "domain/cloud/state/state.go")

	_, err := globGoFiles(c.Context(), root, "nothing/*/here/**")
	c.Check(err, tc.ErrorMatches, `pattern "nothing/\*/here/\*\*" matched no tracked Go files`)
}

func (s *discoverSuite) TestFilterGoFiles(c *tc.C) {
	root := c.MkDir()
	filtered := filterGoFiles(root, []string{
		"a/a.go",
		"./a/a.go",
		"vendor/v.go",
		"sub/vendor/v.go",
		"node_modules/n.go",
		"_build/x.go",
		".git/g.go",
		"scripts/sqlcheck/s.go",
		"scripts/other/o.go",
	})
	c.Check(filtered, tc.DeepEquals, []string{
		"a/a.go",
		"a/a.go",
		"scripts/other/o.go",
	})
}

func (s *discoverSuite) TestUniqueStrings(c *tc.C) {
	c.Check(uniqueStrings(nil), tc.IsNil)
	c.Check(uniqueStrings([]string{"a", "b", "a", "c", "b"}), tc.DeepEquals, []string{"a", "b", "c"})
}

func (s *discoverSuite) TestReadGoFiles(c *tc.C) {
	root := c.MkDir()
	makeTree(c, root, "a/a.go", "a/b.go")

	files, err := readGoFiles(root, []string{"a/a.go", "a/b.go"})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(len(files), tc.Equals, 2)
	c.Check(files[0].Path, tc.Equals, "a/a.go")
	c.Check(string(files[0].Src), tc.Equals, "package a.go\n")

	_, err = readGoFiles(root, []string{"a/missing.go"})
	c.Check(err, tc.ErrorMatches, ".*no such file or directory.*")
}

func (s *discoverSuite) TestGroupByPackage(c *tc.C) {
	files := []goFile{
		{Path: "a/one.go", Src: []byte("package p\n")},
		{Path: "a/two.go", Src: []byte("package p\n")},
		{Path: "a/one_test.go", Src: []byte("package p_test\n")},
		{Path: "b/three.go", Src: []byte("package q\n")},
		// No package clause: grouped by directory base name.
		{Path: "c/four.go", Src: []byte("// no package\n")},
	}
	groups := groupByPackage(files)
	c.Check(len(groups), tc.Equals, 4)

	c.Check(len(groups[0]), tc.Equals, 2)
	c.Check(groups[0][0].Path, tc.Equals, "a/one.go")
	c.Check(groups[0][1].Path, tc.Equals, "a/two.go")

	var keys []string
	for _, g := range groups {
		keys = append(keys, g[0].Path)
	}
	// Group order follows first appearance.
	c.Check(keys, tc.DeepEquals, []string{
		"a/one.go",
		"a/one_test.go",
		"b/three.go",
		"c/four.go",
	})

	// Same package, same directory: one group with both files.
	same := []goFile{
		{Path: "a/one.go", Src: []byte("package p\n")},
		{Path: "a/two.go", Src: []byte("package p\n")},
	}
	groups = groupByPackage(same)
	c.Check(len(groups), tc.Equals, 1)
	c.Check(len(groups[0]), tc.Equals, 2)
}

func (s *discoverSuite) TestPackageNameOf(c *tc.C) {
	c.Check(packageNameOf([]byte("package main\n")), tc.Equals, "main")
	c.Check(packageNameOf([]byte("// comment\n\npackage state_test\n")), tc.Equals, "state_test")
	c.Check(packageNameOf([]byte("no package here\n")), tc.Equals, "")
}
