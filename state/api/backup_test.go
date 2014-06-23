// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api_test

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"

	gc "launchpad.net/gocheck"

	"github.com/juju/juju/state/api"
	"github.com/juju/juju/state/apiserver"
	"github.com/juju/juju/state/backup"
)

//---------------------------
// test helpers

type fakeFile struct {
	*bytes.Buffer
}

func (f *fakeFile) Close() error {
	return nil
}

func getBackupResponse(data string, digest string) *http.Response {
	resp := http.Response{}

	resp.Body = &fakeFile{bytes.NewBufferString(data)}

	resp.Header = http.Header{}
	if digest == "" && data != "" {
		hash, _ := backup.GetHash(resp.Body)
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

		archive.Write([]byte("foobarbam"))

		archive.Seek(0, os.SEEK_SET)
		hash, err := backup.GetHash(archive)
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

//---------------------------
// Success tests

func (s *clientSuite) TestBackupValid(c *gc.C) {
	s.setBackupServerSuccess()
	expected := fmt.Sprintf(backup.FilenameTemplate, "20140623-010101")
	client := s.APIState.Client()

	validate := false
	filename, err := client.Backup(expected, validate)
	if filename != "" {
		s.removeBackupArchive(filename)
	}

	c.Assert(err, gc.IsNil)
	c.Assert(filename, gc.Equals, expected)
}

func (s *clientSuite) TestBackupDefaultFilename(c *gc.C) {
	s.setBackupServerSuccess()
	expected := fmt.Sprintf(backup.FilenameTemplate, ".*")
	client := s.APIState.Client()

	validate := false
	filename, err := client.Backup("", validate)
	if filename != "" {
		s.removeBackupArchive(filename)
	}

	c.Assert(err, gc.IsNil)
	c.Assert(filename, gc.Matches, expected)
}

func (s *clientSuite) TestBackupValidateHashSuccess(c *gc.C) {
	hash := "kVi4iZOb1a7A9SBOPIv7ShWUKIU="
	data := "...archive data..."

	archive, _ := ioutil.TempFile("", "")
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

	c.Assert(err, gc.ErrorMatches, "SHA digest missing from response. Can't verify backup file.")
}

func (s *clientSuite) TestBackupDigestHeaderEmpty(c *gc.C) {
	resp := getBackupResponse("", "")
	err := api.ValidateBackupHash("", resp)

	c.Assert(err, gc.ErrorMatches, "SHA digest missing from response. Can't verify backup file.")
}

func (s *clientSuite) TestBackupDigestHeaderInvalid(c *gc.C) {
	resp := getBackupResponse("", "not valid SHA hash")
	err := api.ValidateBackupHash("", resp)

	c.Assert(err, gc.ErrorMatches, "SHA digest missing from response. Can't verify backup file.")
}

func (s *clientSuite) TestBackupOutfileNotOpened(c *gc.C) {
	resp := getBackupResponse("...archive data...", "")
	err := api.ValidateBackupHash("/tmp/backup-does-not-exist.tar.gz", resp)

	c.Assert(err, gc.ErrorMatches, "could not open backup file: .*")
}

// XXX How to test this?  (patching GetHash does not work)
/*
func (s *clientSuite) TestBackupHashNotExtracted(c *gc.C) {
    gethash := func(archive *os.File) (string, error) {
        return "", fmt.Errorf("unable to extract hash: error!")
    }
	s.PatchValue(backup.GetHash, gethash)

    archive, _ := ioutil.TempFile("", "")
    defer archive.Close()
    filename := archive.Name()
	s.removeBackupArchive(filename)

    resp := getBackupResponse("...", "")
    err := api.ValidateBackupHash(filename, resp)

    c.Assert(err, gc.ErrorMatches, "unable to extract hash: .*")
}
*/

func (s *clientSuite) TestBackupHashMismatch(c *gc.C) {
	hash := "invalid hash"
	data := "...archive data..."

	archive, _ := ioutil.TempFile("", "")
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

// XXX How to test this?
func (s *clientSuite) TestBackup(c *gc.C) {
	client := s.APIState.Client()
	expected := fmt.Sprintf(backup.FilenameTemplate, "20140623-010101")

	validate := false
	filename, err := client.Backup(expected, validate)
	c.Assert(err, gc.IsNil)
	c.Assert(filename, gc.Equals, expected)
}
