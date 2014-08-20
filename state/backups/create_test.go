// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"

	gc "launchpad.net/gocheck"

	"github.com/juju/juju/state/backups"
	"github.com/juju/juju/testing"
)

type createSuite struct {
	LegacySuite
}

var _ = gc.Suite(&createSuite{}) // Register the suite.

func (s *createSuite) TestCreateLegacy(c *gc.C) {
	_, testFiles, expected := s.createTestFiles(c)
	s.patchSources(c, testFiles)

	dumper := backups.NewTestDBDumper()
	args := backups.NewTestCreateArgs(testFiles, dumper)
	result, err := backups.Create(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.NotNil)

	archiveFile, size, checksum := backups.DumpCreateResult(result)
	c.Assert(archiveFile, gc.NotNil)

	// Check the result.
	file, ok := archiveFile.(*os.File)
	c.Assert(ok, gc.Equals, true)

	s.checkSize(c, file, size, 0)
	s.checkChecksum(c, file, checksum, "")
	s.checkArchive(c, file, expected)
}

type LegacySuite struct {
	testing.BaseSuite
}

type tarContent struct {
	Name   string
	Body   string
	Nested []tarContent
}

var expectedContents = map[string]string{
	"TarDirectoryEmpty":                     "",
	"TarDirectoryPopulated":                 "",
	"TarDirectoryPopulated/TarSubFile1":     "TarSubFile1",
	"TarDirectoryPopulated/TarSubDirectory": "",
	"TarFile1":                              "TarFile1",
	"TarFile2":                              "TarFile2",
}

func (s *LegacySuite) createTestFiles(c *gc.C) (string, []string, []tarContent) {
	var expected []tarContent
	var tempFiles []string

	rootDir := c.MkDir()

	for name, body := range expectedContents {
		filename := filepath.Join(rootDir, filepath.FromSlash(name))

		top := (path.Dir(name) == ".")

		if body == "" {
			err := os.MkdirAll(filename, os.FileMode(0755))
			c.Check(err, gc.IsNil)
		} else {
			if !top {
				err := os.MkdirAll(filepath.Dir(filename), os.FileMode(0755))
				c.Check(err, gc.IsNil)
			}
			file, err := os.Create(filename)
			c.Assert(err, gc.IsNil)
			file.WriteString(body)
			file.Close()
		}

		if top {
			tempFiles = append(tempFiles, filename)
		}
		content := tarContent{filepath.ToSlash(filename), body, nil}
		expected = append(expected, content)
	}

	return rootDir, tempFiles, expected
}

func (s *LegacySuite) patchSources(c *gc.C, testFiles []string) {
	s.PatchValue(backups.GetFilesToBackup, func(string) ([]string, error) {
		return testFiles, nil
	})
}

func readTarFile(c *gc.C, tarFile io.Reader) map[string]string {
	tr := tar.NewReader(tarFile)
	contents := make(map[string]string)

	// Iterate through the files in the archive.
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			// end of tar archive
			break
		}
		c.Assert(err, gc.IsNil)
		buf, err := ioutil.ReadAll(tr)
		c.Assert(err, gc.IsNil)
		contents[hdr.Name] = string(buf)
	}

	return contents
}

func (s *LegacySuite) checkTarContents(
	c *gc.C, tarFile io.Reader, allExpected []tarContent,
) {
	contents := readTarFile(c, tarFile)

	// Check that the expected entries are there.
	// XXX Check for unexpected entries.
	for _, expected := range allExpected {
		relPath := strings.TrimPrefix(expected.Name, string(os.PathSeparator))

		c.Log(fmt.Sprintf("checking for presence of %q on tar file", relPath))
		body, found := contents[relPath]
		if !found {
			c.Errorf("%q not found", expected.Name)
			continue
		}

		if expected.Body != "" {
			c.Log("Also checking the file contents")
			c.Check(body, gc.Equals, expected.Body)
		}

		if expected.Nested != nil {
			c.Log("Also checking the nested tar file")
			nestedFile := bytes.NewBufferString(body)
			s.checkTarContents(c, nestedFile, expected.Nested)
		}
	}

	if c.Failed() {
		c.Log("-----------------------")
		c.Log("expected:")
		for _, expected := range allExpected {
			c.Log(fmt.Sprintf("%s -> %q", expected.Name, expected.Body))
		}
		c.Log("got:")
		for name, body := range contents {
			if len(body) > 200 {
				body = body[:200] + "...(truncated)"
			}
			c.Log(fmt.Sprintf("%s -> %q", name, body))
		}
	}
}

func (s *LegacySuite) checkChecksum(c *gc.C, file *os.File, checksum, expected string) {
	if expected == "" {
		expected = checksum
	}

	c.Check(checksum, gc.Equals, expected)

	fileShaSum := shaSumFile(c, file)
	c.Check(fileShaSum, gc.Equals, expected)
	resetFile(c, file)
}

func (s *LegacySuite) checkSize(c *gc.C, file *os.File, size, expected int64) {
	if expected == 0 {
		expected = size
	}

	c.Check(size, gc.Equals, expected)

	stat, err := file.Stat()
	c.Assert(err, gc.IsNil)
	c.Check(stat.Size(), gc.Equals, expected)
}

func (s *LegacySuite) checkArchive(c *gc.C, file *os.File, bundle []tarContent) {
	expected := []tarContent{
		{"juju-backup", "", nil},
		{"juju-backup/dump", "", nil},
		{"juju-backup/root.tar", "", bundle},
	}

	tarFile, err := gzip.NewReader(file)
	c.Assert(err, gc.IsNil)

	s.checkTarContents(c, tarFile, expected)
	resetFile(c, file)
}

func resetFile(c *gc.C, reader io.Reader) {
	file, ok := reader.(*os.File)
	c.Assert(ok, gc.Equals, true)
	_, err := file.Seek(0, os.SEEK_SET)
	c.Assert(err, gc.IsNil)
}
