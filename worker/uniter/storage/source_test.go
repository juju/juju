// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v4/hooks"

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
	c.Assert(q.Context, gc.PanicMatches, "no hooks have been queued")

	err := q.Update(params.StorageAttachment{
		Life:     params.Alive,
		Kind:     params.StorageKindFilesystem,
		Location: "/srv",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(q.Empty(), jc.IsFalse)

	ctx := q.Context()
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
