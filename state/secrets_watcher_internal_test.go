// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"time"

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

// TestUnexpireThenDeletePreservesCancellation ensures that if an expiring
// revision is un-expired (queuing a zero-time cancellation) and then deleted
// before the batch is delivered, the cancellation is preserved.
func (s *secretsExpiryWatcherMergeSuite) TestUnexpireThenDeletePreservesCancellation(c *gc.C) {
	uri := secrets.NewURI()
	changeID := "model-uuid:" + uri.ID + "/1"
	w := &secretsExpiryWatcher{
		known: map[string]expiryWatcherDetails{
			changeID: {
				uri:        uri,
				revision:   1,
				willExpire: false,
			},
		},
	}
	details := []corewatcher.SecretTriggerChange{{
		URI:      uri,
		Revision: 1,
	}}

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
