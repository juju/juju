// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backup_test

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	gc "launchpad.net/gocheck"

	"github.com/juju/juju/state/backup"
	"github.com/juju/juju/testing"
)

//---------------------------
// defaultFileName()

func (b *BackupSuite) TestDefaultFilename(c *gc.C) {
	filename := backup.DefaultFilename(nil)

	// This is a sanity check that no one accidentally
	// (or accidentally maliciously) breaks the default filename format.
	c.Check(filename, gc.Matches, `jujubackup-\d{8}-\d{6}\..*`)
	// The most crucial part is that the suffix is .tar.gz.
	c.Check(filename, gc.Matches, `.*\.tar\.gz$`)
}

func (b *BackupSuite) TestDefaultFilenameDateFormat(c *gc.C) {
	filename := backup.DefaultFilename(nil)

	var Y, M, D, h, m, s int
	template := fmt.Sprintf(backup.FilenameTemplate, backup.TimestampFormat)
	_, err := fmt.Sscanf(filename, template, &Y, &M, &D, &h, &m, &s)
	c.Assert(err, gc.IsNil)

	timestamp := time.Date(Y, time.Month(M), D, h, m, s, 0, time.UTC)
	elapsed := int(time.Since(timestamp)) / int(time.Second)
	c.Check(elapsed < 10, gc.Equals, true)
}

func (b *BackupSuite) TestDefaultFilenameUnique(c *gc.C) {
	filename1 := backup.DefaultFilename(nil)
	time.Sleep(1 * time.Second)
	filename2 := backup.DefaultFilename(nil)

	c.Check(filename1, gc.Not(gc.Equals), filename2)
}

//---------------------------
// CreateEmptyFile()

func (b *BackupSuite) TestCreateEmptyFileFilenameExplicit(c *gc.C) {
	requested := filepath.Join(c.MkDir(), "backup.tar.gz")
	_, filename, err := backup.CreateEmptyFile(requested, false)
	c.Check(err, gc.IsNil)

	c.Check(filename, gc.Equals, requested)
}

func (b *BackupSuite) TestCreateEmptyFileFilenameDefault(c *gc.C) {
	file, filename, err := backup.CreateEmptyFile("", false)
	defer os.Remove(filename)
	c.Check(err, gc.IsNil)
	c.Check(file, gc.NotNil)
	c.Check(filename, gc.Matches, `jujubackup-\d{8}-\d{6}\.tar\.gz`)
}

func (b *BackupSuite) TestCreateEmptyFileFilenameDirDefault(c *gc.C) {
	file, filename, err := backup.CreateEmptyFile("/tmp/", false)
	defer os.Remove(filename)
	c.Check(err, gc.IsNil)
	c.Check(file, gc.NotNil)
	c.Check(filename, gc.Matches, `/tmp/jujubackup-\d{8}-\d{6}\.tar\.gz`)
}

func (b *BackupSuite) TestCreateEmptyFile(c *gc.C) {
	requested := filepath.Join(c.MkDir(), "backup.tar.gz")
	file, filename, err := backup.CreateEmptyFile(requested, false)
	c.Check(err, gc.IsNil)
	err = file.Close()
	c.Assert(err, gc.IsNil)

	file, err = os.Open(filename)
	c.Assert(err, gc.IsNil)

	buffer := make([]byte, 10)
	size, err := file.Read(buffer)
	c.Check(err, gc.Equals, io.EOF)
	c.Check(size, gc.Equals, 0)
}

func (b *BackupSuite) TestCreateEmptyFileReallyEmpty(c *gc.C) {
	requested := filepath.Join(c.MkDir(), "backup.tar.gz")
	file, _, err := backup.CreateEmptyFile(requested, false)
	c.Check(err, gc.IsNil)

	buffer := make([]byte, 10)
	size, err := file.Read(buffer)
	c.Check(err, gc.Equals, io.EOF)
	c.Check(size, gc.Equals, 0)
}

func (b *BackupSuite) TestCreateEmptyFileAlreadyExists(c *gc.C) {
	requested := filepath.Join(c.MkDir(), "backup.tar.gz")
	file, err := os.Create(requested)
	c.Assert(err, gc.IsNil)
	err = file.Close()
	c.Assert(err, gc.IsNil)

	_, _, err = backup.CreateEmptyFile(requested, false)
	c.Check(err, gc.IsNil)
}

func (b *BackupSuite) TestCreateEmptyFileAlreadyExistsExclusive(c *gc.C) {
	requested := filepath.Join(c.MkDir(), "backup.tar.gz")
	file, err := os.Create(requested)
	c.Assert(err, gc.IsNil)
	err = file.Close()
	c.Assert(err, gc.IsNil)

	_, _, err = backup.CreateEmptyFile(requested, true)
	c.Check(err, gc.ErrorMatches, "could not create backup file: .*")
}

//---------------------------
// WriteBackup()

func (b *BackupSuite) TestWriteBackup(c *gc.C) {
	infile := bytes.NewBufferString("<compressed data>")
	archive := bytes.Buffer{}
	hash, err := backup.WriteBackup(&archive, infile)
	c.Check(err, gc.IsNil)

	c.Check(hash, gc.Equals, "787b8915389d921fa23fb40e16ae81ea979758bf")
	c.Check(archive.String(), gc.Equals, "<compressed data>")
}

func (b *BackupSuite) TestWriteBackupBadReader(c *gc.C) {
	infile := testing.FakeFile{ReadError: "failed to read"}
	archive := bytes.Buffer{}
	_, err := backup.WriteBackup(&archive, &infile)
	c.Check(err, gc.ErrorMatches, "could not write to the backup file: failed to read")
	c.Check(archive.String(), gc.Equals, "")
}

func (b *BackupSuite) TestWriteBackupBadWriter(c *gc.C) {
	infile := bytes.NewBufferString("<compressed data>")
	archive := testing.FakeFile{WriteError: "failed to write"}
	_, err := backup.WriteBackup(&archive, infile)
	c.Check(err, gc.ErrorMatches, "could not write to the backup file: failed to write")
}
