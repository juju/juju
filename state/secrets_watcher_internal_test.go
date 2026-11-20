// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"time"

	"github.com/juju/mgo/v3/bson"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/secrets"
	corewatcher "github.com/juju/juju/core/watcher"
	"github.com/juju/juju/state/watcher"
)

type secretsExpiryWatcherMergeSuite struct{}

var _ = gc.Suite(&secretsExpiryWatcherMergeSuite{})

// TestDeleteMatchesRevision ensures a deletion for one revision does not
// overwrite a pending change for a different revision of the same secret.
func (s *secretsExpiryWatcherMergeSuite) TestDeleteMatchesRevision(c *gc.C) {
	uri := secrets.NewURI()
	next := time.Now()
	w := &secretsExpiryWatcher{
		known: map[string]expiryWatcherDetails{
			"secret-id/1": {
				uri:        uri,
				revision:   1,
				willExpire: true,
			},
		},
	}
	details := []corewatcher.SecretTriggerChange{{
		URI:             uri,
		Revision:        2,
		NextTriggerTime: next,
	}}

	got, err := w.merge(details, watcher.Change{
		Id:    "secret-id/1",
		Revno: -1,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, gc.HasLen, 2)

	var sawDeletion, sawRevision2 bool
	for _, ch := range got {
		switch ch.Revision {
		case 1:
			sawDeletion = ch.NextTriggerTime.IsZero()
		case 2:
			sawRevision2 = ch.NextTriggerTime.Equal(next)
		}
	}
	c.Check(sawDeletion, jc.IsTrue)
	c.Check(sawRevision2, jc.IsTrue)
}

// TestExpireThenDeleteInOneBatch ensures that when a revision that was
// expiring is deleted before a pending expiry trigger is delivered, the
// pending trigger is removed and a single zero-time deletion is appended.
func (s *secretsExpiryWatcherMergeSuite) TestExpireThenDeleteInOneBatch(c *gc.C) {
	uri := secrets.NewURI()
	changeID := "model-uuid:" + uri.ID + "/1"
	next := time.Now()
	w := &secretsExpiryWatcher{
		known: map[string]expiryWatcherDetails{
			changeID: {
				uri:        uri,
				revision:   1,
				willExpire: true,
			},
		},
	}
	details := []corewatcher.SecretTriggerChange{
		{URI: uri, Revision: 1, NextTriggerTime: next},
	}

	got, err := w.merge(details, watcher.Change{
		Id:    changeID,
		Revno: -1,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, gc.HasLen, 1)
	c.Check(got[0].URI.ID, gc.Equals, uri.ID)
	c.Check(got[0].Revision, gc.Equals, 1)
	c.Check(got[0].NextTriggerTime.IsZero(), jc.IsTrue)
}

// TestDeleteUnknownRecord ensures a deletion event for a record the
// watcher does not know about leaves the pending batch untouched.
func (s *secretsExpiryWatcherMergeSuite) TestDeleteUnknownRecord(c *gc.C) {
	uri := secrets.NewURI()
	next := time.Now()
	w := &secretsExpiryWatcher{
		known: make(map[string]expiryWatcherDetails),
	}
	details := []corewatcher.SecretTriggerChange{
		{URI: uri, Revision: 1, NextTriggerTime: next},
	}

	got, err := w.merge(details, watcher.Change{
		Id:    "model-uuid:" + uri.ID + "/1",
		Revno: -1,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, gc.HasLen, 1)
	c.Check(got[0].URI.ID, gc.Equals, uri.ID)
	c.Check(got[0].Revision, gc.Equals, 1)
	c.Check(got[0].NextTriggerTime.Equal(next), jc.IsTrue)
}

// fakeLeaderToken implements leadership.Token.
type fakeLeaderToken struct{}

// Check is part of the leadership.Token interface.
func (fakeLeaderToken) Check() error { return nil }

type secretsExpiryWatcherMergeDBSuite struct {
	internalStateSuite
}

var _ = gc.Suite(&secretsExpiryWatcherMergeDBSuite{})

// newExpiryWatcher returns an expiry watcher over the test state without
// running its loop, so merge can be driven directly.
func (s *secretsExpiryWatcherMergeDBSuite) newExpiryWatcher(owner names.Tag) *secretsExpiryWatcher {
	return &secretsExpiryWatcher{
		commonWatcher: newCommonWatcher(s.state),
		known:         make(map[string]expiryWatcherDetails),
		owners:        []string{owner.String()},
	}
}

// createExpiringSecret creates revision 1 of a secret owned by owner,
// expiring at the given time.
func (s *secretsExpiryWatcherMergeDBSuite) createExpiringSecret(
	c *gc.C, owner names.Tag, expire time.Time,
) *secrets.URI {
	uri := secrets.NewURI()
	_, err := NewSecrets(s.state).CreateSecret(uri, CreateSecretParams{
		Version: 1,
		Owner:   owner,
		UpdateSecretParams: UpdateSecretParams{
			LeaderToken: &fakeLeaderToken{},
			ExpireTime:  ptr(expire),
			Data:        map[string]string{"foo": "bar"},
			Checksum:    "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b",
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	return uri
}

// TestUpdateMatchesRevision ensures an update event for one revision does
// not clobber a pending trigger for a different revision of the same
// secret; both entries survive and the updated revision lands at the tail.
func (s *secretsExpiryWatcherMergeDBSuite) TestUpdateMatchesRevision(c *gc.C) {
	owner := names.NewModelTag(s.state.ModelUUID())
	next1 := s.clock.Now().Round(time.Second).UTC().Add(time.Minute)
	next2 := next1.Add(time.Hour)
	uri := s.createExpiringSecret(c, owner, next1)

	w := s.newExpiryWatcher(owner)
	details, err := w.initial()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(details, gc.HasLen, 1)

	// Saving new content queues a second revision with its own expiry.
	_, err = NewSecrets(s.state).UpdateSecret(uri, UpdateSecretParams{
		LeaderToken: &fakeLeaderToken{},
		ExpireTime:  ptr(next2),
		Data:        map[string]string{"foo": "baz"},
	})
	c.Assert(err, jc.ErrorIsNil)

	got, err := w.merge(details, watcher.Change{
		Id:    s.state.docID(secretRevisionKey(uri, 2)),
		Revno: 1,
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(got, gc.HasLen, 2)
	c.Check(got[0].URI.ID, gc.Equals, uri.ID)
	c.Check(got[0].Revision, gc.Equals, 1)
	c.Check(got[0].NextTriggerTime.Equal(next1), jc.IsTrue)
	c.Check(got[1].URI.ID, gc.Equals, uri.ID)
	c.Check(got[1].Revision, gc.Equals, 2)
	c.Check(got[1].NextTriggerTime.Equal(next2), jc.IsTrue)
}

// TestUnexpireThenDeletePreservesCancellation ensures that if an expiring
// revision is un-expired (queuing a zero-time cancellation) and then deleted
// before the batch is delivered, the cancellation is preserved.
func (s *secretsExpiryWatcherMergeDBSuite) TestUnexpireThenDeletePreservesCancellation(c *gc.C) {
	owner := names.NewModelTag(s.state.ModelUUID())
	next := s.clock.Now().Round(time.Second).UTC().Add(time.Minute)
	uri := s.createExpiringSecret(c, owner, next)

	w := s.newExpiryWatcher(owner)
	details, err := w.initial()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(details, gc.HasLen, 1)

	// Un-expiring the revision queues a zero-time cancellation in place
	// of the pending expiry trigger.
	changeID := s.state.docID(secretRevisionKey(uri, 1))
	_, err = NewSecrets(s.state).UpdateSecret(uri, UpdateSecretParams{
		LeaderToken: &fakeLeaderToken{},
		ExpireTime:  ptr(time.Time{}),
	})
	c.Assert(err, jc.ErrorIsNil)
	details, err = w.merge(details, watcher.Change{
		Id:    changeID,
		Revno: 2,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(details, gc.HasLen, 1)
	c.Check(details[0].URI.ID, gc.Equals, uri.ID)
	c.Check(details[0].Revision, gc.Equals, 1)
	c.Check(details[0].NextTriggerTime.IsZero(), jc.IsTrue)

	// Deleting the revision before the batch is delivered preserves the
	// cancellation.
	got, err := w.merge(details, watcher.Change{
		Id:    changeID,
		Revno: -1,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, gc.HasLen, 1)
	c.Check(got[0].URI.ID, gc.Equals, uri.ID)
	c.Check(got[0].Revision, gc.Equals, 1)
	c.Check(got[0].NextTriggerTime.IsZero(), jc.IsTrue)
}

// TestUpdateForVanishedRevision ensures an update event for a revision
// whose document has already been removed is ignored: the pending batch
// and the known details are left untouched.
func (s *secretsExpiryWatcherMergeDBSuite) TestUpdateForVanishedRevision(c *gc.C) {
	owner := names.NewModelTag(s.state.ModelUUID())
	next := s.clock.Now().Round(time.Second).UTC().Add(time.Minute)
	uri := s.createExpiringSecret(c, owner, next)

	w := s.newExpiryWatcher(owner)
	details, err := w.initial()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(details, gc.HasLen, 1)

	// Remove the revision doc so the merge fetch finds nothing.
	revColl, closer, err := s.state.db().GetCollection(secretRevisionsC)
	c.Assert(err, jc.ErrorIsNil)
	defer closer()
	err = revColl.Writeable().RemoveId(s.state.docID(secretRevisionKey(uri, 1)))
	c.Assert(err, jc.ErrorIsNil)

	got, err := w.merge(details, watcher.Change{
		Id:    s.state.docID(secretRevisionKey(uri, 1)),
		Revno: 2,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, gc.HasLen, 1)
	c.Check(got[0].URI.ID, gc.Equals, uri.ID)
	c.Check(got[0].Revision, gc.Equals, 1)
	c.Check(got[0].NextTriggerTime.Equal(next), jc.IsTrue)
	c.Check(w.known, gc.HasLen, 1)
}

type secretsRotationWatcherMergeDBSuite struct {
	internalStateSuite
}

var _ = gc.Suite(&secretsRotationWatcherMergeDBSuite{})

// TestUpdateReplacesPendingTrigger ensures an update event replaces the
// pending trigger for the same secret, at the tail, rather than being
// appended after it.
func (s *secretsRotationWatcherMergeDBSuite) TestUpdateReplacesPendingTrigger(c *gc.C) {
	owner := names.NewModelTag(s.state.ModelUUID())
	next1 := s.clock.Now().Round(time.Second).UTC().Add(2 * time.Hour)
	next2 := s.clock.Now().Round(time.Second).UTC().Add(time.Hour)
	uri := secrets.NewURI()
	_, err := NewSecrets(s.state).CreateSecret(uri, CreateSecretParams{
		Version: 1,
		Owner:   owner,
		UpdateSecretParams: UpdateSecretParams{
			LeaderToken:    &fakeLeaderToken{},
			NextRotateTime: ptr(next1),
			Data:           map[string]string{"foo": "bar"},
			Checksum:       "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b",
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	w := &secretsRotationWatcher{
		commonWatcher: newCommonWatcher(s.state),
		known:         make(map[string]rotateWatcherDetails),
		owners:        []string{owner.String()},
	}
	details, err := w.initial()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(details, gc.HasLen, 1)

	// Rotation updates only apply when the new time is earlier.
	_, err = NewSecrets(s.state).UpdateSecret(uri, UpdateSecretParams{
		LeaderToken:    &fakeLeaderToken{},
		NextRotateTime: ptr(next2),
	})
	c.Assert(err, jc.ErrorIsNil)

	got, err := w.merge(details, watcher.Change{
		Id:    s.state.docID(uri.ID),
		Revno: 2,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, gc.HasLen, 1)
	c.Check(got[0].URI.ID, gc.Equals, uri.ID)
	c.Check(got[0].NextTriggerTime.Equal(next2), jc.IsTrue)
}

type secretBackendRotationWatcherMergeDBSuite struct {
	internalStateSuite
}

var _ = gc.Suite(&secretBackendRotationWatcherMergeDBSuite{})

// TestUpdateReplacesPendingTrigger ensures an update event replaces the
// pending trigger for the same backend, at the tail, while the pending
// trigger for another backend is retained.
func (s *secretBackendRotationWatcherMergeDBSuite) TestUpdateReplacesPendingTrigger(c *gc.C) {
	next := s.clock.Now().Round(time.Second).UTC()
	next1 := next.Add(time.Hour)
	next2 := next.Add(2 * time.Hour)
	next3 := next.Add(3 * time.Hour)

	// Seed the backend rotation docs directly; the txn-revno is bumped by
	// hand since the merge only processes changes for docs whose revno has
	// advanced.
	rotateColl, closer, err := s.state.db().GetCollection(secretBackendsRotateC)
	c.Assert(err, jc.ErrorIsNil)
	defer closer()
	err = rotateColl.Writeable().Insert(secretBackendRotationDoc{
		DocID:          "backend-1",
		TxnRevno:       1,
		Name:           "backend-1",
		NextRotateTime: next1,
	})
	c.Assert(err, jc.ErrorIsNil)
	err = rotateColl.Writeable().Insert(secretBackendRotationDoc{
		DocID:          "backend-2",
		TxnRevno:       1,
		Name:           "backend-2",
		NextRotateTime: next2,
	})
	c.Assert(err, jc.ErrorIsNil)

	w := &secretBackendRotationWatcher{
		commonWatcher: newCommonWatcher(s.state),
		known:         make(map[string]secretBackendRotateWatcherDetails),
	}
	details, err := w.initial()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(details, gc.HasLen, 2)

	// The backend rotation collection is controller-scoped, so doc IDs
	// carry no model UUID prefix.
	err = rotateColl.Writeable().UpdateId("backend-1", bson.M{
		"$set": bson.M{"next-rotate-time": next3},
		"$inc": bson.M{"txn-revno": 1},
	})
	c.Assert(err, jc.ErrorIsNil)

	got, err := w.merge(details, watcher.Change{
		Id:    "backend-1",
		Revno: 2,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, gc.HasLen, 2)
	c.Check(got[0].ID, gc.Equals, "backend-2")
	c.Check(got[0].NextTriggerTime.Equal(next2), jc.IsTrue)
	c.Check(got[1].ID, gc.Equals, "backend-1")
	c.Check(got[1].Name, gc.Equals, "backend-1")
	c.Check(got[1].NextTriggerTime.Equal(next3), jc.IsTrue)
}

type secretsRotationWatcherMergeSuite struct{}

var _ = gc.Suite(&secretsRotationWatcherMergeSuite{})

// TestDeleteRemovesPriorUpdates ensures a deletion clears any older pending
// updates for the same secret and appends the deletion at the tail.
func (s *secretsRotationWatcherMergeSuite) TestDeleteRemovesPriorUpdates(c *gc.C) {
	uri := secrets.NewURI()
	otherURI := secrets.NewURI()
	changeID := "model-uuid:" + uri.ID
	next := time.Now()
	w := &secretsRotationWatcher{
		known: map[string]rotateWatcherDetails{
			changeID: {URI: uri},
		},
	}
	details := []corewatcher.SecretTriggerChange{
		{URI: uri, NextTriggerTime: next},
		{URI: otherURI, NextTriggerTime: next.Add(time.Hour)},
		{URI: uri, NextTriggerTime: next.Add(2 * time.Hour)},
	}

	got, err := w.merge(details, watcher.Change{
		Id:    changeID,
		Revno: -1,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, gc.HasLen, 2)
	c.Check(got[0].URI.ID, gc.Equals, otherURI.ID)
	c.Check(got[1].URI.ID, gc.Equals, uri.ID)
	c.Check(got[1].NextTriggerTime.IsZero(), jc.IsTrue)
}

// TestDeleteUnknownRecord ensures a deletion event for a record the
// watcher does not know about leaves the pending batch untouched.
func (s *secretsRotationWatcherMergeSuite) TestDeleteUnknownRecord(c *gc.C) {
	uri := secrets.NewURI()
	next := time.Now()
	w := &secretsRotationWatcher{
		known: make(map[string]rotateWatcherDetails),
	}
	details := []corewatcher.SecretTriggerChange{
		{URI: uri, NextTriggerTime: next},
	}

	got, err := w.merge(details, watcher.Change{
		Id:    "model-uuid:" + uri.ID,
		Revno: -1,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, gc.HasLen, 1)
	c.Check(got[0].URI.ID, gc.Equals, uri.ID)
	c.Check(got[0].NextTriggerTime.Equal(next), jc.IsTrue)
}

type secretBackendRotationWatcherMergeSuite struct{}

var _ = gc.Suite(&secretBackendRotationWatcherMergeSuite{})

// TestDeleteRemovesPriorUpdates ensures a deletion clears any older pending
// updates for the same backend and appends the deletion at the tail.
func (s *secretBackendRotationWatcherMergeSuite) TestDeleteRemovesPriorUpdates(c *gc.C) {
	next := time.Now()
	w := &secretBackendRotationWatcher{
		known: map[string]secretBackendRotateWatcherDetails{
			"backend-id/1": {ID: "backend-id/1", Name: "backend-1"},
		},
	}
	details := []corewatcher.SecretBackendRotateChange{
		{ID: "backend-id/1", Name: "backend-1", NextTriggerTime: next},
		{ID: "backend-id/2", Name: "backend-2", NextTriggerTime: next.Add(time.Hour)},
		{ID: "backend-id/1", Name: "backend-1", NextTriggerTime: next.Add(2 * time.Hour)},
	}

	got, err := w.merge(details, watcher.Change{
		Id:    "backend-id/1",
		Revno: -1,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, gc.HasLen, 2)
	c.Check(got[0].ID, gc.Equals, "backend-id/2")
	c.Check(got[0].NextTriggerTime.Equal(next.Add(time.Hour)), jc.IsTrue)
	c.Check(got[1].ID, gc.Equals, "backend-id/1")
	c.Check(got[1].Name, gc.Equals, "backend-1")
	c.Check(got[1].NextTriggerTime.IsZero(), jc.IsTrue)
}

// TestDeleteUnknownRecord ensures a deletion event for a record the
// watcher does not know about leaves the pending batch untouched.
func (s *secretBackendRotationWatcherMergeSuite) TestDeleteUnknownRecord(c *gc.C) {
	next := time.Now()
	w := &secretBackendRotationWatcher{
		known: make(map[string]secretBackendRotateWatcherDetails),
	}
	details := []corewatcher.SecretBackendRotateChange{
		{ID: "backend-id/1", Name: "backend-1", NextTriggerTime: next},
	}

	got, err := w.merge(details, watcher.Change{
		Id:    "backend-id/1",
		Revno: -1,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, gc.HasLen, 1)
	c.Check(got[0].ID, gc.Equals, "backend-id/1")
	c.Check(got[0].NextTriggerTime.Equal(next), jc.IsTrue)
}
