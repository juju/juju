// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"github.com/juju/mgo/v3/bson"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	jujutxn "github.com/juju/txn/v3"
	gc "gopkg.in/check.v1"
)

type cleanupInternalSuite struct {
	internalStateSuite
}

var _ = gc.Suite(&cleanupInternalSuite{})

func (s *cleanupInternalSuite) TestRemoveDyingRemoteApplicationOnForce(c *gc.C) {
	remote, err := s.state.AddRemoteApplication(AddRemoteApplicationParams{
		Name:        "remote",
		SourceModel: names.NewModelTag("source-model"),
	})
	c.Assert(err, jc.ErrorIsNil)

	coll, closer, err := s.state.db().GetCollection(remoteApplicationsC)
	c.Assert(err, jc.ErrorIsNil)
	defer closer()
	err = coll.Writeable().UpdateId(s.state.docID(remote.Name()), bson.M{
		"$set": bson.M{"life": Dying},
	})
	c.Assert(err, jc.ErrorIsNil)

	noForce := false
	c.Assert(s.state.removeRemoteApplicationsForDyingModel(DestroyModelParams{Force: &noForce}), jc.ErrorIsNil)
	c.Assert(remote.Refresh(), jc.ErrorIsNil)
	c.Check(remote.Life(), gc.Equals, Dying)

	force := true
	c.Assert(s.state.removeRemoteApplicationsForDyingModel(DestroyModelParams{Force: &force}), jc.ErrorIsNil)
	c.Check(remote.Refresh(), jc.Satisfies, errors.IsNotFound)
}

func (s *cleanupInternalSuite) TestRemoveRemoteApplicationsForDyingModelContinuesAfterFailure(c *gc.C) {
	s.assertRemoveRemoteApplicationsAfterFailure(c, true)
}

func (s *cleanupInternalSuite) TestRemoveRemoteApplicationsForDyingModelStopsAfterFailure(c *gc.C) {
	s.assertRemoveRemoteApplicationsAfterFailure(c, false)
}

func (s *cleanupInternalSuite) assertRemoveRemoteApplicationsAfterFailure(c *gc.C, force bool) {
	for _, name := range []string{"first", "second"} {
		_, err := s.state.AddRemoteApplication(AddRemoteApplicationParams{
			Name:        name,
			SourceModel: names.NewModelTag("source-model"),
		})
		c.Assert(err, jc.ErrorIsNil)
	}
	// Fail the first removal regardless of which application is read first.
	// A forced pass also attempts the second application, so fail it too.
	failures := []error{errors.New("first removal failed")}
	if force {
		failures = append(failures, errors.New("second removal failed"))
	}
	database := &cleanupFailureDatabase{
		Database: s.state.database,
		failures: failures,
	}
	s.PatchValue(&s.state.database, database)
	err := s.state.removeRemoteApplicationsForDyingModel(DestroyModelParams{Force: &force})
	c.Check(err, jc.ErrorIs, failures[0])
	// Every injected failure was consumed, so each application was attempted.
	c.Check(database.failures, gc.HasLen, 0)
	if force {
		c.Check(err, gc.ErrorMatches, `cannot remove remote applications first, second: .*first removal failed`)
	}
	remaining, err := s.state.AllRemoteApplications()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(remaining, gc.HasLen, 2)

	// The failed applications can be removed on the next pass.
	c.Assert(s.state.removeRemoteApplicationsForDyingModel(DestroyModelParams{Force: &force}), jc.ErrorIsNil)
	remaining, err = s.state.AllRemoteApplications()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(remaining, gc.HasLen, 0)
}

type cleanupFailureDatabase struct {
	Database
	failures []error
}

func (db *cleanupFailureDatabase) Run(source jujutxn.TransactionSource) error {
	if len(db.failures) > 0 {
		err := db.failures[0]
		db.failures = db.failures[1:]
		return err
	}
	return db.Database.Run(source)
}
