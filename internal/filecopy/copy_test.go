// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package filecopy

import (
	"archive/tar"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/juju/tc"
)

type fileCopySuite struct{}

func TestFileCopySuite(t *testing.T) {
	tc.Run(t, &fileCopySuite{})
}

func (s *fileCopySuite) TestMakeTarFile(c *tc.C) {
	source := filepath.Join(c.MkDir(), "source")
	c.Assert(os.WriteFile(source, []byte("file contents"), 0644), tc.ErrorIsNil)

	var archive bytes.Buffer
	c.Assert(MakeTar(source, "/target/file", &archive), tc.ErrorIsNil)

	reader := tar.NewReader(&archive)
	header, err := reader.Next()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(header.Name, tc.Equals, "file")
	c.Check(header.FileInfo().Mode().IsRegular(), tc.IsTrue)
	contents, err := io.ReadAll(reader)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(string(contents), tc.Equals, "file contents")

	_, err = reader.Next()
	c.Check(err, tc.ErrorIs, io.EOF)
}

func (s *fileCopySuite) TestMakeTarDirectory(c *tc.C) {
	source := filepath.Join(c.MkDir(), "source")
	c.Assert(os.MkdirAll(filepath.Join(source, "nested"), 0755), tc.ErrorIsNil)
	c.Assert(os.Mkdir(filepath.Join(source, "empty"), 0755), tc.ErrorIsNil)
	c.Assert(os.WriteFile(filepath.Join(source, "file"), []byte("file"), 0644), tc.ErrorIsNil)
	c.Assert(os.WriteFile(filepath.Join(source, "nested", "file"), []byte("nested"), 0644), tc.ErrorIsNil)
	c.Assert(os.Symlink("file", filepath.Join(source, "link")), tc.ErrorIsNil)

	var archive bytes.Buffer
	c.Assert(MakeTar(source, "/target/", &archive), tc.ErrorIsNil)

	entries := readTarEntries(c, &archive)
	c.Check(len(entries), tc.Equals, 4)

	c.Check(entries["target/file"].contents, tc.Equals, "file")
	c.Check(entries["target/nested/file"].contents, tc.Equals, "nested")
	c.Check(entries["target/empty"].typeflag, tc.Equals, byte(tar.TypeDir))
	c.Check(entries["target/link"].typeflag, tc.Equals, byte(tar.TypeSymlink))
	c.Check(entries["target/link"].linkname, tc.Equals, "file")
}

func (s *fileCopySuite) TestUntarAll(c *tc.C) {
	archive := makeTarArchive(c, []tarEntry{
		{name: "remote/root/file", contents: "file contents"},
		{name: "remote/root/nested/", typeflag: tar.TypeDir},
		{name: "remote/root/nested/file", contents: "nested contents"},
		{name: "remote/root/link", typeflag: tar.TypeSymlink, linkname: "../../outside"},
		{name: "remote/root/../escape", contents: "must not be extracted"},
	})

	destRoot := c.MkDir()
	dest := filepath.Join(destRoot, "dest")
	c.Assert(UntarAll("/remote/root", bytes.NewReader(archive), dest), tc.ErrorIsNil)

	contents, err := os.ReadFile(filepath.Join(dest, "file"))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(string(contents), tc.Equals, "file contents")
	contents, err = os.ReadFile(filepath.Join(dest, "nested", "file"))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(string(contents), tc.Equals, "nested contents")

	_, err = os.Lstat(filepath.Join(dest, "link"))
	c.Check(err, tc.ErrorIs, os.ErrNotExist)
	_, err = os.Stat(filepath.Join(destRoot, "escape"))
	c.Check(err, tc.ErrorIs, os.ErrNotExist)
}

func (s *fileCopySuite) TestUntarAllRejectsCorruptArchive(c *tc.C) {
	archive := makeTarArchive(c, []tarEntry{{name: "other/file", contents: "contents"}})
	c.Check(
		UntarAll("/remote/root", bytes.NewReader(archive), c.MkDir()),
		tc.ErrorMatches,
		"tar contents corrupted",
	)

	c.Check(
		UntarAll("/remote/root", bytes.NewReader([]byte("not a tar")), c.MkDir()),
		tc.ErrorMatches,
		"unexpected EOF",
	)
}

func (s *fileCopySuite) TestGetPrefix(c *tc.C) {
	tests := []struct {
		path   string
		expect string
	}{
		{path: "/var/lib/juju", expect: "var/lib/juju"},
		{path: "var/lib/juju", expect: "var/lib/juju"},
		{path: "////", expect: ""},
	}
	for _, test := range tests {
		c.Check(getPrefix(test.path), tc.Equals, test.expect)
	}
}

func (s *fileCopySuite) TestStripPathShortcuts(c *tc.C) {
	tests := []struct {
		path   string
		expect string
	}{
		{path: "../file", expect: "file"},
		{path: "../../file", expect: "file"},
		{path: "a/../../file", expect: "file"},
		{path: "/file", expect: "file"},
		{path: ".", expect: ""},
		{path: "..", expect: ""},
	}
	for _, test := range tests {
		c.Check(stripPathShortcuts(test.path), tc.Equals, test.expect)
	}
}

func (s *fileCopySuite) TestIsDestRelative(c *tc.C) {
	base := filepath.Join(c.MkDir(), "base")
	tests := []struct {
		dest   string
		expect bool
	}{
		{dest: base, expect: true},
		{dest: filepath.Join(base, "file"), expect: true},
		{dest: filepath.Join(base, "nested", "file"), expect: true},
		{dest: filepath.Join(base, "..", "outside"), expect: false},
		{dest: filepath.Join(base, "nested", "..", "..", "outside"), expect: false},
	}
	for _, test := range tests {
		c.Check(isDestRelative(base, test.dest), tc.Equals, test.expect)
	}
}

type tarEntry struct {
	name     string
	contents string
	typeflag byte
	linkname string
}

func makeTarArchive(c *tc.C, entries []tarEntry) []byte {
	var archive bytes.Buffer
	tw := tar.NewWriter(&archive)
	for _, entry := range entries {
		header := &tar.Header{
			Name:     entry.name,
			Mode:     0644,
			Size:     int64(len(entry.contents)),
			Typeflag: entry.typeflag,
			Linkname: entry.linkname,
		}
		if entry.typeflag == tar.TypeDir {
			header.Mode = 0755
			header.Size = 0
		}
		c.Assert(tw.WriteHeader(header), tc.ErrorIsNil)
		if entry.typeflag == 0 {
			_, err := io.WriteString(tw, entry.contents)
			c.Assert(err, tc.ErrorIsNil)
		}
	}
	c.Assert(tw.Close(), tc.ErrorIsNil)
	return archive.Bytes()
}

func readTarEntries(c *tc.C, archive io.Reader) map[string]tarEntry {
	entries := make(map[string]tarEntry)
	reader := tar.NewReader(archive)
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		c.Assert(err, tc.ErrorIsNil)
		contents, err := io.ReadAll(reader)
		c.Assert(err, tc.ErrorIsNil)
		entries[header.Name] = tarEntry{
			name:     header.Name,
			contents: string(contents),
			typeflag: header.Typeflag,
			linkname: header.Linkname,
		}
	}
	return entries
}
