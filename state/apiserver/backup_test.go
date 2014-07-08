// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/juju/juju/environmentserver/authentication"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/api/params"
	"github.com/juju/juju/state/apiserver"
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"
)

type backupSuite struct {
	authHttpSuite
}

var _ = gc.Suite(&backupSuite{})

func (s *backupSuite) SetUpSuite(c *gc.C) {
	s.authHttpSuite.SetUpSuite(c)
	s.archiveContentType = "application/x-tar-gz"
	s.apiBinding = "backup"
	s.httpMethod = "POST"
}

func (s *backupSuite) sendValidRequest(c *gc.C) *http.Response {
	url, err := s.URL("")
	c.Assert(err, gc.IsNil)
	resp, err := s.sendURL(url)
	c.Assert(err, gc.IsNil)
	return resp
}

func (s *backupSuite) backupURL(c *gc.C) string {
	URL, err := s.URL("")
	c.Assert(err, gc.IsNil)
	return URL.String()
}

func (s *backupSuite) TestBackupHTTPHandling(c *gc.C) {
	var result params.Error
	s.checkServedSecurely(c)
	s.checkHTTPMethodInvalid(c, "GET", &result)
	s.checkHTTPMethodInvalid(c, "PUT", &result)

	s.checkRequiresAuth(c, &result)
	s.checkRequiresUser(c, &result)

	query := ""
	s.checkLegacyPathUnavailable(c, query)
}

func (s *backupSuite) TestBackupCalledAndFileServed(c *gc.C) {
	testGetMongoConnectionInfo := func(thisState *state.State) *authentication.ConnectionInfo {
		info := &authentication.ConnectionInfo{
			Password: "foobar",
			Tag:      "machine-0",
		}
		info.Addrs = append(info.Addrs, "localhost:80")
		return info
	}
	var data struct{ tempDir, mongoPassword, username, address string }
	testBackup := func(password string, username string, tempDir string, address string) (string, string, error) {
		data.tempDir = tempDir
		data.mongoPassword = password
		data.username = username
		data.address = address
		backupFilePath := filepath.Join(tempDir, "testBackupFile")
		file, err := os.Create(backupFilePath)
		if err != nil {
			return "", "", err
		}
		file.Write([]byte("foobarbam"))
		file.Close()
		return backupFilePath, "some-sha", nil
	}
	s.PatchValue(&apiserver.Backup, testBackup)
	s.PatchValue(&apiserver.GetMongoConnectionInfo, testGetMongoConnectionInfo)

	resp := s.sendValidRequest(c)

	// Check the response.
	s.checkFileResponse(c, resp, "foobarbam", "application/octet-stream")
	c.Check(resp.Header.Get("Digest"), gc.Equals, "SHA=some-sha")

	// Check the passed values.
	c.Check(data.tempDir, gc.NotNil)
	_, err := os.Stat(data.tempDir)
	c.Check(err, jc.Satisfies, os.IsNotExist)
	c.Check(data.mongoPassword, gc.Equals, "foobar")
	c.Check(data.username, gc.Equals, "machine-0")
	c.Check(data.address, gc.Equals, "localhost:80")
}

func (s *backupSuite) TestBackupErrorWhenBackupFails(c *gc.C) {
	var data struct{ tempDir string }
	testBackup := func(password string, username string, tempDir string, address string) (string, string, error) {
		data.tempDir = tempDir
		return "", "", fmt.Errorf("something bad")
	}
	s.PatchValue(&apiserver.Backup, testBackup)

	resp := s.sendValidRequest(c)

	// Check the response.
	var result params.Error
	s.checkErrorResponse(c, resp, http.StatusInternalServerError, "backup failed: something bad", &result)

	// Check the passed values.
	c.Assert(data.tempDir, gc.NotNil)
	_, err := os.Stat(data.tempDir)
	c.Check(err, jc.Satisfies, os.IsNotExist)
}

func (s *backupSuite) TestBackupErrorWhenBackupFileDoesNotExist(c *gc.C) {
	var data struct{ tempDir string }
	testBackup := func(password string, username string, tempDir string, address string) (string, string, error) {
		data.tempDir = tempDir
		backupFilePath := filepath.Join(tempDir, "testBackupFile")
		return backupFilePath, "some-sha", nil
	}
	s.PatchValue(&apiserver.Backup, testBackup)

	resp := s.sendValidRequest(c)

	// Check the response.
	var result params.Error
	s.checkErrorResponse(c, resp, http.StatusInternalServerError, "backup failed: missing backup file", &result)

	// Check the passed values.
	c.Assert(data.tempDir, gc.NotNil)
	_, err := os.Stat(data.tempDir)
	c.Check(err, jc.Satisfies, os.IsNotExist)
}
