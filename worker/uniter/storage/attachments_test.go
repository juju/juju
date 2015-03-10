// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"path/filepath"
	"time"

	"launchpad.net/tomb"

	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v4/hooks"

	"github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/storage"
)

type attachmentsSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&attachmentsSuite{})

func (s *attachmentsSuite) TestNewAttachments(c *gc.C) {
	stateDir := filepath.Join(c.MkDir(), "nonexistent")
	unitTag := names.NewUnitTag("mysql/0")
	abort := make(chan struct{})
	st := &mockStorageAccessor{
		unitStorageAttachments: func(u names.UnitTag) ([]params.StorageAttachment, error) {
			c.Assert(u, gc.Equals, unitTag)
			return nil, nil
		},
	}

	att, err := storage.NewAttachments(st, unitTag, stateDir, abort)
	c.Assert(err, jc.ErrorIsNil)
	defer func() {
		err := att.Stop()
		c.Assert(err, jc.ErrorIsNil)
	}()
	// state dir should have been created.
	c.Assert(stateDir, jc.IsDirectory)
}

func (s *attachmentsSuite) TestNewAttachmentsInit(c *gc.C) {
	stateDir := c.MkDir()
	unitTag := names.NewUnitTag("mysql/0")
	abort := make(chan struct{})

	// Simulate remote state returning a single Alive storage attachment.
	attachments := []params.StorageAttachment{{
		StorageTag: "storage-data-0",
		Life:       params.Alive,
	}}
	st := &mockStorageAccessor{
		unitStorageAttachments: func(u names.UnitTag) ([]params.StorageAttachment, error) {
			c.Assert(u, gc.Equals, unitTag)
			return attachments, nil
		},
		watchStorageAttachment: func(s names.StorageTag, u names.UnitTag) (watcher.NotifyWatcher, error) {
			return newMockNotifyWatcher(), nil
		},
	}

	storageTag := names.NewStorageTag("data/0")
	withAttachments := func(f func(*storage.Attachments)) {
		att, err := storage.NewAttachments(st, unitTag, stateDir, abort)
		c.Assert(err, jc.ErrorIsNil)
		defer func() {
			err := att.Stop()
			c.Assert(err, jc.ErrorIsNil)
		}()
		f(att)
	}

	// No state files, so no storagers will be started.
	var called int
	withAttachments(func(att *storage.Attachments) {
		called++
		err := att.ValidateHook(hook.Info{
			Kind:      hooks.StorageAttached,
			StorageId: storageTag.Id(),
		})
		c.Assert(err, gc.ErrorMatches, `unknown storage "data/0"`)
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
		called++
		err := att.ValidateHook(hook.Info{
			Kind:      hooks.StorageDetached,
			StorageId: storageTag.Id(),
		})
		c.Assert(err, jc.ErrorIsNil)
		err = att.ValidateHook(hook.Info{
			Kind:      hooks.StorageAttached,
			StorageId: "data/1",
		})
		c.Assert(err, gc.ErrorMatches, `unknown storage "data/1"`)
	})
	c.Assert(called, gc.Equals, 2)
	c.Assert(filepath.Join(stateDir, "data-0"), jc.IsNonEmptyFile)
	c.Assert(filepath.Join(stateDir, "data-1"), jc.DoesNotExist)
}

func (s *attachmentsSuite) TestAttachmentsUpdate(c *gc.C) {
	stateDir := c.MkDir()
	unitTag := names.NewUnitTag("mysql/0")
	abort := make(chan struct{})

	storageTag0 := names.NewStorageTag("data/0")
	storageTag1 := names.NewStorageTag("data/1")
	attachmentsByTag := map[names.StorageTag]*params.StorageAttachment{
		storageTag0: {
			StorageTag: storageTag0.String(),
			UnitTag:    unitTag.String(),
			Life:       params.Alive,
			Kind:       params.StorageKindBlock,
			Location:   "/dev/sdb",
		},
		storageTag1: {
			StorageTag: storageTag1.String(),
			UnitTag:    unitTag.String(),
			Life:       params.Dying,
			Kind:       params.StorageKindBlock,
			Location:   "/dev/sdb",
		},
	}

	st := &mockStorageAccessor{
		unitStorageAttachments: func(u names.UnitTag) ([]params.StorageAttachment, error) {
			c.Assert(u, gc.Equals, unitTag)
			return nil, nil
		},
		watchStorageAttachment: func(s names.StorageTag, u names.UnitTag) (watcher.NotifyWatcher, error) {
			w := newMockNotifyWatcher()
			w.changes <- struct{}{}
			return w, nil
		},
		storageAttachment: func(s names.StorageTag, u names.UnitTag) (params.StorageAttachment, error) {
			att, ok := attachmentsByTag[s]
			c.Assert(ok, jc.IsTrue)
			return *att, nil
		},
	}

	att, err := storage.NewAttachments(st, unitTag, stateDir, abort)
	c.Assert(err, jc.ErrorIsNil)
	defer func() {
		err := att.Stop()
		c.Assert(err, jc.ErrorIsNil)
	}()

	assertNoHooks := func() {
		select {
		case <-att.Hooks():
			c.Fatal("unexpected hook")
		case <-time.After(testing.ShortWait):
		}
	}
	waitOneHook := func() hook.Info {
		var hi hook.Info
		var ok bool
		select {
		case hi, ok = <-att.Hooks():
			c.Assert(ok, jc.IsTrue)
		case <-time.After(testing.LongWait):
			c.Fatal("timed out waiting for hook")
		}
		assertNoHooks()
		return hi
	}

	// data/0 is initially unattached and untracked, so
	// updating with Alive will cause a storager to be
	// started and a storage-attached event to be emitted.
	err = att.UpdateStorage([]names.StorageTag{storageTag0})
	c.Assert(err, jc.ErrorIsNil)
	hi := waitOneHook()
	c.Assert(hi, gc.Equals, hook.Info{
		Kind:      hooks.StorageAttached,
		StorageId: storageTag0.Id(),
	})

	// data/0 is initially unattached and untracked, so
	// updating with Dying will not cause a storager to
	// be started.
	err = att.UpdateStorage([]names.StorageTag{storageTag1})
	c.Assert(err, jc.ErrorIsNil)
	assertNoHooks()

	// Cause an Alive hook to be queued, but don't consume it;
	// then update to Dying, and ensure no hooks are generated.
	// Additionally, the storager should be stopped and no
	// longer tracked.
	attachmentsByTag[storageTag1].Life = params.Alive
	err = att.UpdateStorage([]names.StorageTag{storageTag1})
	c.Assert(err, jc.ErrorIsNil)
	err = att.ValidateHook(hook.Info{
		Kind:      hooks.StorageAttached,
		StorageId: storageTag1.Id(),
	})
	c.Assert(err, jc.ErrorIsNil)

	attachmentsByTag[storageTag1].Life = params.Dying
	err = att.UpdateStorage([]names.StorageTag{storageTag1})
	c.Assert(err, jc.ErrorIsNil)
	assertNoHooks()
	err = att.ValidateHook(hook.Info{
		Kind:      hooks.StorageAttached,
		StorageId: storageTag1.Id(),
	})
	c.Assert(err, gc.ErrorMatches, `unknown storage "data/1"`)
}

type mockStorageAccessor struct {
	watchStorageAttachment func(names.StorageTag, names.UnitTag) (watcher.NotifyWatcher, error)
	storageAttachment      func(names.StorageTag, names.UnitTag) (params.StorageAttachment, error)
	unitStorageAttachments func(names.UnitTag) ([]params.StorageAttachment, error)
}

func (m *mockStorageAccessor) WatchStorageAttachment(s names.StorageTag, u names.UnitTag) (watcher.NotifyWatcher, error) {
	return m.watchStorageAttachment(s, u)
}

func (m *mockStorageAccessor) StorageAttachment(s names.StorageTag, u names.UnitTag) (params.StorageAttachment, error) {
	return m.storageAttachment(s, u)
}

func (m *mockStorageAccessor) UnitStorageAttachments(u names.UnitTag) ([]params.StorageAttachment, error) {
	return m.unitStorageAttachments(u)
}

type mockNotifyWatcher struct {
	tomb    tomb.Tomb
	changes chan struct{}
}

func newMockNotifyWatcher() *mockNotifyWatcher {
	m := &mockNotifyWatcher{
		changes: make(chan struct{}, 1),
	}
	go func() {
		<-m.tomb.Dying()
		close(m.changes)
		m.tomb.Kill(tomb.ErrDying)
		m.tomb.Done()
	}()
	return m
}

func (m *mockNotifyWatcher) Changes() <-chan struct{} {
	return m.changes
}

func (m *mockNotifyWatcher) Stop() error {
	m.tomb.Kill(nil)
	return m.tomb.Wait()
}

func (m *mockNotifyWatcher) Err() error {
	return m.tomb.Err()
}
