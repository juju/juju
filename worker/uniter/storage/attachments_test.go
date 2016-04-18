// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"io/ioutil"
	"path/filepath"

	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/set"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable/hooks"

	"github.com/juju/juju/apiserver/params"
	corestorage "github.com/juju/juju/storage"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/operation"
	"github.com/juju/juju/worker/uniter/remotestate"
	"github.com/juju/juju/worker/uniter/resolver"
	"github.com/juju/juju/worker/uniter/storage"
)

type attachmentsSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&attachmentsSuite{})

func assertStorageTags(c *gc.C, a *storage.Attachments, tags ...names.StorageTag) {
	sTags, err := a.StorageTags()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sTags, jc.SameContents, tags)
}

func (s *attachmentsSuite) TestNewAttachments(c *gc.C) {
	stateDir := filepath.Join(c.MkDir(), "nonexistent")
	unitTag := names.NewUnitTag("mysql/0")
	abort := make(chan struct{})
	st := &mockStorageAccessor{
		unitStorageAttachments: func(u names.UnitTag) ([]params.StorageAttachmentId, error) {
			c.Assert(u, gc.Equals, unitTag)
			return nil, nil
		},
	}

	_, err := storage.NewAttachments(st, unitTag, stateDir, abort)
	c.Assert(err, jc.ErrorIsNil)
	// state dir should have been created.
	c.Assert(stateDir, jc.IsDirectory)
}

func (s *attachmentsSuite) TestNewAttachmentsInit(c *gc.C) {
	stateDir := c.MkDir()
	unitTag := names.NewUnitTag("mysql/0")
	abort := make(chan struct{})

	// Simulate remote state returning a single Alive storage attachment.
	storageTag := names.NewStorageTag("data/0")
	attachmentIds := []params.StorageAttachmentId{{
		StorageTag: storageTag.String(),
		UnitTag:    unitTag.String(),
	}}
	attachment := params.StorageAttachment{
		StorageTag: storageTag.String(),
		UnitTag:    unitTag.String(),
		Life:       params.Alive,
		Kind:       params.StorageKindBlock,
		Location:   "/dev/sdb",
	}

	st := &mockStorageAccessor{
		unitStorageAttachments: func(u names.UnitTag) ([]params.StorageAttachmentId, error) {
			c.Assert(u, gc.Equals, unitTag)
			return attachmentIds, nil
		},
		storageAttachment: func(s names.StorageTag, u names.UnitTag) (params.StorageAttachment, error) {
			c.Assert(s, gc.Equals, storageTag)
			return attachment, nil
		},
	}

	withAttachments := func(f func(*storage.Attachments)) {
		att, err := storage.NewAttachments(st, unitTag, stateDir, abort)
		c.Assert(err, jc.ErrorIsNil)
		f(att)
	}

	// No state files, so no storagers will be started.
	var called int
	withAttachments(func(att *storage.Attachments) {
		called++
		c.Assert(att.Pending(), gc.Equals, 1)
		err := att.ValidateHook(hook.Info{
			Kind:      hooks.StorageAttached,
			StorageId: storageTag.Id(),
		})
		c.Assert(err, gc.ErrorMatches, `unknown storage "data/0"`)
		assertStorageTags(c, att) // no active attachment
	})
	c.Assert(called, gc.Equals, 1)

	// Commit a storage-attached to local state and try again.
	state0, err := storage.ReadStateFile(stateDir, storageTag)
	c.Assert(err, jc.ErrorIsNil)
	err = state0.CommitHook(hook.Info{Kind: hooks.StorageAttached, StorageId: "data/0"})
	c.Assert(err, jc.ErrorIsNil)
	// Create an extra one so we can make sure it gets removed.
	state1, err := storage.ReadStateFile(stateDir, names.NewStorageTag("data/1"))
	c.Assert(err, jc.ErrorIsNil)
	err = state1.CommitHook(hook.Info{Kind: hooks.StorageAttached, StorageId: "data/1"})
	c.Assert(err, jc.ErrorIsNil)

	withAttachments(func(att *storage.Attachments) {
		// We should be able to get the initial storage context
		// for existing storage immediately, without having to
		// wait for any hooks to fire.
		ctx, err := att.Storage(storageTag)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(ctx, gc.NotNil)
		c.Assert(ctx.Tag(), gc.Equals, storageTag)
		c.Assert(ctx.Tag(), gc.Equals, storageTag)
		c.Assert(ctx.Kind(), gc.Equals, corestorage.StorageKindBlock)
		c.Assert(ctx.Location(), gc.Equals, "/dev/sdb")

		called++
		c.Assert(att.Pending(), gc.Equals, 0)
		err = att.ValidateHook(hook.Info{
			Kind:      hooks.StorageDetaching,
			StorageId: storageTag.Id(),
		})
		c.Assert(err, jc.ErrorIsNil)
		err = att.ValidateHook(hook.Info{
			Kind:      hooks.StorageAttached,
			StorageId: "data/1",
		})
		c.Assert(err, gc.ErrorMatches, `unknown storage "data/1"`)
		assertStorageTags(c, att, storageTag)
	})
	c.Assert(called, gc.Equals, 2)
	c.Assert(filepath.Join(stateDir, "data-0"), jc.IsNonEmptyFile)
	c.Assert(filepath.Join(stateDir, "data-1"), jc.DoesNotExist)
}

func (s *attachmentsSuite) TestAttachmentsUpdateShortCircuitDeath(c *gc.C) {
	stateDir := c.MkDir()
	unitTag := names.NewUnitTag("mysql/0")
	abort := make(chan struct{})

	storageTag0 := names.NewStorageTag("data/0")
	storageTag1 := names.NewStorageTag("data/1")

	removed := set.NewTags()
	st := &mockStorageAccessor{
		unitStorageAttachments: func(u names.UnitTag) ([]params.StorageAttachmentId, error) {
			return nil, nil
		},
		remove: func(s names.StorageTag, u names.UnitTag) error {
			c.Assert(u, gc.Equals, unitTag)
			removed.Add(s)
			return nil
		},
	}

	att, err := storage.NewAttachments(st, unitTag, stateDir, abort)
	c.Assert(err, jc.ErrorIsNil)
	r := storage.NewResolver(att)

	// First make sure we create a storage-attached hook operation for
	// data/0. We do this to show that until the hook is *committed*,
	// we will still short-circuit removal.
	localState := resolver.LocalState{State: operation.State{
		Kind: operation.Continue,
	}}
	_, err = r.NextOp(localState, remotestate.Snapshot{
		Life: params.Alive,
		Storage: map[names.StorageTag]remotestate.StorageSnapshot{
			storageTag0: {
				Life:     params.Alive,
				Kind:     params.StorageKindBlock,
				Location: "/dev/sdb",
				Attached: true,
			},
		},
	}, &mockOperations{})
	c.Assert(err, jc.ErrorIsNil)

	for _, storageTag := range []names.StorageTag{storageTag0, storageTag1} {
		_, err = r.NextOp(localState, remotestate.Snapshot{
			Life: params.Alive,
			Storage: map[names.StorageTag]remotestate.StorageSnapshot{
				storageTag: {Life: params.Dying},
			},
		}, nil)
		c.Assert(err, gc.Equals, resolver.ErrNoOperation)
	}
	c.Assert(removed.SortedValues(), jc.DeepEquals, []names.Tag{
		storageTag0, storageTag1,
	})
}

func (s *attachmentsSuite) TestAttachmentsStorage(c *gc.C) {
	stateDir := c.MkDir()
	unitTag := names.NewUnitTag("mysql/0")
	abort := make(chan struct{})

	st := &mockStorageAccessor{
		unitStorageAttachments: func(u names.UnitTag) ([]params.StorageAttachmentId, error) {
			return nil, nil
		},
	}

	att, err := storage.NewAttachments(st, unitTag, stateDir, abort)
	c.Assert(err, jc.ErrorIsNil)
	r := storage.NewResolver(att)

	storageTag := names.NewStorageTag("data/0")
	_, err = att.Storage(storageTag)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	assertStorageTags(c, att)

	// Inform the resolver of an attachment.
	localState := resolver.LocalState{State: operation.State{
		Kind: operation.Continue,
	}}
	op, err := r.NextOp(localState, remotestate.Snapshot{
		Life: params.Alive,
		Storage: map[names.StorageTag]remotestate.StorageSnapshot{
			storageTag: {
				Kind:     params.StorageKindBlock,
				Life:     params.Alive,
				Location: "/dev/sdb",
				Attached: true,
			},
		},
	}, &mockOperations{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(op.String(), gc.Equals, "run hook storage-attached")
	assertStorageTags(c, att, storageTag)

	ctx, err := att.Storage(storageTag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ctx, gc.NotNil)
	c.Assert(ctx.Tag(), gc.Equals, storageTag)
	c.Assert(ctx.Kind(), gc.Equals, corestorage.StorageKindBlock)
	c.Assert(ctx.Location(), gc.Equals, "/dev/sdb")
}

func (s *attachmentsSuite) TestAttachmentsCommitHook(c *gc.C) {
	stateDir := c.MkDir()
	unitTag := names.NewUnitTag("mysql/0")
	abort := make(chan struct{})

	var removed bool
	storageTag := names.NewStorageTag("data/0")
	st := &mockStorageAccessor{
		unitStorageAttachments: func(u names.UnitTag) ([]params.StorageAttachmentId, error) {
			return nil, nil
		},
		remove: func(s names.StorageTag, u names.UnitTag) error {
			removed = true
			c.Assert(s, gc.Equals, storageTag)
			return nil
		},
	}

	att, err := storage.NewAttachments(st, unitTag, stateDir, abort)
	c.Assert(err, jc.ErrorIsNil)
	r := storage.NewResolver(att)

	// Inform the resolver of an attachment.
	localState := resolver.LocalState{State: operation.State{
		Kind: operation.Continue,
	}}
	_, err = r.NextOp(localState, remotestate.Snapshot{
		Life: params.Alive,
		Storage: map[names.StorageTag]remotestate.StorageSnapshot{
			storageTag: {
				Kind:     params.StorageKindBlock,
				Life:     params.Alive,
				Location: "/dev/sdb",
				Attached: true,
			},
		},
	}, &mockOperations{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(att.Pending(), gc.Equals, 1)

	// No file exists until storage-attached is committed.
	stateFile := filepath.Join(stateDir, "data-0")
	c.Assert(stateFile, jc.DoesNotExist)

	err = att.CommitHook(hook.Info{
		Kind:      hooks.StorageAttached,
		StorageId: storageTag.Id(),
	})
	c.Assert(err, jc.ErrorIsNil)
	data, err := ioutil.ReadFile(stateFile)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(data), gc.Equals, "attached: true\n")
	c.Assert(att.Pending(), gc.Equals, 0)

	c.Assert(removed, jc.IsFalse)
	err = att.CommitHook(hook.Info{
		Kind:      hooks.StorageDetaching,
		StorageId: storageTag.Id(),
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(stateFile, jc.DoesNotExist)
	c.Assert(removed, jc.IsTrue)
}

func (s *attachmentsSuite) TestAttachmentsSetDying(c *gc.C) {
	stateDir := c.MkDir()
	unitTag := names.NewUnitTag("mysql/0")
	abort := make(chan struct{})

	storageTag := names.NewStorageTag("data/0")
	var destroyed, removed bool
	st := &mockStorageAccessor{
		unitStorageAttachments: func(u names.UnitTag) ([]params.StorageAttachmentId, error) {
			c.Assert(u, gc.Equals, unitTag)
			return []params.StorageAttachmentId{{
				StorageTag: storageTag.String(),
				UnitTag:    unitTag.String(),
			}}, nil
		},
		storageAttachment: func(s names.StorageTag, u names.UnitTag) (params.StorageAttachment, error) {
			c.Assert(u, gc.Equals, unitTag)
			c.Assert(s, gc.Equals, storageTag)
			return params.StorageAttachment{}, &params.Error{
				Message: "not provisioned",
				Code:    params.CodeNotProvisioned,
			}
		},
		destroyUnitStorageAttachments: func(u names.UnitTag) error {
			c.Assert(u, gc.Equals, unitTag)
			destroyed = true
			return nil
		},
		remove: func(s names.StorageTag, u names.UnitTag) error {
			c.Assert(removed, jc.IsFalse)
			c.Assert(s, gc.Equals, storageTag)
			c.Assert(u, gc.Equals, unitTag)
			removed = true
			return nil
		},
	}

	att, err := storage.NewAttachments(st, unitTag, stateDir, abort)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(att.Pending(), gc.Equals, 1)
	r := storage.NewResolver(att)

	// Inform the resolver that the unit is Dying. The storage is still
	// Alive, and is now provisioned, but will be destroyed and removed
	// by the resolver.
	localState := resolver.LocalState{State: operation.State{
		Kind: operation.Continue,
	}}
	_, err = r.NextOp(localState, remotestate.Snapshot{
		Life: params.Dying,
		Storage: map[names.StorageTag]remotestate.StorageSnapshot{
			storageTag: {
				Kind:     params.StorageKindBlock,
				Life:     params.Alive,
				Location: "/dev/sdb",
				Attached: true,
			},
		},
	}, &mockOperations{})
	c.Assert(err, gc.Equals, resolver.ErrNoOperation)
	c.Assert(destroyed, jc.IsTrue)
	c.Assert(att.Pending(), gc.Equals, 0)
	c.Assert(removed, jc.IsTrue)
}

func (s *attachmentsSuite) TestAttachmentsWaitPending(c *gc.C) {
	stateDir := c.MkDir()
	unitTag := names.NewUnitTag("mysql/0")
	abort := make(chan struct{})

	storageTag := names.NewStorageTag("data/0")
	st := &mockStorageAccessor{
		unitStorageAttachments: func(u names.UnitTag) ([]params.StorageAttachmentId, error) {
			return nil, nil
		},
	}

	att, err := storage.NewAttachments(st, unitTag, stateDir, abort)
	c.Assert(err, jc.ErrorIsNil)
	r := storage.NewResolver(att)

	nextOp := func(installed bool) error {
		localState := resolver.LocalState{State: operation.State{
			Installed: installed,
			Kind:      operation.Continue,
		}}
		_, err := r.NextOp(localState, remotestate.Snapshot{
			Life: params.Alive,
			Storage: map[names.StorageTag]remotestate.StorageSnapshot{
				storageTag: {
					Life:     params.Alive,
					Attached: false,
				},
			},
		}, &mockOperations{})
		return err
	}

	// Inform the resolver of a new, unprovisioned storage attachment.
	// Before install, we should wait for its completion; after install,
	// we should not.
	err = nextOp(false /* workload not installed */)
	c.Assert(att.Pending(), gc.Equals, 1)
	c.Assert(err, gc.Equals, resolver.ErrWaiting)

	err = nextOp(true /* workload installed */)
	c.Assert(err, gc.Equals, resolver.ErrNoOperation)
}
