// Copyright 2011, 2012, 2013, 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package zip_test

import (
	stdzip "archive/zip"
	"fmt"
	"path"
	"sort"

	gc "launchpad.net/gocheck"

	jc "launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/utils/zip"
)

type ZipSuite struct {
	BaseSuite
}

var _ = gc.Suite(&ZipSuite{})

func (s *ZipSuite) TestWalk(c *gc.C) {
	reader := s.makeZip(c,
		file{"file", "", 0644},
		symlink{"symlink", "file"},
		dir{"dir", 0755},
		dir{"dir/dir", 0755},
		file{"dir/dir/file", "", 0644},
		symlink{"dir/dir/symlink", ".."},
	)
	expect := []string{
		"file", "symlink", "dir", "dir/dir", "dir/dir/file", "dir/dir/symlink",
	}
	actual := []string{}
	callback := func(zipFile *stdzip.File) error {
		actual = append(actual, path.Clean(zipFile.Name))
		return nil
	}

	err := zip.Walk(reader, callback)
	c.Assert(err, gc.IsNil)
	sort.Strings(expect)
	sort.Strings(actual)
	c.Check(actual, jc.DeepEquals, expect)
}

func (s *ZipSuite) TestWalkError(c *gc.C) {
	reader := s.makeZip(c,
		dir{"dir", 0755},
		dir{"dir/dir", 0755},
	)
	count := 0
	callback := func(_ *stdzip.File) error {
		count += 1
		return fmt.Errorf("lol borken")
	}
	err := zip.Walk(reader, callback)
	c.Assert(err, gc.ErrorMatches, `cannot process "dir": lol borken`)
	c.Assert(count, gc.Equals, 1)
}

func (s *ZipSuite) TestFind(c *gc.C) {
	reader := s.makeZip(c,
		file{"some-file", "", 0644},
		file{"another-file", "", 0644},
		symlink{"some-symlink", "some-file"},
		dir{"some-dir", 0755},
		dir{"some-dir/another-dir", 0755},
		file{"some-dir/another-file", "", 0644},
	)

	for i, test := range []struct {
		pattern string
		expect  []string
	}{{
		"", nil,
	}, {
		"no-matches", nil,
	}, {
		"some-file", []string{
			"some-file"},
	}, {
		"another-file", []string{
			"another-file",
			"some-dir/another-file"},
	}, {
		"some-*", []string{
			"some-file",
			"some-symlink",
			"some-dir"},
	}, {
		"another-*", []string{
			"another-file",
			"some-dir/another-dir",
			"some-dir/another-file"},
	}, {
		"*", []string{
			"some-file",
			"another-file",
			"some-symlink",
			"some-dir",
			"some-dir/another-dir",
			"some-dir/another-file"},
	}} {
		c.Logf("test %d: %q", i, test.pattern)
		actual, err := zip.Find(reader, test.pattern)
		c.Assert(err, gc.IsNil)
		sort.Strings(test.expect)
		sort.Strings(actual)
		c.Check(actual, jc.DeepEquals, test.expect)
	}

	c.Logf("test $spanish-inquisition: FindAll")
	expect, err := zip.Find(reader, "*")
	c.Assert(err, gc.IsNil)
	actual, err := zip.FindAll(reader)
	c.Assert(err, gc.IsNil)
	sort.Strings(expect)
	sort.Strings(actual)
	c.Check(actual, jc.DeepEquals, expect)
}

func (s *ZipSuite) TestExtractAll(c *gc.C) {
	creators := []creator{
		file{"some-file", "content 1", 0644},
		file{"another-file", "content 2", 0640},
		symlink{"some-symlink", "some-file"},
		dir{"some-dir", 0750},
		file{"some-dir/another-file", "content 3", 0644},
		dir{"some-dir/another-dir", 0755},
		symlink{"some-dir/another-dir/another-symlink", "../../another-file"},
	}
	reader := s.makeZip(c, creators...)
	targetPath := c.MkDir()
	err := zip.ExtractAll(reader, targetPath)
	c.Assert(err, gc.IsNil)
	for i, creator := range creators {
		c.Logf("test %d: %#v", i, creator)
		creator.check(c, targetPath)
	}
}

func (s *ZipSuite) TestExtractAllOverwriteFiles(c *gc.C) {
	name := "some-file"
	for i, test := range []creator{
		file{name, "content", 0644},
		dir{name, 0751},
		symlink{name, "wherever"},
	} {
		c.Logf("test %d: %#v", i, test)
		targetPath := c.MkDir()
		file{name, "original", 0}.create(c, targetPath)
		reader := s.makeZip(c, test)
		err := zip.ExtractAll(reader, targetPath)
		c.Check(err, gc.IsNil)
		test.check(c, targetPath)
	}
}

func (s *ZipSuite) TestFails(c *gc.C) {
	c.Fatalf("not finished")
}
