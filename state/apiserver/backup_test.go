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
}

var _ = gc.Suite(&backupSuite{})

func (s *backupSuite) backupURL(c *gc.C) string {
	uri := s.baseURL(c)
	uri.Path += "/backup"
	return uri.String()
}

func (s *backupSuite) TestRequiresAuth(c *gc.C) {
	resp, err := s.sendRequest(c, "", "", "POST", s.backupURL(c), "", nil)
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

func (s *backupSuite) TestAuthRequiresClientNotMachine(c *gc.C) {
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
	testGetMongoConnectionInfo := func(thisState *state.State) *state.Info {
		info := &state.Info{
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

	resp, err := s.authRequest(c, "POST", s.backupURL(c), "", nil)
	c.Assert(err, gc.IsNil)
	defer resp.Body.Close()

	c.Check(data.tempDir, gc.NotNil)
	_, err = os.Stat(data.tempDir)
	c.Check(err, jc.Satisfies, os.IsNotExist)
	c.Check(data.mongoPassword, gc.Equals, "foobar")
	c.Check(data.username, gc.Equals, "machine-0")
	c.Check(data.address, gc.Equals, "localhost:80")

	c.Check(resp.StatusCode, gc.Equals, 200)
	c.Check(resp.Header.Get("Digest"), gc.Equals, "SHA=some-sha")
	c.Check(resp.Header.Get("Content-Disposition"), gc.Equals,
		"attachment; filename=\"testBackupFile\"")
	c.Check(resp.Header.Get("Content-Type"), gc.Equals, "application/octet-stream")

	body, _ := ioutil.ReadAll(resp.Body)
	c.Check(body, jc.DeepEquals, []byte("foobarbam"))
}

func (s *backupSuite) TestErrorWhenBackupFails(c *gc.C) {
	var data struct{ tempDir string }
	testBackup := func(password string, username string, tempDir string, address string) (string, string, error) {
		data.tempDir = tempDir
		return "", "", fmt.Errorf("something bad")
	}
	s.PatchValue(&apiserver.Backup, testBackup)

	resp, err := s.authRequest(c, "POST", s.backupURL(c), "", nil)
	c.Assert(err, gc.IsNil)
	defer resp.Body.Close()

	c.Assert(data.tempDir, gc.NotNil)
	_, err = os.Stat(data.tempDir)
	c.Check(err, jc.Satisfies, os.IsNotExist)

	s.assertErrorResponse(c, resp, 500, "backup failed: something bad")
}

func (s *backupSuite) TestErrorWhenBackupFileDoesNotExist(c *gc.C) {
	var data struct{ tempDir string }
	testBackup := func(password string, username string, tempDir string, address string) (string, string, error) {
		data.tempDir = tempDir
		backupFilePath := filepath.Join(tempDir, "testBackupFile")
		return backupFilePath, "some-sha", nil
	}
	s.PatchValue(&apiserver.Backup, testBackup)

	resp, err := s.authRequest(c, "POST", s.backupURL(c), "", nil)
	c.Assert(err, gc.IsNil)
	defer resp.Body.Close()

	c.Assert(data.tempDir, gc.NotNil)
	_, err = os.Stat(data.tempDir)
	c.Check(err, jc.Satisfies, os.IsNotExist)

	s.assertErrorResponse(c, resp, 500, "backup failed: missing backup file")
}
