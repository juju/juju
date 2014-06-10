// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backup_test

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	stdtesting "testing"

	gc "launchpad.net/gocheck"

	"github.com/juju/juju/state/backup"
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

func (b *BackupSuite) TearDownTest(c *gc.C) {
	b.BaseSuite.TearDownTest(c)
}

func (b *BackupSuite) TestWritesIntoBuff(c *gc.C) {
	fileWithContentPath := path.Join(b.cwd, "TestWritesIntoBuff.txt")
	fileWithContent, err := os.Create(fileWithContentPath)
	c.Check(err, gc.IsNil)
	testString := "A bogus content"
	fileWithContent.Write([]byte(testString))
	fileWithContent.Close()

	var buf bytes.Buffer
	err = backup.WriteInto(fileWithContentPath, &buf)

	c.Check(err, gc.IsNil)
	c.Assert(buf.String(), gc.DeepEquals, testString)
}

func (b *BackupSuite) createTestFiles(c *gc.C) {
	tarDir1 := path.Join(b.cwd, "TarDirectory1")
	err := os.Mkdir(tarDir1, os.FileMode(0755))
	c.Check(err, gc.IsNil)
	tarFile1 := path.Join(b.cwd, "TarFile1")
	tarFile1Handle, err := os.Create(tarFile1)
	c.Check(err, gc.IsNil)
	tarFile1Handle.Close()
	tarFile2 := path.Join(b.cwd, "TarFile2")
	tarFile2Handle, err := os.Create(tarFile2)
	c.Check(err, gc.IsNil)
	tarFile2Handle.Close()
	b.testFiles = []string{tarDir1, tarFile1, tarFile2}

}

func (b *BackupSuite) TestTarsFiles(c *gc.C) {
	b.createTestFiles(c)
	outputTar := path.Join(b.cwd, "output_tar_file.tar")
	shaSum, err := backup.TarFiles(b.testFiles, outputTar)
	c.Check(err, gc.IsNil)
	c.Assert(shaSum, gc.Equals, "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855")

}

func shaSumGzipContents(c *gc.C, gzPath string) string {
	f, err := os.Open(gzPath)
	c.Check(err, gc.IsNil)
	defer f.Close()
	gz, err := gzip.NewReader(f)
	c.Check(err, gc.IsNil)
	defer gz.Close()

	var buf []byte
	_, err = gz.Read(buf)
	c.Check(err, gc.IsNil)

	sha256hash := sha256.New()

	bufR := bytes.NewBuffer(buf)
	_, err = io.Copy(sha256hash, bufR)
	c.Check(err, gc.IsNil)

	return fmt.Sprintf("%x", sha256hash.Sum(nil))

}

func (b *BackupSuite) TestTarGzsFiles(c *gc.C) {
	b.createTestFiles(c)

	outputTarGz := path.Join(b.cwd, "output_tar_file.tgz")
	_, err := backup.TarFiles(b.testFiles, outputTarGz)
	c.Check(err, gc.IsNil)

	shaSum := shaSumGzipContents(c, outputTarGz)
	c.Assert(shaSum, gc.Equals, "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855")
}

func (b *BackupSuite) TestBackUp(c *gc.C) {
	b.createTestFiles(c)
	ranCommand := false
	backup.GetFilesToBackup = func() ([]string, error) { return b.testFiles, nil }
	backup.RunCommand = func(command string) (string, error) {
		ranCommand = true
		return "", nil
	}
	bkpFile, shaFile, err := backup.BackUp("boguspassword", b.cwd, 123456)
	c.Check(err, gc.IsNil)
	c.Assert(ranCommand, gc.Equals, true)

	bkpSha, err := ioutil.ReadFile(path.Join(b.cwd, shaFile))
	c.Check(err, gc.IsNil)
	bkpFileR, err := os.Open(path.Join(b.cwd, bkpFile))
	c.Check(err, gc.IsNil)
	defer bkpFileR.Close()
	sha256hash := sha256.New()
	io.Copy(sha256hash, bkpFileR)
	shaSum := fmt.Sprintf("%x", sha256hash.Sum(nil))

	c.Assert(string(bkpSha), gc.Equals, shaSum)

	tarShaSum := shaSumGzipContents(c, path.Join(b.cwd, bkpFile))
	c.Assert(tarShaSum, gc.Equals, "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855")

}
