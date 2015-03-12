// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"time"

	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v4/hooks"

	"github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	corestorage "github.com/juju/juju/storage"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/storage"
)

const initiallyUnattached = false
const initiallyAttached = true

type storageHookQueueSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&storageHookQueueSuite{})

func newHookQueue(attached bool) storage.StorageHookQueue {
	return storage.NewStorageHookQueue(
		names.NewUnitTag("mysql/0"),
		names.NewStorageTag("data/0"),
		attached,
	)
}

func updateHookQueue(c *gc.C, q storage.StorageHookQueue, life params.Life) {
	err := q.Update(params.StorageAttachment{
		Life:     life,
		Kind:     params.StorageKindBlock,
		Location: "/dev/sdb",
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *storageHookQueueSuite) TestStorageHookQueueAttachedHook(c *gc.C) {
	q := newHookQueue(initiallyUnattached)
	updateHookQueue(c, q, params.Alive)
	c.Assert(q.Empty(), jc.IsFalse)
	c.Assert(q.Next(), gc.Equals, hook.Info{
		Kind:      hooks.StorageAttached,
		StorageId: "data/0",
	})
}

func (s *storageHookQueueSuite) TestStorageHookQueueAlreadyAttached(c *gc.C) {
	q := newHookQueue(initiallyAttached)
	updateHookQueue(c, q, params.Alive)
	// Already attached, so no hooks should have been queued.
	c.Assert(q.Empty(), jc.IsTrue)
}

func (s *storageHookQueueSuite) TestStorageHookQueueAttachedDetach(c *gc.C) {
	q := newHookQueue(initiallyAttached)
	updateHookQueue(c, q, params.Dying)
	c.Assert(q.Empty(), jc.IsFalse)
	c.Assert(q.Next(), gc.Equals, hook.Info{
		Kind:      hooks.StorageDetached,
		StorageId: "data/0",
	})
}

func (s *storageHookQueueSuite) TestStorageHookQueueUnattachedDetach(c *gc.C) {
	q := newHookQueue(initiallyUnattached)
	updateHookQueue(c, q, params.Dying)
	// the storage wasn't attached, so Dying short-circuits.
	c.Assert(q.Empty(), jc.IsTrue)
}

func (s *storageHookQueueSuite) TestStorageHookQueueAttachedUnconsumedDetach(c *gc.C) {
	q := newHookQueue(initiallyUnattached)
	updateHookQueue(c, q, params.Alive)
	c.Assert(q.Next(), gc.Equals, hook.Info{
		Kind:      hooks.StorageAttached,
		StorageId: "data/0",
	})
	// don't consume the storage-attached hook; it should then be unqueued
	updateHookQueue(c, q, params.Dying)
	// since the storage-attached hook wasn't consumed, Dying short-circuits.
	c.Assert(q.Empty(), jc.IsTrue)
}

func (s *storageHookQueueSuite) TestStorageHookQueueAttachDetach(c *gc.C) {
	q := newHookQueue(initiallyUnattached)
	updateHookQueue(c, q, params.Alive)
	q.Pop()
	updateHookQueue(c, q, params.Dying)
	c.Assert(q.Empty(), jc.IsFalse)
	c.Assert(q.Next(), gc.Equals, hook.Info{
		Kind:      hooks.StorageDetached,
		StorageId: "data/0",
	})
}

func (s *storageHookQueueSuite) TestStorageHookQueueDead(c *gc.C) {
	q := newHookQueue(initiallyAttached)
	updateHookQueue(c, q, params.Dying)
	q.Pop()
	updateHookQueue(c, q, params.Dead)
	// Dead does not cause any hook to be queued.
	c.Assert(q.Empty(), jc.IsTrue)
}

func (s *storageHookQueueSuite) TestStorageHookQueueContext(c *gc.C) {
	q := newHookQueue(initiallyUnattached)
	_, ok := q.Context()
	c.Assert(ok, jc.IsFalse)

	err := q.Update(params.StorageAttachment{
		Life:     params.Alive,
		Kind:     params.StorageKindFilesystem,
		Location: "/srv",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(q.Empty(), jc.IsFalse)

	ctx, ok := q.Context()
	c.Assert(ok, jc.IsTrue)
	c.Assert(ctx, gc.NotNil)
	c.Assert(ctx.Tag(), gc.Equals, names.NewStorageTag("data/0"))
	c.Assert(ctx.Kind(), gc.Equals, corestorage.StorageKindFilesystem)
	c.Assert(ctx.Location(), gc.Equals, "/srv")
}

func (s *storageHookQueueSuite) TestStorageHookQueueEmpty(c *gc.C) {
	q := newHookQueue(initiallyAttached)
	c.Assert(q.Empty(), jc.IsTrue)
	c.Assert(q.Next, gc.PanicMatches, "source is empty")
	c.Assert(q.Pop, gc.PanicMatches, "source is empty")
}

func (s *storageHookQueueSuite) TestStorageSourceStop(c *gc.C) {
	unitTag := names.NewUnitTag("mysql/0")
	storageTag := names.NewStorageTag("data/0")

	// Simulate remote state returning a single Alive storage attachment.
	st := &mockStorageAccessor{
		watchStorageAttachment: func(s names.StorageTag, u names.UnitTag) (watcher.NotifyWatcher, error) {
			return newMockNotifyWatcher(), nil
		},
	}

	const initiallyUnattached = false
	source, err := storage.NewStorageSource(st, unitTag, storageTag, initiallyUnattached)
	c.Assert(err, jc.ErrorIsNil)
	err = source.Stop()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *storageHookQueueSuite) TestStorageSourceUpdateErrors(c *gc.C) {
	unitTag := names.NewUnitTag("mysql/0")
	storageTag := names.NewStorageTag("data/0")

	// Simulate remote state returning a single Alive storage attachment.
	var calls int
	w := newMockNotifyWatcher()
	st := &mockStorageAccessor{
		watchStorageAttachment: func(s names.StorageTag, u names.UnitTag) (watcher.NotifyWatcher, error) {
			return w, nil
		},
		storageAttachment: func(s names.StorageTag, u names.UnitTag) (params.StorageAttachment, error) {
			calls++
			switch calls {
			case 1:
				return params.StorageAttachment{}, &params.Error{Code: params.CodeNotFound}
			case 2:
				return params.StorageAttachment{}, &params.Error{Code: params.CodeNotProvisioned}
			case 3:
				// This error should cause the source to stop with an error.
				return params.StorageAttachment{}, &params.Error{
					Code:    params.CodeUnauthorized,
					Message: "unauthorized",
				}
			}
			panic("unexpected call to StorageAttachment")
		},
	}

	const initiallyUnattached = false
	source, err := storage.NewStorageSource(st, unitTag, storageTag, initiallyUnattached)
	c.Assert(err, jc.ErrorIsNil)

	assertNoSourceChange := func() {
		select {
		case <-source.Changes():
			c.Fatal("unexpected source change")
		case <-time.After(testing.ShortWait):
		}
	}
	waitSourceChange := func() hook.SourceChange {
		select {
		case ch, ok := <-source.Changes():
			c.Assert(ok, jc.IsTrue)
			assertNoSourceChange()
			return ch
		case <-time.After(testing.LongWait):
			c.Fatal("timed out waiting for source change")
			panic("unreachable")
		}
	}

	assertNoSourceChange()

	// First change is "NotFound": not an error.
	w.changes <- struct{}{}
	change := waitSourceChange()
	c.Assert(change(), jc.ErrorIsNil)

	// Second change is "NotProvisioned": not an error.
	w.changes <- struct{}{}
	change = waitSourceChange()
	c.Assert(change(), jc.ErrorIsNil)

	// Third change is "Unauthorized": this *is* an error.
	w.changes <- struct{}{}
	change = waitSourceChange()
	c.Assert(change(), gc.ErrorMatches, "refreshing storage details: unauthorized")
}
