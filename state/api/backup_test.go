// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api_test

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"

	gc "launchpad.net/gocheck"

	"github.com/juju/juju/state/api"
	"github.com/juju/juju/state/apiserver"
	"github.com/juju/juju/state/backup"
	"github.com/juju/juju/testing"
)

const tempPrefix = "test-juju-backup-client-"

//---------------------------
// test helpers

func getBackupResponse(data string, digest string) *http.Response {
	resp := http.Response{}

	resp.Body = &testing.CloseableBuffer{bytes.NewBufferString(data)}

	resp.Header = http.Header{}
	if digest == "" && data != "" {
		hash, _ := backup.GetHashUncompressed(resp.Body, "application/x-tar-gz")
		resp.Header.Set("Digest", "SHA="+hash)
	} else {
		resp.Header.Set("Digest", digest)
	}

	return &resp
}

func (s *clientSuite) setBackupServerSuccess() {
	succeed := func(pw, user, dir, addr string) (string, string, error) {
		backupFilePath := filepath.Join(dir, "testBackupFile")

		archive, err := os.Create(backupFilePath)
		if err != nil {
			return "", "", err
		}
		defer archive.Close()
		s.removeBackupArchive(backupFilePath)

		compressed := gzip.NewWriter(archive)
		defer compressed.Close()

		compressed.Write([]byte("foobarbam"))

		archive.Seek(0, os.SEEK_SET)
		hash, err := backup.GetHashUncompressed(archive, "application/gzip")
		if err != nil {
			return "", "", err
		}

		return backupFilePath, hash, nil
	}
	s.PatchValue(&apiserver.Backup, succeed)
}

func (s *clientSuite) setBackupServerError(err error) {
	fail := func(pw, user, dir, addr string) (string, string, error) {
		return "", "", err
	}
	s.PatchValue(&apiserver.Backup, fail)
}

func (s *clientSuite) removeBackupArchive(filename string) {
	s.AddCleanup(func(*gc.C) { os.Remove(filename) })
}

func (s *clientSuite) assertFilenameMatchesDefault(c *gc.C, filename string) {
	c.Assert(filename, gc.Matches, fmt.Sprintf(backup.FilenameTemplate, ".*"))
}

//---------------------------
// Success tests

func (s *clientSuite) TestBackupValid(c *gc.C) {
	s.setBackupServerSuccess()
	client := s.APIState.Client()

	filenameOrig := fmt.Sprintf(backup.FilenameTemplate, "20140623-010101")
	validate := false
	filename, err := client.Backup(filenameOrig, validate)
	if filename != "" {
		s.removeBackupArchive(filename)
	}

	c.Check(err, gc.IsNil)

	c.Check(filename, gc.Equals, filenameOrig)
}

func (s *clientSuite) TestBackupDefaultFilename(c *gc.C) {
	s.setBackupServerSuccess()
	client := s.APIState.Client()

	validate := false
	filename, err := client.Backup("", validate)
	if filename != "" {
		s.removeBackupArchive(filename)
	}

	c.Assert(err, gc.IsNil)
	s.assertFilenameMatchesDefault(c, filename)
}

func (s *clientSuite) TestBackupValidateHashSuccess(c *gc.C) {
	hash := "kVi4iZOb1a7A9SBOPIv7ShWUKIU="
	data := "...archive data..."

	archive, _ := ioutil.TempFile("", tempPrefix)
	defer archive.Close()
	filename := archive.Name()
	s.removeBackupArchive(filename)

	archive.Write([]byte(data))

	resp := getBackupResponse(data, "SHA="+hash)
	err := api.ValidateBackupHash(filename, resp)

	c.Assert(err, gc.IsNil)
}

//---------------------------
// Failure tests

// XXX How to test these?
/*
func (s *clientSuite) TestBackupOutfileNotCreated(c *gc.C) {
	c.Error("not implemented")
}

func (s *clientSuite) TestBackupOutfileNotWritten(c *gc.C) {
	c.Error("not implemented")
}
*/

func (s *clientSuite) TestBackupDigestHeaderMissing(c *gc.C) {
	resp := http.Response{}
	err := api.ValidateBackupHash("", &resp)

	c.Assert(err, gc.ErrorMatches, "could not verify backup file: SHA digest missing from response")
}

func (s *clientSuite) TestBackupDigestHeaderEmpty(c *gc.C) {
	resp := getBackupResponse("", "")
	err := api.ValidateBackupHash("", resp)

	c.Assert(err, gc.ErrorMatches, "could not verify backup file: SHA digest missing from response")
}

func (s *clientSuite) TestBackupDigestHeaderInvalid(c *gc.C) {
	resp := getBackupResponse("", "not valid SHA hash")
	err := api.ValidateBackupHash("", resp)

	c.Assert(err, gc.ErrorMatches, "could not verify backup file: unrecognized Digest header (expected \"SHA=\")")
}

func (s *clientSuite) TestBackupOutfileNotOpened(c *gc.C) {
	resp := getBackupResponse("...archive data...", "")
	err := api.ValidateBackupHash("/tmp/backup-does-not-exist.tar.gz", resp)

	c.Assert(err, gc.ErrorMatches, "could not verify backup file: unable to open: .*")
}

// XXX How to test this?  (patching GetHash does not work)
func (s *clientSuite) TestBackupHashNotExtracted(c *gc.C) {
	gethash := func(filename string) (string, error) {
		return "", fmt.Errorf("unable to extract hash: error!")
	}
	s.PatchValue(api.GetHashByFilename, gethash)

	archive, _ := ioutil.TempFile("", tempPrefix)
	defer archive.Close()
	filename := archive.Name()
	s.removeBackupArchive(filename)

	resp := getBackupResponse("...", "")
	err := api.ValidateBackupHash(filename, resp)

	c.Assert(err, gc.ErrorMatches, "unable to extract hash: .*")
}

func (s *clientSuite) TestBackupHashMismatch(c *gc.C) {
	hash := "invalid hash"
	data := "...archive data..."

	archive, _ := ioutil.TempFile("", tempPrefix)
	defer archive.Close()
	filename := archive.Name()
	s.removeBackupArchive(filename)

	archive.Write([]byte(data))

	resp := getBackupResponse(data, "SHA="+hash)
	err := api.ValidateBackupHash(filename, resp)

	c.Assert(err, gc.ErrorMatches, "archive hash did not match value from server: .*")
}

//---------------------------
// System tests

/*
1) the filename is created on disk
2) The content of the filename is not nil
3) It is a valid tarball
4) The hash matches expectations (though presumably that's already covered by other code)
5) we could assert that some of the filenames in the tarball match what we expect to be in a backup.
*/

// XXX How to test this?
func (s *clientSuite) TestBackup(c *gc.C) {
	client := s.APIState.Client()
	expected := fmt.Sprintf(backup.FilenameTemplate, "20140623-010101")

	validate := false
	filename, err := client.Backup(expected, validate)
	c.Assert(err, gc.IsNil)
	c.Assert(filename, gc.Equals, expected)
}
