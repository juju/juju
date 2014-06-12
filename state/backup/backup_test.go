// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backup

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	stdtesting "testing"

	gc "launchpad.net/gocheck"

	"github.com/juju/juju/testing"
)

func Test(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}

var _ = gc.Suite(&BackupSuite{})

type BackupSuite struct {
	testing.BaseSuite
	cwd       string
	testFiles []string
}

func (b *BackupSuite) SetUpTest(c *gc.C) {
	b.cwd = c.MkDir()
	b.BaseSuite.SetUpTest(c)
}

func (b *BackupSuite) createTestFiles(c *gc.C) {
	tarDirE := path.Join(b.cwd, "TarDirectoryEmpty")
	err := os.Mkdir(tarDirE, os.FileMode(0755))
	c.Check(err, gc.IsNil)

	tarDirP := path.Join(b.cwd, "TarDirectoryPopulated")
	err = os.Mkdir(tarDirP, os.FileMode(0755))
	c.Check(err, gc.IsNil)

	tarSubFile1 := path.Join(tarDirP, "TarSubFile1")
	tarSubFile1Handle, err := os.Create(tarSubFile1)
	c.Check(err, gc.IsNil)
	tarSubFile1Handle.Close()

	tarSubDir := path.Join(tarDirP, "TarDirectoryPopulatedSubDirectory")
	err = os.Mkdir(tarSubDir, os.FileMode(0755))
	c.Check(err, gc.IsNil)

	tarFile1 := path.Join(b.cwd, "TarFile1")
	tarFile1Handle, err := os.Create(tarFile1)
	c.Check(err, gc.IsNil)
	tarFile1Handle.Close()

	tarFile2 := path.Join(b.cwd, "TarFile2")
	tarFile2Handle, err := os.Create(tarFile2)
	c.Check(err, gc.IsNil)
	tarFile2Handle.Close()
	b.testFiles = []string{tarDirE, tarDirP, tarFile1, tarFile2}

}

func (b *BackupSuite) removeTestFiles(c *gc.C) {
	for _, removable := range b.testFiles {
		err := os.RemoveAll(removable)
		c.Assert(err, gc.IsNil)
	}
}

var testExpectedTarContents = []string{
	"TarDirectoryEmpty",
	"TarDirectoryPopulated",
	"TarDirectoryPopulated/TarSubFile1",
	"TarDirectoryPopulated/TarDirectoryPopulatedSubDirectory",
	"TarFile1",
	"TarFile2",
}

func (b *BackupSuite) checkContents(c *gc.C, expectedContents []string, tarFile, prefix string, compressed bool) {
	f, err := os.Open(tarFile)
	c.Assert(err, gc.IsNil)
	var r io.Reader
	r = bufio.NewReader(f)
	if compressed {
		r, err = gzip.NewReader(r)
		c.Assert(err, gc.IsNil)
	}

	tr := tar.NewReader(r)

	tarContents := make(map[string]string)
	// Iterate through the files in the archive.
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			// end of tar archive
			break
		}
		c.Assert(err, gc.IsNil)
		tarContents[hdr.Name] = ""
	}
	for _, expectedContent := range expectedContents {
		fullExpectedContent := path.Join(prefix, expectedContent)
		fullExpectedContent = strings.TrimPrefix(fullExpectedContent, string(os.PathSeparator))
		_, ok := tarContents[fullExpectedContent]
		c.Log(tarContents)
		c.Log(expectedContents)
		c.Log(fmt.Sprintf("checking for presence of %q on tar file", fullExpectedContent))
		c.Assert(ok, gc.Equals, true)
	}

}

func shaSumFile(c *gc.C, fileToSum string) string {
	f, err := os.Open(fileToSum)
	c.Assert(err, gc.IsNil)
	defer f.Close()
	sha256hash := sha256.New()
	_, err = io.Copy(sha256hash, f)
	c.Assert(err, gc.IsNil)
	return fmt.Sprintf("%x", sha256hash.Sum(nil))
}

func (b *BackupSuite) TestTarsFiles(c *gc.C) {
	b.createTestFiles(c)
	outputTar := path.Join(b.cwd, "output_tar_file.tar")
	shaSum, err := tarFiles(b.testFiles, outputTar, "", false)
	c.Check(err, gc.IsNil)
	fileShaSum := shaSumFile(c, outputTar)
	c.Assert(shaSum, gc.Equals, fileShaSum)
	b.removeTestFiles(c)
	b.checkContents(c, testExpectedTarContents, outputTar, b.cwd, false)
}

func (b *BackupSuite) TestTarGzsFiles(c *gc.C) {
	b.createTestFiles(c)

	outputTarGz := path.Join(b.cwd, "output_tar_file.tgz")
	shaSum, err := tarFiles(b.testFiles, outputTarGz, "", true)
	c.Check(err, gc.IsNil)

	fileShaSum := shaSumFile(c, outputTarGz)
	c.Assert(shaSum, gc.Equals, fileShaSum)

	b.checkContents(c, testExpectedTarContents, outputTarGz, b.cwd, true)
}

func (b *BackupSuite) TestBackUp(c *gc.C) {
	b.createTestFiles(c)
	ranCommand := false
	getMongodumpPath = func() (string, error) { return "bogusmongodump", nil }
	getFilesToBackup = func() ([]string, error) { return b.testFiles, nil }
	runCommand = func(command string) (string, error) {
		ranCommand = true
		return "", nil
	}
	bkpFile, shaSum, err := Backup("boguspassword", b.cwd, 123456)
	c.Check(err, gc.IsNil)
	c.Assert(ranCommand, gc.Equals, true)
	fileShaSum := shaSumFile(c, path.Join(b.cwd, bkpFile))
	c.Assert(shaSum, gc.Equals, fileShaSum)
	expectedContents := []string{
		"juju-backup",
		"juju-backup/dump",
		"juju-backup/root.tar"}
	b.checkContents(c, expectedContents, path.Join(b.cwd, bkpFile), "", true)
}
