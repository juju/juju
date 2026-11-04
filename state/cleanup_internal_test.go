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

	"github.com/juju/juju/core/crossmodel"
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
	db := s.failRemovals(force)
	err := s.state.removeRemoteApplicationsForDyingModel(DestroyModelParams{Force: &force})
	c.Check(err, jc.ErrorIs, db.injected[0])
	// Every failure injected for this pass was consumed.
	c.Check(db.failures, gc.HasLen, 0)
	if force {
		c.Check(err, gc.ErrorMatches, `cannot remove remote applications first, second: .*first removal failed`)
	}
	// Nothing was removed, so an unforced pass stopped at the first failure.
	remaining, err := s.state.AllRemoteApplications()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(remaining, gc.HasLen, 2)

	// The failed applications can be removed on the next pass.
	c.Assert(s.state.removeRemoteApplicationsForDyingModel(DestroyModelParams{Force: &force}), jc.ErrorIsNil)
	remaining, err = s.state.AllRemoteApplications()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(remaining, gc.HasLen, 0)
}

func (s *cleanupInternalSuite) TestRemoveApplicationsForDyingModelContinuesAfterFailure(c *gc.C) {
	s.assertRemoveApplicationsAfterFailure(c, true)
}

func (s *cleanupInternalSuite) TestRemoveApplicationsForDyingModelStopsAfterFailure(c *gc.C) {
	s.assertRemoveApplicationsAfterFailure(c, false)
}

func (s *cleanupInternalSuite) assertRemoveApplicationsAfterFailure(c *gc.C, force bool) {
	ch := AddTestingCharm(c, s.state, "wordpress")
	for _, name := range []string{"first", "second"} {
		AddTestingApplication(c, s.state, name, ch)
	}
	db := s.failRemovals(force)
	err := s.state.removeApplicationsForDyingModel(DestroyModelParams{Force: &force}, nil)
	c.Check(err, jc.ErrorIs, db.injected[0])
	// Every failure injected for this pass was consumed.
	c.Check(db.failures, gc.HasLen, 0)
	if force {
		c.Check(err, gc.ErrorMatches, `cannot remove applications first, second: .*first removal failed`)
	}
	// Nothing was removed, so an unforced pass stopped at the first failure.
	remaining, err := s.state.AllApplications()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(remaining, gc.HasLen, 2)

	// The failed applications can be removed on the next pass.
	c.Assert(s.state.removeApplicationsForDyingModel(DestroyModelParams{Force: &force}, nil), jc.ErrorIsNil)
	remaining, err = s.state.AllApplications()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(remaining, gc.HasLen, 0)
}

func (s *cleanupInternalSuite) TestRemoveOffersForDyingModelContinuesAfterFailure(c *gc.C) {
	s.assertRemoveOffersAfterFailure(c, true)
}

func (s *cleanupInternalSuite) TestRemoveOffersForDyingModelStopsAfterFailure(c *gc.C) {
	s.assertRemoveOffersAfterFailure(c, false)
}

func (s *cleanupInternalSuite) assertRemoveOffersAfterFailure(c *gc.C, force bool) {
	AddTestingApplication(c, s.state, "mysql", AddTestingCharm(c, s.state, "mysql"))
	offers := NewApplicationOffers(s.state)
	for _, name := range []string{"first", "second"} {
		_, err := offers.AddOffer(crossmodel.AddApplicationOfferArgs{
			OfferName:       name,
			ApplicationName: "mysql",
			Endpoints:       map[string]string{"server": "server"},
			Owner:           s.owner.Id(),
		})
		c.Assert(err, jc.ErrorIsNil)
	}
	db := s.failRemovals(force)
	err := s.state.removeOffersForDyingModel(force)
	c.Check(err, jc.ErrorIs, db.injected[0])
	// Every failure injected for this pass was consumed.
	c.Check(db.failures, gc.HasLen, 0)
	if force {
		c.Check(err, gc.ErrorMatches, `cannot remove application offers first, second: .*first removal failed`)
	}
	// Nothing was removed, so an unforced pass stopped at the first failure.
	remaining, err := offers.AllApplicationOffers()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(remaining, gc.HasLen, 2)

	// The failed offers can be removed on the next pass.
	c.Assert(s.state.removeOffersForDyingModel(force), jc.ErrorIsNil)
	remaining, err = offers.AllApplicationOffers()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(remaining, gc.HasLen, 0)
}

// failRemovals fails the first removal transaction regardless of which
// entity is read first. A forced pass also attempts the second entity, so
// it fails that too.
func (s *cleanupInternalSuite) failRemovals(force bool) *cleanupFailureDatabase {
	failures := []error{errors.New("first removal failed")}
	if force {
		failures = append(failures, errors.New("second removal failed"))
	}
	db := &cleanupFailureDatabase{
		Database: s.state.database,
		failures: failures,
		injected: failures,
	}
	s.PatchValue(&s.state.database, db)
	return db
}

type cleanupFailureDatabase struct {
	Database
	failures []error
	injected []error
}

func (db *cleanupFailureDatabase) Run(source jujutxn.TransactionSource) error {
	if len(db.failures) > 0 {
		err := db.failures[0]
		db.failures = db.failures[1:]
		return err
	}
	return db.Database.Run(source)
}
