// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"time"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state"
	"github.com/juju/juju/state/backups/metadata"
)

type backupSuite struct {
	ConnSuite
}

var _ = gc.Suite(&backupSuite{})

func (s *backupSuite) metadata(c *gc.C) *metadata.Metadata {
	origin := metadata.NewOrigin(
		s.State.EnvironUUID(),
		"0",
		"localhost",
	)
	meta := metadata.NewMetadata(*origin, "", nil)
	err := meta.Finish(int64(42), "some hash")
	c.Assert(err, gc.IsNil)
	return meta
}

func (s *backupSuite) checkMeta(c *gc.C, meta, expected *metadata.Metadata, id string,
) {
	if id != "" {
		c.Check(meta.ID(), gc.Equals, id)
	}
	c.Check(meta.Notes, gc.Equals, expected.Notes)
	c.Check(meta.Started.Unix(), gc.Equals, expected.Started.Unix())
	c.Check(meta.Checksum(), gc.Equals, expected.Checksum())
	c.Check(meta.ChecksumFormat(), gc.Equals, expected.ChecksumFormat())
	c.Check(meta.Size(), gc.Equals, expected.Size())
	c.Check(meta.Origin.Environment, gc.Equals, expected.Origin.Environment)
	c.Check(meta.Origin.Machine, gc.Equals, expected.Origin.Machine)
	c.Check(meta.Origin.Hostname, gc.Equals, expected.Origin.Hostname)
	c.Check(meta.Origin.Version, gc.Equals, expected.Origin.Version)
	if meta.Stored() != nil && expected.Stored != nil {
		c.Check(meta.Stored().Unix(), gc.Equals, expected.Stored().Unix())
	} else {
		c.Check(meta.Stored(), gc.Equals, expected.Stored())
	}
}

func (s *backupSuite) TestNewBackupID(c *gc.C) {
	origin := metadata.NewOrigin("spam", "0", "localhost")
	started := time.Date(2014, time.Month(9), 12, 13, 19, 27, 0, time.UTC)
	meta := metadata.NewMetadata(*origin, "", &started)
	id := state.NewBackupID(meta)

	c.Check(id, gc.Equals, "20140912-131927.spam")
}

func (s *backupSuite) TestGetBackupMetadataFound(c *gc.C) {
	original := s.metadata(c)
	id, err := state.AddBackupMetadata(s.State, original)
	c.Assert(err, gc.IsNil)

	meta, err := state.GetBackupMetadata(s.State, id)
	c.Assert(err, gc.IsNil)

	s.checkMeta(c, meta, original, id)
}

func (s *backupSuite) TestGetBackupMetadataNotFound(c *gc.C) {
	_, err := state.GetBackupMetadata(s.State, "spam")

	c.Check(err, jc.Satisfies, errors.IsNotFound)
}

func (s *backupSuite) TestAddBackupMetadataSuccess(c *gc.C) {
	original := s.metadata(c)
	id, err := state.AddBackupMetadata(s.State, original)
	c.Assert(err, gc.IsNil)

	meta, err := state.GetBackupMetadata(s.State, id)
	c.Assert(err, gc.IsNil)

	s.checkMeta(c, meta, original, id)
}

func (s *backupSuite) TestAddBackupMetadataGeneratedID(c *gc.C) {
	original := s.metadata(c)
	original.SetID("spam")
	id, err := state.AddBackupMetadata(s.State, original)
	c.Assert(err, gc.IsNil)

	c.Check(id, gc.Not(gc.Equals), "spam")
}

func (s *backupSuite) TestAddBackupMetadataEmpty(c *gc.C) {
	original := &metadata.Metadata{}
	c.Assert(original.Started, gc.NotNil)
	_, err := state.AddBackupMetadata(s.State, original)

	c.Check(err, gc.NotNil)
}

func (s *backupSuite) TestAddBackupMetadataAlreadyExists(c *gc.C) {
	original := s.metadata(c)
	id, err := state.AddBackupMetadata(s.State, original)
	c.Assert(err, gc.IsNil)
	err = state.AddBackupMetadataID(s.State, original, id)

	c.Check(err, jc.Satisfies, errors.IsAlreadyExists)
}

func (s *backupSuite) TestSetBackupStoredSuccess(c *gc.C) {
	stored := time.Now()
	original := s.metadata(c)
	id, err := state.AddBackupMetadata(s.State, original)
	c.Assert(err, gc.IsNil)
	meta, err := state.GetBackupMetadata(s.State, id)
	c.Assert(err, gc.IsNil)
	c.Assert(meta.Stored(), gc.IsNil)

	err = state.SetBackupStored(s.State, id, stored)
	c.Assert(err, gc.IsNil)

	meta, err = state.GetBackupMetadata(s.State, id)
	c.Assert(err, gc.IsNil)
	c.Check(meta.Stored().Unix(), gc.Equals, stored.UTC().Unix())
}

func (s *backupSuite) TestSetBackupStoredNotFound(c *gc.C) {
	stored := time.Now()
	err := state.SetBackupStored(s.State, "spam", stored)

	c.Check(err, jc.Satisfies, errors.IsNotFound)
}
