// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remotestate

import (
	stdtesting "testing"

	"github.com/juju/tc"
	"github.com/juju/worker/v5/catacomb"

	"github.com/juju/juju/internal/testing"
)

type WatcherInternalSuite struct {
	testing.BaseSuite
}

func TestWatcherInternalSuite(t *stdtesting.T) {
	tc.Run(t, &WatcherInternalSuite{})
}

func (s *WatcherInternalSuite) TestDeleteSecretSignal(c *tc.C) {
	w := RemoteStateWatcher{
		current: Snapshot{
			ObsoleteSecretRevisions: map[string][]int{
				"secret:9m4e2mr0ui3e8a215n4g": {665, 666},
				"secret:777e2mr0ui3e8a215n4g": {777},
			},
			DeletedSecretRevisions: map[string][]int{
				"secret:9m4e2mr0ui3e8a215n4g": {665},
			},
		},
	}
	w.RemoveSecretsCompleted(map[string][]int{
		"secret:9m4e2mr0ui3e8a215n4g": {665},
		"secret:777e2mr0ui3e8a215n4g": {},
	})
	c.Assert(w.current.DeletedSecretRevisions, tc.HasLen, 0)
	c.Assert(w.current.ObsoleteSecretRevisions, tc.DeepEquals, map[string][]int{
		"secret:9m4e2mr0ui3e8a215n4g": {666},
	})
}

func (s *WatcherInternalSuite) TestScopedContextDetachedFromCatacomb(c *tc.C) {
	var w RemoteStateWatcher
	err := catacomb.Invoke(catacomb.Plan{
		Name: "remote-state-watcher",
		Site: &w.catacomb,
		Work: func() error {
			<-w.catacomb.Dying()
			return w.catacomb.ErrDying()
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	ctx, cancel := w.scopedContext()
	defer cancel()

	w.catacomb.Kill(nil)
	select {
	case <-w.catacomb.Dead():
	case <-c.Context().Done():
		c.Fatal("catacomb did not die")
	}

	// The context must survive the catacomb's death so that an in-flight
	// operation (e.g. a storage-detaching hook) is not cancelled by teardown.
	select {
	case <-ctx.Done():
		c.Fatal("scoped context should not be cancelled by catacomb death")
	default:
	}

	cancel()
	select {
	case <-ctx.Done():
	default:
		c.Fatal("scoped context should be cancelled by its cancel func")
	}
}

func (s *WatcherInternalSuite) TestStorageWatcherScopedContextDetachedFromCatacomb(c *tc.C) {
	var sw storageAttachmentWatcher
	err := catacomb.Invoke(catacomb.Plan{
		Name: "storage-attachment-watcher",
		Site: &sw.catacomb,
		Work: func() error {
			<-sw.catacomb.Dying()
			return sw.catacomb.ErrDying()
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	ctx, cancel := sw.scopedContext()
	defer cancel()

	sw.catacomb.Kill(nil)
	select {
	case <-sw.catacomb.Dead():
	case <-c.Context().Done():
		c.Fatal("catacomb did not die")
	}

	select {
	case <-ctx.Done():
		c.Fatal("scoped context should not be cancelled by catacomb death")
	default:
	}

	cancel()
	select {
	case <-ctx.Done():
	default:
		c.Fatal("scoped context should be cancelled by its cancel func")
	}
}
