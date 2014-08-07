// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"os"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/state"
	"github.com/juju/juju/state/backup"
	"github.com/juju/juju/version"
)

type backupSuite struct {
	StateSuite
}

var _ = gc.Suite(&backupSuite{})

func (s *backupSuite) metadata(c *gc.C) *backup.Metadata {
	origin := state.NewBackupOrigin(s.State, "0")
	origin.Hostname = "localhost"
	return backup.NewMetadata("some-hash", 42, *origin, "")
}

func (s *backupSuite) checkMetadata(
	c *gc.C, metadata, expected *backup.Metadata, id string,
) bool {
	res := true
	if id != "" {
		res = c.Check(metadata.ID, gc.Equals, id)
	}
	res = c.Check(metadata.Notes, gc.Equals, expected.Notes) && res
	res = c.Check(metadata.Timestamp.Unix(), gc.DeepEquals, expected.Timestamp.Unix()) && res
	res = c.Check(metadata.CheckSum, gc.Equals, expected.CheckSum) && res
	res = c.Check(metadata.CheckSumFormat, gc.Equals, expected.CheckSumFormat) && res
	res = c.Check(metadata.Size, gc.Equals, expected.Size) && res
	res = c.Check(metadata.Origin, gc.DeepEquals, expected.Origin) && res
	res = c.Check(metadata.Archived, gc.DeepEquals, expected.Archived) && res
	return res
}

//---------------------------
// NewBackupOrigin()

func (s *backupSuite) TestBackupsNewBackupOrigin(c *gc.C) {
	origin := state.NewBackupOrigin(s.State, "0")
	hostname, err := os.Hostname()
	c.Assert(err, gc.IsNil)

	c.Check(origin.Environment, gc.Matches, s.State.EnvironTag().Id())
	c.Check(origin.Machine, gc.Equals, "0")
	c.Check(origin.Hostname, gc.Equals, hostname)
	c.Check(origin.Version.Major, gc.Equals, version.Current.Major)
	c.Check(origin.Version.Minor, gc.Equals, version.Current.Minor)
}

//---------------------------
// getBackupMetadata()

func (s *backupSuite) TestBackupsGetBackupMetadataFound(c *gc.C) {
	expected := s.metadata(c)
	id, err := state.AddBackupMetadata(s.State, expected)
	c.Assert(err, gc.IsNil)

	metadata, err := state.GetBackupMetadata(s.State, id)
	c.Check(err, gc.IsNil)

	s.checkMetadata(c, metadata, expected, id)
}

func (s *backupSuite) TestBackupsGetBackupMetadataNotFound(c *gc.C) {
	_, err := state.GetBackupMetadata(s.State, "spam")
	c.Check(err, jc.Satisfies, errors.IsNotFound)
}

//---------------------------
// addBackupMetadata()

func (s *backupSuite) TestBackupsAddBackupMetadataSuccess(c *gc.C) {
	expected := s.metadata(c)
	id, err := state.AddBackupMetadata(s.State, expected)
	c.Check(err, gc.IsNil)

	metadata, err := state.GetBackupMetadata(s.State, id)
	c.Assert(err, gc.IsNil)

	s.checkMetadata(c, metadata, expected, id)
}

func (s *backupSuite) TestBackupsAddBackupMetadataGeneratedID(c *gc.C) {
	expected := s.metadata(c)
	expected.ID = "spam"
	id, err := state.AddBackupMetadata(s.State, expected)
	c.Check(err, gc.IsNil)

	c.Check(id, gc.Not(gc.Equals), "spam")
}

func (s *backupSuite) TestBackupsAddBackupMetadataEmpty(c *gc.C) {
	original := backup.Metadata{}
	id, err := state.AddBackupMetadata(s.State, &original)
	c.Check(err, gc.IsNil)

	metadata, err := state.GetBackupMetadata(s.State, id)
	c.Assert(err, gc.IsNil)
	c.Check(metadata, gc.NotNil)
}

func (s *backupSuite) TestBackupsAddBackupMetadataAlreadyExists(c *gc.C) {
	expected := s.metadata(c)
	id, err := state.AddBackupMetadata(s.State, expected)
	c.Assert(err, gc.IsNil)
	err = state.AddBackupMetadataID(s.State, expected, id)

	c.Check(err, jc.Satisfies, errors.IsAlreadyExists)
}

//---------------------------
// setBackupArchived()

func (s *backupSuite) TestBackupsSetBackupArchivedSuccess(c *gc.C) {
	original := s.metadata(c)
	id, err := state.AddBackupMetadata(s.State, original)
	c.Check(err, gc.IsNil)
	metadata, err := state.GetBackupMetadata(s.State, id)
	c.Assert(err, gc.IsNil)
	c.Assert(metadata.Archived, gc.Equals, false)

	err = state.SetBackupArchived(s.State, id)
	c.Check(err, gc.IsNil)

	metadata, err = state.GetBackupMetadata(s.State, id)
	c.Assert(err, gc.IsNil)
	c.Assert(metadata.Archived, gc.Equals, true)
}

func (s *backupSuite) TestBackupsSetBackupArchivedNotFound(c *gc.C) {
	err := state.SetBackupArchived(s.State, "spam")

	c.Check(err, jc.Satisfies, errors.IsNotFound)
}
