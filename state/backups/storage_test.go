// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"time"

	"github.com/juju/errors"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state"
	"github.com/juju/juju/state/backups"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/testing"
)

type storageSuite struct {
	gitjujutesting.MgoSuite
	testing.BaseSuite
	State *state.State
}

var _ = gc.Suite(&storageSuite{})

func (s *storageSuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)
	s.MgoSuite.SetUpSuite(c)
}

func (s *storageSuite) TearDownSuite(c *gc.C) {
	s.MgoSuite.TearDownSuite(c)
	s.BaseSuite.TearDownSuite(c)
}

func (s *storageSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.MgoSuite.SetUpTest(c)
	s.State = statetesting.NewState(c)
}

func (s *storageSuite) TearDownTest(c *gc.C) {
	if s.State != nil {
		// If setup fails, we don't have a State yet
		s.State.Close()
	}
	s.MgoSuite.TearDownTest(c)
	s.BaseSuite.TearDownTest(c)
}

func (s *storageSuite) metadata(c *gc.C) *backups.Metadata {
	meta := backups.NewMetadata()
	meta.Origin.Environment = s.State.EnvironUUID()
	meta.Origin.Machine = "0"
	meta.Origin.Hostname = "localhost"
	err := meta.MarkComplete(int64(42), "some hash")
	c.Assert(err, jc.ErrorIsNil)
	return meta
}

func (s *storageSuite) checkMeta(c *gc.C, meta, expected *backups.Metadata, id string,
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
	if meta.Stored() != nil && expected.Stored() != nil {
		c.Check(meta.Stored().Unix(), gc.Equals, expected.Stored().Unix())
	} else {
		c.Check(meta.Stored(), gc.Equals, expected.Stored())
	}
}

func (s *storageSuite) TestNewStorageID(c *gc.C) {
	meta := s.metadata(c)
	meta.Origin.Environment = "spam"
	meta.Started = time.Date(2014, time.Month(9), 12, 13, 19, 27, 0, time.UTC)
	id := backups.NewBackupID(meta)

	c.Check(id, gc.Equals, "20140912-131927.spam")
}

func (s *storageSuite) TestGetBackupMetadataFound(c *gc.C) {
	original := s.metadata(c)
	id, err := backups.AddBackupMetadata(s.State, original)
	c.Assert(err, jc.ErrorIsNil)

	meta, err := backups.GetBackupMetadata(s.State, id)
	c.Assert(err, jc.ErrorIsNil)

	s.checkMeta(c, meta, original, id)
}

func (s *storageSuite) TestGetBackupMetadataNotFound(c *gc.C) {
	_, err := backups.GetBackupMetadata(s.State, "spam")

	c.Check(err, jc.Satisfies, errors.IsNotFound)
}

func (s *storageSuite) TestAddBackupMetadataSuccess(c *gc.C) {
	original := s.metadata(c)
	id, err := backups.AddBackupMetadata(s.State, original)
	c.Assert(err, jc.ErrorIsNil)

	meta, err := backups.GetBackupMetadata(s.State, id)
	c.Assert(err, jc.ErrorIsNil)

	s.checkMeta(c, meta, original, id)
}

func (s *storageSuite) TestAddBackupMetadataGeneratedID(c *gc.C) {
	original := s.metadata(c)
	original.SetID("spam")
	id, err := backups.AddBackupMetadata(s.State, original)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(id, gc.Not(gc.Equals), "spam")
}

func (s *storageSuite) TestAddBackupMetadataEmpty(c *gc.C) {
	original := backups.NewMetadata()
	_, err := backups.AddBackupMetadata(s.State, original)

	c.Check(err, gc.NotNil)
}

func (s *storageSuite) TestAddBackupMetadataAlreadyExists(c *gc.C) {
	original := s.metadata(c)
	id, err := backups.AddBackupMetadata(s.State, original)
	c.Assert(err, jc.ErrorIsNil)
	err = backups.AddBackupMetadataID(s.State, original, id)

	c.Check(err, jc.Satisfies, errors.IsAlreadyExists)
}

func (s *storageSuite) TestSetBackupStoredTimeSuccess(c *gc.C) {
	stored := time.Now()
	original := s.metadata(c)
	id, err := backups.AddBackupMetadata(s.State, original)
	c.Assert(err, jc.ErrorIsNil)
	meta, err := backups.GetBackupMetadata(s.State, id)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(meta.Stored(), gc.IsNil)

	err = backups.SetBackupStoredTime(s.State, id, stored)
	c.Assert(err, jc.ErrorIsNil)

	meta, err = backups.GetBackupMetadata(s.State, id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(meta.Stored().Unix(), gc.Equals, stored.UTC().Unix())
}

func (s *storageSuite) TestSetBackupStoredTimeNotFound(c *gc.C) {
	stored := time.Now()
	err := backups.SetBackupStoredTime(s.State, "spam", stored)

	c.Check(err, jc.Satisfies, errors.IsNotFound)
}
