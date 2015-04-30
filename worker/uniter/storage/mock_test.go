// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"time"

	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"launchpad.net/tomb"

	"github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/worker/uniter/hook"
)

type mockStorageAccessor struct {
	watchStorageAttachment func(names.StorageTag, names.UnitTag) (watcher.NotifyWatcher, error)
	storageAttachment      func(names.StorageTag, names.UnitTag) (params.StorageAttachment, error)
	unitStorageAttachments func(names.UnitTag) ([]params.StorageAttachment, error)
	remove                 func(names.StorageTag, names.UnitTag) error
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

func (m *mockStorageAccessor) RemoveStorageAttachment(s names.StorageTag, u names.UnitTag) error {
	return m.remove(s, u)
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

func assertNoHooks(c *gc.C, hooks <-chan hook.Info) {
	select {
	case <-hooks:
		c.Fatal("unexpected hook")
	case <-time.After(testing.ShortWait):
	}
}

func waitOneHook(c *gc.C, hooks <-chan hook.Info) hook.Info {
	var hi hook.Info
	var ok bool
	select {
	case hi, ok = <-hooks:
		c.Assert(ok, jc.IsTrue)
	case <-time.After(testing.LongWait):
		c.Fatal("timed out waiting for hook")
	}
	assertNoHooks(c, hooks)
	return hi
}
