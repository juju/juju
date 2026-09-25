// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"time"

	"github.com/juju/mgo/v3/bson"
	"github.com/juju/mgo/v3/txn"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coretesting "github.com/juju/juju/testing"
)

type remoteApplicationWatcherSuite struct {
	internalStateSuite
}

var _ = gc.Suite(&remoteApplicationWatcherSuite{})

func (s *remoteApplicationWatcherSuite) TestIdentityChangesWithoutLifeChange(c *gc.C) {
	app, err := s.state.AddRemoteApplication(AddRemoteApplicationParams{
		Name: "remote", SourceModel: names.NewModelTag(s.state.ModelUUID()),
		OfferUUID: "old-offer",
	})
	c.Assert(err, jc.ErrorIsNil)
	w := s.state.WatchRemoteApplications()
	defer func() { c.Check(w.Stop(), jc.ErrorIsNil) }()
	s.assertChanges(c, w, "remote")
	s.assertNoChanges(c, w)

	// The watcher can observe the final Alive document after replacement
	// without seeing the intermediate deletion. Exercise identity changes
	// independently, keeping the name and life unchanged.
	for _, fields := range []bson.D{
		{{"offer-uuid", "new-offer"}},
		{{"version", app.ConsumeVersion() + 1}},
	} {
		c.Assert(s.state.db().RunTransaction([]txn.Op{{
			C: remoteApplicationsC, Id: app.doc.DocID,
			Assert: isAliveDoc, Update: bson.D{{"$set", fields}},
		}}), jc.ErrorIsNil)
		s.assertChanges(c, w, "remote")
		s.assertNoChanges(c, w)
	}

	// Other document changes do not restart the application worker.
	c.Assert(s.state.db().RunTransaction([]txn.Op{{
		C: remoteApplicationsC, Id: app.doc.DocID,
		Assert: isAliveDoc, Update: bson.D{{"$set", bson.D{{"url", "admin/model.remote"}}}},
	}}), jc.ErrorIsNil)
	s.assertNoChanges(c, w)
	c.Assert(app.Destroy(), jc.ErrorIsNil)
	s.assertChanges(c, w, "remote")
	s.assertNoChanges(c, w)

	_, err = s.state.AddRemoteApplication(AddRemoteApplicationParams{
		Name: "remote", SourceModel: names.NewModelTag(s.state.ModelUUID()),
		OfferUUID: "replacement-offer",
	})
	c.Assert(err, jc.ErrorIsNil)
	s.assertChanges(c, w, "remote")
	s.assertNoChanges(c, w)
}

func (s *remoteApplicationWatcherSuite) TestLifecycleChanges(c *gc.C) {
	app, err := s.state.AddRemoteApplication(AddRemoteApplicationParams{
		Name: "remote", SourceModel: names.NewModelTag(s.state.ModelUUID()),
	})
	c.Assert(err, jc.ErrorIsNil)
	w := s.state.WatchRemoteApplications()
	defer func() { c.Check(w.Stop(), jc.ErrorIsNil) }()
	s.assertChanges(c, w, "remote")

	for _, life := range []Life{Dying, Dead} {
		c.Assert(s.state.db().RunTransaction([]txn.Op{{
			C: remoteApplicationsC, Id: app.doc.DocID,
			Assert: txn.DocExists, Update: bson.D{{"$set", bson.D{{"life", life}}}},
		}}), jc.ErrorIsNil)
		s.assertChanges(c, w, "remote")
		s.assertNoChanges(c, w)
	}

	// Once Dead has been reported, further changes and deletion are silent.
	c.Assert(s.state.db().RunTransaction([]txn.Op{{
		C: remoteApplicationsC, Id: app.doc.DocID,
		Assert: txn.DocExists, Update: bson.D{{"$inc", bson.D{{"version", 1}}}},
	}}), jc.ErrorIsNil)
	s.assertNoChanges(c, w)
	c.Assert(s.state.db().RunTransaction([]txn.Op{{
		C: remoteApplicationsC, Id: app.doc.DocID,
		Assert: txn.DocExists, Remove: true,
	}}), jc.ErrorIsNil)
	s.assertNoChanges(c, w)
}

func (s *remoteApplicationWatcherSuite) TestInitialDeadApplication(c *gc.C) {
	app, err := s.state.AddRemoteApplication(AddRemoteApplicationParams{
		Name: "remote", SourceModel: names.NewModelTag(s.state.ModelUUID()),
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.state.db().RunTransaction([]txn.Op{{
		C: remoteApplicationsC, Id: app.doc.DocID,
		Assert: isAliveDoc, Update: bson.D{{"$set", bson.D{{"life", Dead}}}},
	}}), jc.ErrorIsNil)
	w := s.state.WatchRemoteApplications()
	defer func() { c.Check(w.Stop(), jc.ErrorIsNil) }()
	s.assertChanges(c, w, "remote")
	s.assertNoChanges(c, w)
	c.Assert(s.state.db().RunTransaction([]txn.Op{{
		C: remoteApplicationsC, Id: app.doc.DocID,
		Assert: txn.DocExists, Remove: true,
	}}), jc.ErrorIsNil)
	s.assertNoChanges(c, w)
}

func (s *remoteApplicationWatcherSuite) TestStopsWhenStateCloses(c *gc.C) {
	w := s.state.WatchRemoteApplications()
	defer w.Stop()
	s.assertChanges(c, w)
	c.Assert(s.controller.Close(), jc.ErrorIsNil)
	select {
	case _, ok := <-w.Changes():
		c.Check(ok, jc.IsFalse)
	case <-time.After(coretesting.LongWait):
		c.Fatal("remote application watcher did not close")
	}
	c.Check(w.Wait(), jc.ErrorIs, ErrStateClosed)
}

func (s *remoteApplicationWatcherSuite) assertChanges(c *gc.C, w StringsWatcher, expected ...string) {
	select {
	case changes, ok := <-w.Changes():
		c.Assert(ok, jc.IsTrue)
		c.Check(changes, jc.SameContents, expected)
	case <-time.After(coretesting.LongWait):
		c.Fatal("no remote application change")
	}
}

func (s *remoteApplicationWatcherSuite) assertNoChanges(c *gc.C, w StringsWatcher) {
	select {
	case changes := <-w.Changes():
		c.Fatalf("unexpected remote application change: %v", changes)
	case <-time.After(coretesting.ShortWait):
	}
}
