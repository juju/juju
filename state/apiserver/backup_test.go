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
	mongoPassword string
	username      string
	address       string
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

	resp, err := s.sendRequest(c, machine.Tag().String(), password, "POST", s.backupURL(c), "", nil)
	c.Assert(err, gc.IsNil)
	s.assertErrorResponse(c, resp, http.StatusUnauthorized, "unauthorized")

	// Now try a user login.
	// (Still with an invalid method so we don't actually attempt backup.)
	resp, err = s.authRequest(c, "GET", s.backupURL(c), "", nil)
	c.Assert(err, gc.IsNil)
	s.assertErrorResponse(c, resp, http.StatusMethodNotAllowed, `unsupported method: "GET"`)
}

func (s *backupSuite) TestBackupCalledAndFileServed(c *gc.C) {
	testGetStateInfo := func(thisState *state.State) *state.Info {
		info := &state.Info{
			Password: "foobar",
			Tag:      "machine-0",
		}
		info.Addrs = append(info.Addrs, "localhost:80")
		return info
	}
	testBackup := func(password string, username string, tempDir string, address string) (string, string, error) {
		s.tempDir = tempDir
		s.mongoPassword = password
		s.username = username
		s.address = address
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
	s.PatchValue(&apiserver.GetStateInfo, testGetStateInfo)

	resp, err := s.authRequest(c, "POST", s.backupURL(c), "", nil)
	c.Assert(err, gc.IsNil)
	defer resp.Body.Close()

	c.Check(s.tempDir, gc.NotNil)
	_, err = os.Stat(s.tempDir)
	c.Check(err, jc.Satisfies, os.IsNotExist)
	c.Check(s.mongoPassword, gc.Equals, "foobar")
	c.Check(s.username, gc.Equals, "machine-0")
	c.Check(s.address, gc.Equals, "localhost:80")

	c.Check(resp.StatusCode, gc.Equals, 200)
	c.Check(resp.Header.Get("X-Content-SHA"), gc.Equals, "some-sha")
	c.Check(resp.Header.Get("Content-Type"), gc.Equals, "application/octet-stream")

	body, _ := ioutil.ReadAll(resp.Body)
	c.Check(body, jc.DeepEquals, []byte("foobarbam"))
}

func (s *backupSuite) TestErrorWhenBackupFails(c *gc.C) {
	testBackup := func(password string, username string, tempDir string, address string) (string, string, error) {
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
	testBackup := func(password string, username string, tempDir string, address string) (string, string, error) {
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
