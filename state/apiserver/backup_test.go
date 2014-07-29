// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"

	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/storage"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/apiserver"
	"github.com/juju/juju/state/backup"
)

var uploadBackupToStorage = *apiserver.UploadBackupToStorage

type backupSuite struct {
	authHttpSuite
}

var _ = gc.Suite(&backupSuite{})

func (s *backupSuite) backupURL(c *gc.C) string {
	environ, err := s.State.Environment()
	c.Assert(err, gc.IsNil)
	uri := s.baseURL(c)
	uri.Path = fmt.Sprintf("/environment/%s/backup", environ.UUID())
	return uri.String()
}

func (s *backupSuite) TestBackupRequiresAuth(c *gc.C) {
	resp, err := s.sendRequest(c, "", "", "POST", s.backupURL(c), "", nil)
	c.Assert(err, gc.IsNil)
	s.assertErrorResponse(c, resp, http.StatusUnauthorized, "unauthorized")
}

func (s *backupSuite) TestBackupRequiresPOST(c *gc.C) {
	resp, err := s.authRequest(c, "PUT", s.backupURL(c), "", nil)
	c.Assert(err, gc.IsNil)
	s.assertErrorResponse(c, resp, http.StatusMethodNotAllowed, `unsupported method: "PUT"`)

	resp, err = s.authRequest(c, "GET", s.backupURL(c), "", nil)
	c.Assert(err, gc.IsNil)
	s.assertErrorResponse(c, resp, http.StatusMethodNotAllowed, `unsupported method: "GET"`)
}

func (s *backupSuite) TestBackupAuthRequiresClientNotMachine(c *gc.C) {
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

type happyBackup struct {
	tempDir, mongoPassword, username, address string
}

func (b *happyBackup) Backup(info *backup.DBConnInfo, tempDir string) (
	string, string, error,
) {
	b.tempDir = tempDir
	b.mongoPassword = info.Password
	b.username = info.Username
	b.address = info.Hostname
	backupFilePath := filepath.Join(tempDir, "testBackupFile")
	if err := ioutil.WriteFile(backupFilePath, []byte("foobarbam"), 0644); err != nil {
		return "", "", err
	}
	return backupFilePath, "some-sha", nil
}

func (s *backupSuite) TestBackupCalledAndFileServedAndStored(c *gc.C) {
	testGetDBConnInfo := func(thisState *state.State) *backup.DBConnInfo {
		info := backup.DBConnInfo{
			Hostname: "localhost:80",
			Username: names.NewMachineTag("0").String(),
			Password: "foobar",
		}
		return &info
	}

	var b happyBackup
	s.PatchValue(apiserver.Backup, b.Backup)
	s.PatchValue(apiserver.GetDBConnInfo, testGetDBConnInfo)

	resp, err := s.authRequest(c, "POST", s.backupURL(c), "", nil)
	c.Assert(err, gc.IsNil)
	defer resp.Body.Close()

	c.Check(b.tempDir, gc.NotNil)
	_, err = os.Stat(b.tempDir)
	c.Check(err, jc.Satisfies, os.IsNotExist)
	c.Check(b.mongoPassword, gc.Equals, "foobar")
	c.Check(b.username, gc.Equals, "machine-0")
	c.Check(b.address, gc.Equals, "localhost:80")

	c.Check(resp.StatusCode, gc.Equals, 200)
	c.Check(resp.Header.Get("Digest"), gc.Equals, "SHA=some-sha")
	c.Check(resp.Header.Get("Content-Disposition"), gc.Equals,
		"attachment; filename=\"testBackupFile\"")
	c.Check(resp.Header.Get("Content-Type"), gc.Equals, "application/octet-stream")

	body, _ := ioutil.ReadAll(resp.Body)
	c.Check(body, jc.DeepEquals, []byte("foobarbam"))

	stor, err := environs.GetStorage(s.State)
	c.Assert(err, gc.IsNil)
	storReader, err := stor.Get("/backups/testBackupFile")
	c.Assert(err, gc.IsNil)
	bodyFromStorage, _ := ioutil.ReadAll(storReader)
	c.Check(bodyFromStorage, jc.DeepEquals, []byte("foobarbam"))
}

func (s *backupSuite) TestBackupErrorWhenBackupFails(c *gc.C) {
	var data struct{ tempDir string }
	testBackup := func(info *backup.DBConnInfo, tempDir string) (string, string, error) {
		data.tempDir = tempDir
		return "", "", fmt.Errorf("something bad")
	}
	s.PatchValue(apiserver.Backup, testBackup)

	resp, err := s.authRequest(c, "POST", s.backupURL(c), "", nil)
	c.Assert(err, gc.IsNil)
	defer resp.Body.Close()

	c.Assert(data.tempDir, gc.NotNil)
	_, err = os.Stat(data.tempDir)
	c.Check(err, jc.Satisfies, os.IsNotExist)

	s.assertErrorResponse(c, resp, 500, "backup failed: something bad")
}

func (s *backupSuite) TestBackupErrorWhenBackupFileDoesNotExist(c *gc.C) {
	var data struct{ tempDir string }
	testBackup := func(info *backup.DBConnInfo, tempDir string) (string, string, error) {
		data.tempDir = tempDir
		backupFilePath := filepath.Join(tempDir, "testBackupFile")
		return backupFilePath, "some-sha", nil
	}
	s.PatchValue(apiserver.Backup, testBackup)

	resp, err := s.authRequest(c, "POST", s.backupURL(c), "", nil)
	c.Assert(err, gc.IsNil)
	defer resp.Body.Close()

	c.Assert(data.tempDir, gc.NotNil)
	_, err = os.Stat(data.tempDir)
	c.Check(err, jc.Satisfies, os.IsNotExist)

	s.assertErrorResponse(c, resp, 500, `backup failed: .+`)
}

func (s *backupSuite) TestBackupErrorWhenBackupStorageFails(c *gc.C) {
	var b happyBackup
	s.PatchValue(apiserver.Backup, b.Backup)
	s.PatchValue(apiserver.UploadBackupToStorage,
		func(*state.State, *os.File) error {
			return fmt.Errorf("blam")
		},
	)

	resp, err := s.authRequest(c, "POST", s.backupURL(c), "", nil)

	c.Assert(err, gc.IsNil)
	defer resp.Body.Close()
	s.assertErrorResponse(c, resp, 500, "backup storage failed: blam")
}

func (s *backupSuite) TestBackupErrorWhenBackupFileStatFails(c *gc.C) {
	f := s.makeTempFile(c)
	f.Close() // Close the file so that the Stat call fails.
	err := uploadBackupToStorage(s.State, f)
	c.Assert(err, gc.ErrorMatches, "failed to stat backup file: .+")
}

func (s *backupSuite) TestBackupErrorWhenStorageCantBeOpened(c *gc.C) {
	f := s.makeTempFile(c)
	defer f.Close()

	s.PatchValue(apiserver.GetStorage,
		func(*state.State) (storage.Storage, error) {
			return nil, fmt.Errorf("blam")
		},
	)

	err := uploadBackupToStorage(s.State, f)
	c.Assert(err, gc.ErrorMatches, "failed to open storage: blam")
}

func (s *backupSuite) TestBackupErrorWhenStoragePutFails(c *gc.C) {
	f := s.makeTempFile(c)
	defer f.Close()

	stor, err := environs.GetStorage(s.State)
	c.Assert(err, gc.IsNil)
	dummy.Poison(stor, "/backups/foo", fmt.Errorf("blam"))

	err = uploadBackupToStorage(s.State, f)
	c.Assert(err, gc.ErrorMatches, "blam")
}

func (s *backupSuite) makeTempFile(c *gc.C) *os.File {
	tempDir := c.MkDir()
	f, err := os.Create(filepath.Join(tempDir, "foo"))
	c.Assert(err, gc.IsNil)
	return f
}
