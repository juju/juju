// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api_test

import (
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/state/api"
	backup "github.com/juju/juju/state/backup/api"
)

var _ = gc.Suite(&backupSuite{})

type backupSuite struct {
	clientSuite
	backup.BackupSuite
}

func (s *backupSuite) SetUpTest(c *gc.C) {
	s.clientSuite.SetUpTest(c)
	s.BackupSuite.SetUpTest(c)

	s.Client = fakeClient{&s.Faker, s.Client}
	// Ensure we don't test over the websockets RPC (except explicitly).
	s.SetFakeBackupClient()
}

type fakeClient struct {
	backup.FakeAPIClient
	api.Client
}

//---------------------------
// tests

func (s *backupSuite) TestBackupRPC(c *gc.C) {
	// This tests using via the websockets RPC.
	s.CheckAPIClient(c, s.Client)
}

func (s *backupSuite) TestBackupCreateExplicitName(c *gc.C) {
	s.Name = "juju-backup.tar.gz"
	expected := s.SetSuccess(nil, "")
	info, url, err := s.Client.BackupCreate("backup.tar.gz")

	result := backup.BackupResult{Info: info, URL: url}
	s.CheckSuccess(c, &result, expected, "backup.tar.gz", "", err)
}

func (s *backupSuite) TestBackupCreateDefaultName(c *gc.C) {
	s.Name = "juju-backup.tar.gz"
	expected := s.SetSuccess(nil, "")
	info, url, err := s.Client.BackupCreate("")

	result := backup.BackupResult{Info: info, URL: url}
	s.CheckSuccess(c, &result, expected, "juju-backup.tar.gz", "", err)
}

func (s *backupSuite) TestBackupCreateFailure(c *gc.C) {
	s.SetFailure("failed!")
	info, url, err := s.Client.BackupCreate("")

	result := backup.BackupResult{Info: info, URL: url}
	s.CheckError(c, &result, err, "failed!")
}
