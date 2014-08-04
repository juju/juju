// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backup_test

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

	gc "launchpad.net/gocheck"

	"github.com/juju/juju/testing"
)

type expectedTarContents struct {
	Name string
	Body string
}

func shaSumFile(c *gc.C, fileToSum string) string {
	var f io.ReadCloser
	var err error

	f, err = os.Open(fileToSum)
	c.Assert(err, gc.IsNil)
	defer f.Close()

	shahash := sha1.New()
	_, err = io.Copy(shahash, f)
	c.Assert(err, gc.IsNil)
	return base64.StdEncoding.EncodeToString(shahash.Sum(nil))
}

//---------------------------
// the test suite

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

// Assert thar contents checks that the tar[.gz] file provided contains the
// Expected files
// expectedContents: is a slice of the filenames with relative paths that are
// expected to be on the tar file
// tarFile: is the path of the file to be checked
func (b *BackupSuite) checkTarContents(
	c *gc.C, expectedContents []expectedTarContents, tarFile string, compressed bool,
) bool {
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
		c.Log(fmt.Sprintf("archived: %q -> %s", hdr.Name, buf))
	}
	c.Log("checking...")

	res := true
	for _, expectedContent := range expectedContents {
		fullExpectedContent := strings.TrimPrefix(expectedContent.Name, string(os.PathSeparator))
		c.Log(fmt.Sprintf("checking for presence of %q on tar file", fullExpectedContent))
		body, found := tarContents[fullExpectedContent]
		res = c.Check(found, gc.Equals, true) && res
		if expectedContent.Body != "" {
			c.Log("Also checking the file contents")
			res = c.Check(body, gc.Equals, expectedContent.Body) && res
		}
	}
	return res
}
