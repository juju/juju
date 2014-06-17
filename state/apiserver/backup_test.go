// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"

	"github.com/juju/juju/state"
	"github.com/juju/juju/state/apiserver"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "launchpad.net/gocheck"
)

type backupSuite struct {
	authHttpSuite
	tempDir       string
	adminPassword string
	mongoPort     int
}

var _ = gc.Suite(&backupSuite{})

func (s *backupSuite) backupURL(c *gc.C) string {
	uri := s.baseURL(c)
	uri.Path += "/backup"
	return uri.String()
}

func (s *backupSuite) TestRequiresAuth(c *gc.C) {
	resp, err := s.sendRequest(c, "", "", "GET", s.backupURL(c), "", nil)
	c.Assert(err, gc.IsNil)
	s.assertErrorResponse(c, resp, http.StatusUnauthorized, "unauthorized")
}

func (s *backupSuite) TestRequiresPOST(c *gc.C) {
	resp, err := s.authRequest(c, "PUT", s.backupURL(c), "", nil)
	c.Assert(err, gc.IsNil)
	s.assertErrorResponse(c, resp, http.StatusMethodNotAllowed, `unsupported method: "PUT"`)

	resp, err = s.authRequest(c, "GET", s.backupURL(c), "", nil)
	c.Assert(err, gc.IsNil)
	s.assertErrorResponse(c, resp, http.StatusMethodNotAllowed, `unsupported method: "GET"`)
}

func (s *backupSuite) TestAuthRequiresUser(c *gc.C) {
	// Add a machine and try to login.
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	err = machine.SetProvisioned("foo", "fake_nonce", nil)
	c.Assert(err, gc.IsNil)
	password, err := utils.RandomPassword()
	c.Assert(err, gc.IsNil)
	err = machine.SetPassword(password)
	c.Assert(err, gc.IsNil)

	resp, err := s.sendRequest(c, machine.Tag(), password, "GET", s.backupURL(c), "", nil)
	c.Assert(err, gc.IsNil)
	s.assertErrorResponse(c, resp, http.StatusUnauthorized, "unauthorized")

	// Now try a user login.
	resp, err = s.authRequest(c, "GET", s.backupURL(c), "", nil)
	c.Assert(err, gc.IsNil)
	s.assertErrorResponse(c, resp, http.StatusMethodNotAllowed, `unsupported method: "GET"`)
}

func (s *backupSuite) TestBackupCalledAndFileServed(c *gc.C) {
	testBackup := func(adminPassword string, tempDir string, mongoPort int) (string, string, error) {
		s.tempDir = tempDir
		s.adminPassword = adminPassword
		s.mongoPort = mongoPort
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

	resp, err := s.authRequest(c, "POST", s.backupURL(c), "", nil)
	c.Assert(err, gc.IsNil)
	defer resp.Body.Close()

	c.Check(s.tempDir, gc.NotNil)
	_, err = os.Stat(s.tempDir)
	c.Check(err, jc.Satisfies, os.IsNotExist)
	c.Check(s.mongoPort, gc.Equals, 1234)
	c.Check(s.adminPassword, gc.Equals, "admin password")

	c.Check(resp.StatusCode, gc.Equals, 200)
	c.Check(resp.Header.Get("X-Content-SHA"), gc.Equals, "some-sha")
	c.Check(resp.Header.Get("Content-Type"), gc.Equals, "application/octet-stream")

	body, _ := ioutil.ReadAll(resp.Body)
	c.Check(body, jc.DeepEquals, []byte("foobarbam"))
}

func (s *backupSuite) TestErrorWhenBackupFails(c *gc.C) {
	testBackup := func(adminPassword, tempDir string, mongoPort int) (string, string, error) {
		s.tempDir = tempDir
		return "", "", fmt.Errorf("something bad")
	}
	s.PatchValue(&apiserver.Backup, testBackup)

	resp, err := s.authRequest(c, "POST", s.backupURL(c), "", nil)
	c.Assert(err, gc.IsNil)
	defer resp.Body.Close()

	c.Assert(s.tempDir, gc.NotNil)
	_, err = os.Stat(s.tempDir)
	c.Assert(os.IsNotExist(err), jc.IsTrue)

	s.assertErrorResponse(c, resp, 500, "backup failed: something bad")
}

func (s *backupSuite) TestErrorWhenBackupFileDoesNotExist(c *gc.C) {
	testBackup := func(adminPassword string, tempDir string, mongoPort int) (string, string, error) {
		s.tempDir = tempDir
		backupFilePath := filepath.Join(tempDir, "testBackupFile")
		return backupFilePath, "some-sha", nil
	}
	s.PatchValue(&apiserver.Backup, testBackup)

	resp, err := s.authRequest(c, "POST", s.backupURL(c), "", nil)
	c.Assert(err, gc.IsNil)
	defer resp.Body.Close()

	c.Assert(s.tempDir, gc.NotNil)
	_, err = os.Stat(s.tempDir)
	c.Assert(os.IsNotExist(err), jc.IsTrue)

	s.assertErrorResponse(c, resp, 500, "backup failed: missing backup file")
}
