// Copyright 2011-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package zip_test

import (
	stdzip "archive/zip"
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"sort"

	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/testing"
	ft "launchpad.net/juju-core/testing/filetesting"
	"launchpad.net/juju-core/utils/zip"
)

type ZipSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&ZipSuite{})

func (s *ZipSuite) makeZip(c *gc.C, entries ...ft.Entry) *stdzip.Reader {
	basePath := c.MkDir()
	for _, entry := range entries {
		entry.Create(c, basePath)
	}
	defer os.RemoveAll(basePath)

	outPath := filepath.Join(c.MkDir(), "test.zip")
	cmd := exec.Command("/bin/sh", "-c", fmt.Sprintf("cd %q; zip --fifo --symlinks -r %q .", basePath, outPath))
	output, err := cmd.CombinedOutput()
	c.Assert(err, gc.IsNil, gc.Commentf("Command output: %s", output))

	file, err := os.Open(outPath)
	c.Assert(err, gc.IsNil)
	s.AddCleanup(func(c *gc.C) {
		err := file.Close()
		c.Assert(err, gc.IsNil)
	})
	fileInfo, err := file.Stat()
	c.Assert(err, gc.IsNil)
	reader, err := stdzip.NewReader(file, fileInfo.Size())
	c.Assert(err, gc.IsNil)
	return reader
}

func (s *ZipSuite) TestFind(c *gc.C) {
	reader := s.makeZip(c,
		ft.File{"some-file", "", 0644},
		ft.File{"another-file", "", 0644},
		ft.Symlink{"some-symlink", "some-file"},
		ft.Dir{"some-dir", 0755},
		ft.Dir{"some-dir/another-dir", 0755},
		ft.File{"some-dir/another-file", "", 0644},
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

func (s *ZipSuite) TestFindError(c *gc.C) {
	reader := s.makeZip(c, ft.File{"some-file", "", 0644})
	_, err := zip.Find(reader, "[]")
	c.Assert(err, gc.ErrorMatches, "syntax error in pattern")
}

func (s *ZipSuite) TestExtractAll(c *gc.C) {
	entries := []ft.Entry{
		ft.File{"some-file", "content 1", 0644},
		ft.File{"another-file", "content 2", 0640},
		ft.Symlink{"some-symlink", "some-file"},
		ft.Dir{"some-dir", 0750},
		ft.File{"some-dir/another-file", "content 3", 0644},
		ft.Dir{"some-dir/another-dir", 0755},
		ft.Symlink{"some-dir/another-dir/another-symlink", "../../another-file"},
	}
	reader := s.makeZip(c, entries...)
	targetPath := c.MkDir()
	err := zip.ExtractAll(reader, targetPath)
	c.Assert(err, gc.IsNil)
	for i, entry := range entries {
		c.Logf("test %d: %#v", i, entry)
		entry.Check(c, targetPath)
	}
}

func (s *ZipSuite) TestExtractAllOverwriteFiles(c *gc.C) {
	name := "some-file"
	for i, test := range []ft.Entry{
		ft.File{name, "content", 0644},
		ft.Dir{name, 0751},
		ft.Symlink{name, "wherever"},
	} {
		c.Logf("test %d: %#v", i, test)
		targetPath := c.MkDir()
		ft.File{name, "original", 0}.Create(c, targetPath)
		reader := s.makeZip(c, test)
		err := zip.ExtractAll(reader, targetPath)
		c.Check(err, gc.IsNil)
		test.Check(c, targetPath)
	}
}

func (s *ZipSuite) TestExtractAllOverwriteSymlinks(c *gc.C) {
	name := "some-symlink"
	for i, test := range []ft.Entry{
		ft.File{name, "content", 0644},
		ft.Dir{name, 0751},
		ft.Symlink{name, "wherever"},
	} {
		c.Logf("test %d: %#v", i, test)
		targetPath := c.MkDir()
		original := ft.File{"original", "content", 0644}
		original.Create(c, targetPath)
		ft.Symlink{name, "original"}.Create(c, targetPath)
		reader := s.makeZip(c, test)
		err := zip.ExtractAll(reader, targetPath)
		c.Check(err, gc.IsNil)
		test.Check(c, targetPath)
		original.Check(c, targetPath)
	}
}

func (s *ZipSuite) TestExtractAllOverwriteDirs(c *gc.C) {
	name := "some-dir"
	for i, test := range []ft.Entry{
		ft.File{name, "content", 0644},
		ft.Dir{name, 0751},
		ft.Symlink{name, "wherever"},
	} {
		c.Logf("test %d: %#v", i, test)
		targetPath := c.MkDir()
		ft.Dir{name, 0}.Create(c, targetPath)
		reader := s.makeZip(c, test)
		err := zip.ExtractAll(reader, targetPath)
		c.Check(err, gc.IsNil)
		test.Check(c, targetPath)
	}
}

func (s *ZipSuite) TestExtractAllMergeDirs(c *gc.C) {
	targetPath := c.MkDir()
	ft.Dir{"dir", 0755}.Create(c, targetPath)
	originals := []ft.Entry{
		ft.Dir{"dir/original-dir", 0751},
		ft.File{"dir/original-file", "content 1", 0600},
		ft.Symlink{"dir/original-symlink", "original-file"},
	}
	for _, entry := range originals {
		entry.Create(c, targetPath)
	}
	merges := []ft.Entry{
		ft.Dir{"dir", 0751},
		ft.Dir{"dir/merge-dir", 0750},
		ft.File{"dir/merge-file", "content 2", 0640},
		ft.Symlink{"dir/merge-symlink", "merge-file"},
	}
	reader := s.makeZip(c, merges...)
	err := zip.ExtractAll(reader, targetPath)
	c.Assert(err, gc.IsNil)

	for i, test := range append(originals, merges...) {
		c.Logf("test %d: %#v", i, test)
		test.Check(c, targetPath)
	}
}

func (s *ZipSuite) TestExtractAllSymlinkErrors(c *gc.C) {
	for i, test := range []struct {
		content []ft.Entry
		error   string
	}{{
		content: []ft.Entry{
			ft.Symlink{"symlink", "/blah"},
		},
		error: `cannot extract "symlink": symlink "/blah" is absolute`,
	}, {
		content: []ft.Entry{
			ft.Symlink{"symlink", "../blah"},
		},
		error: `cannot extract "symlink": symlink "../blah" leads out of scope`,
	}, {
		content: []ft.Entry{
			ft.Dir{"dir", 0755},
			ft.Symlink{"dir/symlink", "../../blah"},
		},
		error: `cannot extract "dir/symlink": symlink "../../blah" leads out of scope`,
	}} {
		c.Logf("test %d: %s", i, test.error)
		targetPath := c.MkDir()
		reader := s.makeZip(c, test.content...)
		err := zip.ExtractAll(reader, targetPath)
		c.Check(err, gc.ErrorMatches, test.error)
	}
}

func (s *ZipSuite) TestExtractDir(c *gc.C) {
	reader := s.makeZip(c,
		ft.File{"bad-file", "xxx", 0644},
		ft.Dir{"bad-dir", 0755},
		ft.Symlink{"bad-symlink", "bad-file"},
		ft.Dir{"some-dir", 0751},
		ft.File{"some-dir-bad-lol", "xxx", 0644},
		ft.File{"some-dir/some-file", "content 1", 0644},
		ft.File{"some-dir/another-file", "content 2", 0600},
		ft.Dir{"some-dir/another-dir", 0750},
		ft.Symlink{"some-dir/another-dir/some-symlink", "../some-file"},
	)
	targetParent := c.MkDir()
	targetPath := filepath.Join(targetParent, "random-dir")
	err := zip.Extract(reader, targetPath, "some-dir")
	c.Assert(err, gc.IsNil)

	for i, test := range []ft.Entry{
		ft.Dir{"random-dir", 0751},
		ft.File{"random-dir/some-file", "content 1", 0644},
		ft.File{"random-dir/another-file", "content 2", 0600},
		ft.Dir{"random-dir/another-dir", 0750},
		ft.Symlink{"random-dir/another-dir/some-symlink", "../some-file"},
	} {
		c.Logf("test %d: %#v", i, test)
		test.Check(c, targetParent)
	}

	fileInfos, err := ioutil.ReadDir(targetParent)
	c.Check(err, gc.IsNil)
	c.Check(fileInfos, gc.HasLen, 1)

	fileInfos, err = ioutil.ReadDir(targetPath)
	c.Check(err, gc.IsNil)
	c.Check(fileInfos, gc.HasLen, 3)
}

func (s *ZipSuite) TestExtractSingleFile(c *gc.C) {
	reader := s.makeZip(c,
		ft.Dir{"dir", 0755},
		ft.Dir{"dir/dir", 0755},
		ft.File{"dir/dir/some-file", "content 1", 0644},
		ft.File{"dir/dir/some-file-wtf", "content 2", 0644},
	)
	targetParent := c.MkDir()
	targetPath := filepath.Join(targetParent, "just-the-one-file")
	err := zip.Extract(reader, targetPath, "dir/dir/some-file")
	c.Assert(err, gc.IsNil)
	fileInfos, err := ioutil.ReadDir(targetParent)
	c.Check(err, gc.IsNil)
	c.Check(fileInfos, gc.HasLen, 1)
	ft.File{"just-the-one-file", "content 1", 0644}.Check(c, targetParent)
}

func (s *ZipSuite) TestClosesFile(c *gc.C) {
	reader := s.makeZip(c, ft.File{"f", "echo hullo!", 0755})
	targetPath := c.MkDir()
	err := zip.ExtractAll(reader, targetPath)
	c.Assert(err, gc.IsNil)
	cmd := exec.Command("/bin/sh", "-c", filepath.Join(targetPath, "f"))
	var buffer bytes.Buffer
	cmd.Stdout = &buffer
	err = cmd.Run()
	c.Assert(err, gc.IsNil)
	c.Assert(buffer.String(), gc.Equals, "hullo!\n")
}

func (s *ZipSuite) TestExtractSymlinkErrors(c *gc.C) {
	for i, test := range []struct {
		content []ft.Entry
		source  string
		error   string
	}{{
		content: []ft.Entry{
			ft.Dir{"dir", 0755},
			ft.Symlink{"dir/symlink", "/blah"},
		},
		source: "dir",
		error:  `cannot extract "dir/symlink": symlink "/blah" is absolute`,
	}, {
		content: []ft.Entry{
			ft.Dir{"dir", 0755},
			ft.Symlink{"dir/symlink", "../blah"},
		},
		source: "dir",
		error:  `cannot extract "dir/symlink": symlink "../blah" leads out of scope`,
	}, {
		content: []ft.Entry{
			ft.Symlink{"symlink", "blah"},
		},
		source: "symlink",
		error:  `cannot extract "symlink": symlink "blah" leads out of scope`,
	}} {
		c.Logf("test %d: %s", i, test.error)
		targetPath := c.MkDir()
		reader := s.makeZip(c, test.content...)
		err := zip.Extract(reader, targetPath, test.source)
		c.Check(err, gc.ErrorMatches, test.error)
	}
}

func (s *ZipSuite) TestExtractSourceError(c *gc.C) {
	reader := s.makeZip(c, ft.Dir{"dir", 0755})
	err := zip.Extract(reader, c.MkDir(), "../lol")
	c.Assert(err, gc.ErrorMatches, `cannot extract files rooted at "../lol"`)
}
