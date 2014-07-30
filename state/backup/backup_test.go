// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backup

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"io"
	"io/ioutil"
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
	tarSubFile1Handle.WriteString("TarSubFile1")
	tarSubFile1Handle.Close()

	tarSubDir := path.Join(tarDirP, "TarDirectoryPopulatedSubDirectory")
	err = os.Mkdir(tarSubDir, os.FileMode(0755))
	c.Check(err, gc.IsNil)

	tarFile1 := path.Join(b.cwd, "TarFile1")
	tarFile1Handle, err := os.Create(tarFile1)
	c.Check(err, gc.IsNil)
	tarFile1Handle.WriteString("TarFile1")
	tarFile1Handle.Close()

	tarFile2 := path.Join(b.cwd, "TarFile2")
	tarFile2Handle, err := os.Create(tarFile2)
	c.Check(err, gc.IsNil)
	tarFile2Handle.WriteString("TarFile2")
	tarFile2Handle.Close()
	b.testFiles = []string{tarDirE, tarDirP, tarFile1, tarFile2}

}

func (b *BackupSuite) removeTestFiles(c *gc.C) {
	for _, removable := range b.testFiles {
		err := os.RemoveAll(removable)
		c.Assert(err, gc.IsNil)
	}
}

type expectedTarContents struct {
	Name string
	Body string
}

var testExpectedTarContents = []expectedTarContents{
	{"TarDirectoryEmpty", ""},
	{"TarDirectoryPopulated", ""},
	{"TarDirectoryPopulated/TarSubFile1", "TarSubFile1"},
	{"TarDirectoryPopulated/TarDirectoryPopulatedSubDirectory", ""},
	{"TarFile1", "TarFile1"},
	{"TarFile2", "TarFile2"},
}

// Assert thar contents checks that the tar[.gz] file provided contains the
// Expected files
// expectedContents: is a slice of the filenames with relative paths that are
// expected to be on the tar file
// tarFile: is the path of the file to be checked
func (b *BackupSuite) assertTarContents(c *gc.C, expectedContents []expectedTarContents,
	tarFile string,
	compressed bool) {
	f, err := os.Open(tarFile)
	c.Assert(err, gc.IsNil)
	defer f.Close()
	var r io.Reader = f
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
		buf, err := ioutil.ReadAll(tr)
		c.Assert(err, gc.IsNil)
		tarContents[hdr.Name] = string(buf)
	}
	for _, expectedContent := range expectedContents {
		fullExpectedContent := strings.TrimPrefix(expectedContent.Name, string(os.PathSeparator))
		body, ok := tarContents[fullExpectedContent]
		c.Log(tarContents)
		c.Log(expectedContents)
		c.Log(fmt.Sprintf("checking for presence of %q on tar file", fullExpectedContent))
		c.Assert(ok, gc.Equals, true)
		if expectedContent.Body != "" {
			c.Log("Also checking the file contents")
			c.Assert(body, gc.Equals, expectedContent.Body)
		}
	}

}

func shaSumFile(c *gc.C, fileToSum string) string {
	f, err := os.Open(fileToSum)
	c.Assert(err, gc.IsNil)
	defer f.Close()
	shahash := sha1.New()
	_, err = io.Copy(shahash, f)
	c.Assert(err, gc.IsNil)
	return base64.StdEncoding.EncodeToString(shahash.Sum(nil))
}

func (b *BackupSuite) TestTarFilesUncompressed(c *gc.C) {
	b.createTestFiles(c)
	outputTar := path.Join(b.cwd, "output_tar_file.tar")
	trimPath := fmt.Sprintf("%s/", b.cwd)
	shaSum, err := tarFiles(b.testFiles, outputTar, trimPath, false)
	c.Check(err, gc.IsNil)
	fileShaSum := shaSumFile(c, outputTar)
	c.Assert(shaSum, gc.Equals, fileShaSum)
	b.removeTestFiles(c)
	b.assertTarContents(c, testExpectedTarContents, outputTar, false)
}

func (b *BackupSuite) TestTarFilesCompressed(c *gc.C) {
	b.createTestFiles(c)
	outputTarGz := path.Join(b.cwd, "output_tar_file.tgz")
	trimPath := fmt.Sprintf("%s/", b.cwd)
	shaSum, err := tarFiles(b.testFiles, outputTarGz, trimPath, true)
	c.Check(err, gc.IsNil)

	fileShaSum := shaSumFile(c, outputTarGz)
	c.Assert(shaSum, gc.Equals, fileShaSum)

	b.assertTarContents(c, testExpectedTarContents, outputTarGz, true)
}

func (b *BackupSuite) TestBackup(c *gc.C) {
	b.createTestFiles(c)
	ranCommand := false
	getMongodumpPath = func() (string, error) { return "bogusmongodump", nil }
	getFilesToBackup = func() ([]string, error) { return b.testFiles, nil }
	runCommand = func(command string, args ...string) error {
		ranCommand = true
		return nil
	}
	bkpFile, shaSum, err := Backup("boguspassword", "bogus-user", b.cwd, "localhost:8080")
	c.Check(err, gc.IsNil)
	c.Assert(ranCommand, gc.Equals, true)

	// It is important that the filename uses non-special characters
	// only because it is returned in a header (unencoded) by the
	// backup API call. This also avoids compatibility problems with
	// client side filename conventions.
	c.Check(bkpFile, gc.Matches, `^[a-z0-9_.-]+$`)

	fileShaSum := shaSumFile(c, path.Join(b.cwd, bkpFile))
	c.Assert(shaSum, gc.Equals, fileShaSum)

	bkpExpectedContents := []expectedTarContents{
		{"juju-backup", ""},
		{"juju-backup/dump", ""},
		{"juju-backup/root.tar", ""},
	}
	b.assertTarContents(c, bkpExpectedContents, path.Join(b.cwd, bkpFile), true)
}

func (b *BackupSuite) TestStorageName(c *gc.C) {
	c.Assert(StorageName("foo"), gc.Equals, "/backups/foo")
	c.Assert(StorageName("/foo/bar"), gc.Equals, "/backups/bar")
	c.Assert(StorageName("foo/bar"), gc.Equals, "/backups/bar")
}
