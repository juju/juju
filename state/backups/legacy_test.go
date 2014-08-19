// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"archive/tar"
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

type LegacySuite struct {
	testing.BaseSuite

	ranCommand bool
}

func (s *LegacySuite) createTestFiles(c *gc.C) (string, []string) {
	rootDir := c.MkDir()

	tarDirE := path.Join(rootDir, "TarDirectoryEmpty")
	err := os.Mkdir(tarDirE, os.FileMode(0755))
	c.Check(err, gc.IsNil)

	tarDirP := path.Join(rootDir, "TarDirectoryPopulated")
	err = os.Mkdir(tarDirP, os.FileMode(0755))
	c.Check(err, gc.IsNil)

	tarSubFile1 := path.Join(tarDirP, "TarSubFile1")
	tarSubFile1Handle, err := os.Create(tarSubFile1)
	c.Check(err, gc.IsNil)
	tarSubFile1Handle.WriteString("TarSubFile1")
	tarSubFile1Handle.Close()

	tarSubDir := path.Join(tarDirP, "TarDirectoryPopulatedSubDirectory")
	err = os.Mkdir(tarSubDir, os.FileMode(0755))
	c.Check(err, gc.IsNil)

	tarFile1 := path.Join(rootDir, "TarFile1")
	tarFile1Handle, err := os.Create(tarFile1)
	c.Check(err, gc.IsNil)
	tarFile1Handle.WriteString("TarFile1")
	tarFile1Handle.Close()

	tarFile2 := path.Join(rootDir, "TarFile2")
	tarFile2Handle, err := os.Create(tarFile2)
	c.Check(err, gc.IsNil)
	tarFile2Handle.WriteString("TarFile2")
	tarFile2Handle.Close()

	return rootDir, []string{tarDirE, tarDirP, tarFile1, tarFile2}
}

func (s *LegacySuite) patchSources(c *gc.C, testFiles []string) {
	s.PatchValue(backups.GetMongodumpPath, func() (string, error) {
		return "bogusmongodump", nil
	})

	s.PatchValue(backups.GetFilesToBackup, func(string) ([]string, error) {
		return testFiles, nil
	})

	s.PatchValue(backups.RunCommand, func(cmd string, args ...string) error {
		s.ranCommand = true
		return nil
	})
}

type tarContent struct {
	Name string
	Body string
}

var testExpectedTarContents = []tarContent{
	{"TarDirectoryEmpty", ""},
	{"TarDirectoryPopulated", ""},
	{"TarDirectoryPopulated/TarSubFile1", "TarSubFile1"},
	{"TarDirectoryPopulated/TarDirectoryPopulatedSubDirectory", ""},
	{"TarFile1", "TarFile1"},
	{"TarFile2", "TarFile2"},
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

func (s *LegacySuite) checkArchive(c *gc.C, file *os.File) {
	expected := []tarContent{
		{"juju-backup", ""},
		{"juju-backup/dump", ""},
		{"juju-backup/root.tar", ""},
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

type legacySuite struct {
	LegacySuite
}

var _ = gc.Suite(&legacySuite{})

func (s *legacySuite) TestBackup(c *gc.C) {
	_, testFiles := s.createTestFiles(c)
	s.patchSources(c, testFiles)

	outDir := c.MkDir()
	bkpFile, shaSum, err := backups.Backup(
		"boguspassword",
		"bogus-user",
		outDir,
		"localhost:8080",
	)
	c.Check(err, gc.IsNil)

	c.Assert(s.ranCommand, gc.Equals, true)

	// It is important that the filename uses non-special characters
	// only because it is returned in a header (unencoded) by the
	// backup API call. This also avoids compatibility problems with
	// client side filename conventions.
	c.Check(bkpFile, gc.Matches, `^[a-z0-9_.-]+$`)

	// Check the result.
	filename := filepath.Join(outDir, bkpFile)
	file, err := os.Open(filename)
	c.Assert(err, gc.IsNil)

	s.checkChecksum(c, file, shaSum, "")
	s.checkArchive(c, file)
}

func (s *legacySuite) TestStorageName(c *gc.C) {
	c.Check(backups.StorageName("foo"), gc.Equals, "/backups/foo")
	c.Check(backups.StorageName("/foo/bar"), gc.Equals, "/backups/bar")
	c.Check(backups.StorageName("foo/bar"), gc.Equals, "/backups/bar")
}
