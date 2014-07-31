// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client_test

import (
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/state/apiserver/client"
	"github.com/juju/juju/state/backup/api"
)

var _ = gc.Suite(&backupSuite{})

type backupSuite struct {
	clientSuite
	api.BackupSuite
}

func (s *backupSuite) SetUpTest(c *gc.C) {
	s.clientSuite.SetUpTest(c)
	s.BackupSuite.SetUpTest(c)

	s.Client = s.APIState.Client()
}

//---------------------------
// tests

func (s *backupSuite) TestBackupAPI(c *gc.C) {
	// This runs without using the websockets RPC.
	s.CheckAPIClient(c, &client.Client{})
}

func (s *backupSuite) TestBackupRPCError(c *gc.C) {
	s.SetError("failed!")
	err := s.SendError(c, "", "")

	c.Check(err, gc.ErrorMatches, "failed!")
}

func (s *backupSuite) TestBackupRPCFailure(c *gc.C) {
	s.SetFailure("failed!")
	err := s.SendError(c, "", "")

	c.Check(err, gc.ErrorMatches, "failed!")
}

func (s *backupSuite) TestBackupCreateSuccess(c *gc.C) {
	info := s.SetSuccess(nil, "")
	result := s.SendSuccess(c, "create", "backup.tar.gz")

	s.CheckSuccess(c, result, info, "backup.tar.gz", "", nil)
}

func (s *backupSuite) TestBackupCreateSuccessBlank(c *gc.C) {
	s.Name = "juju-backup.tar.gz"
	info := s.SetSuccess(nil, "")
	result := s.SendSuccess(c, "create", "")

	s.CheckSuccess(c, result, info, "juju-backup.tar.gz", "", nil)
}

func (s *backupSuite) TestBackupCreateFailure(c *gc.C) {
	s.SetFailure("failed!")
	err := s.SendError(c, "create", "")

	c.Check(err, gc.ErrorMatches, "failed!")
}
