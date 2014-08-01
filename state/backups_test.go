// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"time"

	gc "launchpad.net/gocheck"

	"github.com/juju/juju/state"
)

type backupSuite struct {
	ConnSuite
}

var _ = gc.Suite(&backupSuite{})

//---------------------------
// nextBackupID()

func (s *backupSuite) TestBackupsNextBackupIDSuccess(c *gc.C) {
	timestamp := time.Date(2014, 7, 31, 16, 59, 59, 0, time.UTC)
	id := state.NextBackupID(s.State, &timestamp)

	c.Check(id, gc.Equals, "20140731-165959")
}

func (s *backupSuite) TestBackupsNextBackupIDEmpty(c *gc.C) {
	id := state.NextBackupID(s.State, nil)

	c.Check(id, gc.Not(gc.Equals), "")
}

//---------------------------
// setBackupStatus()

func (s *backupSuite) TestBackupsSetBackupStatusSuccess(c *gc.C) {
	original := backup.Info{
		Status: backup.StatusInfoOnly,
	}
	id, err := state.AddBackupMetadata(s.State, &original)
	c.Check(err, gc.IsNil)
	expected, err := state.GetBackupMetadata(s.State, id)
	c.Check(err, gc.IsNil)
	c.Check(expected, gc.Equals, backup.StatusInfoOnly)

	status := backup.StatusAvailable
	err := state.SetBackupStatus(s.State, id, status)
	c.Check(err, gc.IsNil)
	info, err := state.GetBackupMetadata(s.State, id)
	c.Check(err, gc.IsNil)

	c.Check(info, gc.Equals, backup.StatusAvailable)
}

func (s *backupSuite) TestBackupsSetBackupStatusMissing(c *gc.C) {
	id := "20140731-165959"
	status := backup.StatusAvailable
	err := state.SetBackupStatus(s.State, id, status)
	c.Check(err, gc.Equals, state.ErrBackupsMetadataNotFound)
}

//---------------------------
// getBackupMetadata()

func (s *backupSuite) TestBackupsGetBackupMetadataFound(c *gc.C) {
	expected := backup.Info{}
	id, err := state.AddBackupMetadata(s.State, &expected)
	c.Check(err, gc.IsNil)

	info, err := state.GetBackupMetadata(s.State, id)
	c.Check(err, gc.IsNil)

	expected.ID = id
	c.Check(info, gc.DeepEquals, expected)
}

func (s *backupSuite) TestBackupsGetBackupMetadataNotFound(c *gc.C) {
	id := "20140731-165959"
	info, err := state.GetBackupMetadata(s.State, id)
	c.Check(err, gc.Equals, state.ErrBackupsMetadataNotFound)
}

//---------------------------
// addBackupMetadata()

func (s *backupSuite) TestBackupsAddBackupMetadataSuccess(c *gc.C) {
	expected := backup.Info{
		ID: "20140731-165959",
	}
	id, err := state.AddBackupMetadata(s.State, &expected)
	c.Check(err, gc.IsNil)

	c.Check(id, gc.Not(gc.Equals), "20140731-165959")
	info := state.GetBackupMetadata(s.State, id)
	expected.ID = id
	c.Check(info, gc.DeepEquals, expected)
}

func (s *backupSuite) TestBackupsAddBackupMetadataEmpty(c *gc.C) {
	original := backup.Info{}
	id, err := state.AddBackupMetadata(s.State, &original)
	c.Check(err, gc.IsNil)

	info := state.GetBackupMetadata(s.State, id)
	c.Check(info, gc.NotNil)
}
