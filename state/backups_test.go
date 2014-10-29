// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"time"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2"

	"github.com/juju/juju/state"
	"github.com/juju/juju/state/backups/metadata"
)

type backupSuite struct {
	ConnSuite
	session *mgo.Session
	dbOp    *state.DBOperator
	dbOps   []*state.DBOperator
}

var _ = gc.Suite(&backupSuite{})

func (s *backupSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)

	s.session = s.ConnSuite.MgoSuite.Session.Copy()
	s.dbOps = []*state.DBOperator{}
	s.dbOp = s.newDBOp()
}

func (s *backupSuite) TearDownTest(c *gc.C) {
	for _, dbOp := range s.dbOps {
		dbOp.Close()
	}
	s.session.Close()
	s.ConnSuite.TearDownTest(c)
}

func (s *backupSuite) newDBOp() *state.DBOperator {
	envUUID := s.State.EnvironTag().Id()

	var dbOp *state.DBOperator
	if len(s.dbOps) == 0 {
		db := s.session.DB(state.BackupsDB)
		dbOp = state.NewDBOperator(db, state.BackupsMetaC, envUUID)
	} else {
		dbOp = s.dbOps[0].Copy()
	}
	s.dbOps = append(s.dbOps, dbOp)
	return dbOp
}

func (s *backupSuite) metadata(c *gc.C) *metadata.Metadata {
	origin := metadata.NewOrigin(
		s.State.EnvironTag().Id(),
		"0",
		"localhost",
	)
	meta := metadata.NewMetadata(*origin, "", nil)
	err := meta.Finish(int64(42), "some hash")
	c.Assert(err, gc.IsNil)
	return meta
}

func (s *backupSuite) doc(c *gc.C) *state.BackupMetaDoc {
	var doc state.BackupMetaDoc
	meta := s.metadata(c)
	doc.UpdateFromMetadata(meta)
	return &doc
}

func (s *backupSuite) checkDoc(
	c *gc.C, doc, expected *state.BackupMetaDoc, id string,
) {
	if id != "" {
		c.Check(doc.ID, gc.Equals, id)
	}
	c.Check(doc.Notes, gc.Equals, expected.Notes)
	c.Check(doc.Started, gc.Equals, expected.Started)
	c.Check(doc.Checksum, gc.Equals, expected.Checksum)
	c.Check(doc.ChecksumFormat, gc.Equals, expected.ChecksumFormat)
	c.Check(doc.Size, gc.Equals, expected.Size)
	c.Check(doc.Environment, gc.Equals, expected.Environment)
	c.Check(doc.Machine, gc.Equals, expected.Machine)
	c.Check(doc.Hostname, gc.Equals, expected.Hostname)
	c.Check(doc.Version, gc.Equals, expected.Version)
	c.Check(doc.Stored, gc.DeepEquals, expected.Stored)
}

func (s *backupSuite) TestNewBackupID(c *gc.C) {
	origin := metadata.NewOrigin("spam", "0", "localhost")
	started := time.Date(2014, time.Month(9), 12, 13, 19, 27, 0, time.UTC)
	meta := metadata.NewMetadata(*origin, "", &started)

	var doc state.BackupMetaDoc
	doc.UpdateFromMetadata(meta)
	id := state.NewBackupID(&doc)

	c.Check(id, gc.Equals, "20140912-131927.spam")
}

func (s *backupSuite) TestGetBackupMetadataFound(c *gc.C) {
	original := s.doc(c)
	id, err := state.AddBackupMetadata(s.dbOp, original)
	c.Assert(err, gc.IsNil)

	doc, err := state.GetBackupMetadata(s.dbOp, id)
	c.Assert(err, gc.IsNil)

	s.checkDoc(c, doc, original, id)
}

func (s *backupSuite) TestGetBackupMetadataNotFound(c *gc.C) {
	_, err := state.GetBackupMetadata(s.dbOp, "spam")

	c.Check(err, jc.Satisfies, errors.IsNotFound)
}

func (s *backupSuite) TestAddBackupMetadataSuccess(c *gc.C) {
	original := s.doc(c)
	id, err := state.AddBackupMetadata(s.dbOp, original)
	c.Assert(err, gc.IsNil)

	doc, err := state.GetBackupMetadata(s.dbOp, id)
	c.Assert(err, gc.IsNil)

	s.checkDoc(c, doc, original, id)
}

func (s *backupSuite) TestAddBackupMetadataGeneratedID(c *gc.C) {
	original := s.doc(c)
	original.ID = "spam"
	id, err := state.AddBackupMetadata(s.dbOp, original)
	c.Assert(err, gc.IsNil)

	c.Check(id, gc.Not(gc.Equals), "spam")
}

func (s *backupSuite) TestAddBackupMetadataEmpty(c *gc.C) {
	original := &state.BackupMetaDoc{}
	c.Assert(original.Started, gc.NotNil)
	_, err := state.AddBackupMetadata(s.dbOp, original)

	c.Check(err, gc.NotNil)
}

func (s *backupSuite) TestAddBackupMetadataAlreadyExists(c *gc.C) {
	original := s.doc(c)
	id, err := state.AddBackupMetadata(s.dbOp, original)
	c.Assert(err, gc.IsNil)
	err = state.AddBackupMetadataID(s.dbOp, original, id)

	c.Check(err, jc.Satisfies, errors.IsAlreadyExists)
}

func (s *backupSuite) TestSetBackupStoredSuccess(c *gc.C) {
	stored := time.Now()
	original := s.doc(c)
	id, err := state.AddBackupMetadata(s.dbOp, original)
	c.Assert(err, gc.IsNil)
	doc, err := state.GetBackupMetadata(s.dbOp, id)
	c.Assert(err, gc.IsNil)
	c.Assert(doc.Stored, gc.Equals, int64(0))

	err = state.SetBackupStored(s.dbOp, id, stored)
	c.Assert(err, gc.IsNil)

	doc, err = state.GetBackupMetadata(s.dbOp, id)
	c.Assert(err, gc.IsNil)
	c.Check(doc.Stored, gc.Equals, stored.UTC().Unix())
}

func (s *backupSuite) TestSetBackupStoredNotFound(c *gc.C) {
	stored := time.Now()
	err := state.SetBackupStored(s.dbOp, "spam", stored)

	c.Check(err, jc.Satisfies, errors.IsNotFound)
}
