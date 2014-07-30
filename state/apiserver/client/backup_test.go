// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client_test

import (
	"fmt"
	"time"

	gc "launchpad.net/gocheck"

	"github.com/juju/juju/state"
	"github.com/juju/juju/state/apiserver/client"
	"github.com/juju/juju/state/backup"
	"github.com/juju/juju/state/backup/api"
	"github.com/juju/juju/version"
)

type backupSuite struct {
	clientSuite
	name      string
	timestamp *time.Time
	checksum  string
	size      int64
	version   *version.Number
	url       string
	apiErr    error
	err       error
}

var _ = gc.Suite(&backupSuite{})

func (s *backupSuite) SetUpTest(c *gc.C) {
	s.clientSuite.SetUpTest(c)
	timestamp := time.Now().UTC()
	s.timestamp = &timestamp
	s.checksum = "some SHA-1 hash"
	s.size = 42
	version := version.Current.Number
	s.version = &version
}

type testBackupAPI struct {
	info *backup.BackupInfo
	url  string
	err  error
}

func (ba *testBackupAPI) Create(name string) (*backup.BackupInfo, string, error) {
	if ba.err != nil {
		return nil, "", ba.err
	}
	return ba.info, ba.url, nil
}

func (s *backupSuite) setAPI() {
	info := backup.BackupInfo{
		Name:      "juju-backup.tar.gz",
		Timestamp: *s.timestamp,
		CheckSum:  s.checksum,
		Size:      s.size,
		Version:   *s.version,
	}
	newapi := func(st *state.State) (api.BackupServerAPI, error) {
		serverAPI := testBackupAPI{
			info: &info,
			url:  s.url,
			err:  s.apiErr,
		}
		return &serverAPI, s.err
	}
	s.PatchValue(client.NewBackupAPI, newapi)
}

func (s *backupSuite) TestBackupAPIValid(c *gc.C) {
	_, err := client.NewBackupAPI(s.State)
	c.Assert(err, gc.IsNil)
}

func (s *backupSuite) TestBackupAPIBadAPI(c *gc.C) {
	s.err = fmt.Errorf("failed!")
	s.setAPI()

	_, err := client.NewBackupAPI(s.State)
	c.Assert(err, gc.ErrorMatches, "failed!")
}

func (s *backupSuite) TestBackupCreateSuccess(c *gc.C) {
	s.setAPI()

	api, err := client.NewBackupAPI(s.State)
	c.Assert(err, gc.IsNil)
	info, url, err := api.Create("backup.tar.gz")

	c.Check(err, gc.IsNil)
	c.Check(info.Name, gc.Equals, "backup.tar.gz")
	c.Check(url, gc.Equals, "")
}

func (s *backupSuite) TestBackupCreateSuccessBlank(c *gc.C) {
	s.setAPI()

	api, err := client.NewBackupAPI(s.State)
	c.Assert(err, gc.IsNil)
	info, url, err := api.Create("")

	c.Check(err, gc.IsNil)
	c.Check(info.Name, gc.Equals, "juju-backup.tar.gz")
	c.Check(url, gc.Equals, "")
}

func (s *backupSuite) TestBackupCreateFailure(c *gc.C) {
	s.apiErr = fmt.Errorf("failed!")
	s.setAPI()

	api, err := client.NewBackupAPI(s.State)
	c.Assert(err, gc.IsNil)
	_, _, err = api.Create("backup.tar.gz")

	c.Check(err, gc.ErrorMatches, "failed!")
}
