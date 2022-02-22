// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package externalcontrollerupdater_test

import (
	"github.com/juju/testing"
	tomb "gopkg.in/tomb.v2"

	"github.com/juju/juju/api/controller/crosscontroller"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/watcher"
)

type mockExternalControllerUpdaterClient struct {
	testing.Stub
	watcher *mockStringsWatcher
	info    crossmodel.ControllerInfo
}

func (m *mockExternalControllerUpdaterClient) WatchExternalControllers() (watcher.StringsWatcher, error) {
	m.MethodCall(m, "WatchExternalControllers")
	return m.watcher, m.NextErr()
}

func (m *mockExternalControllerUpdaterClient) ExternalControllerInfo(controllerUUID string) (*crossmodel.ControllerInfo, error) {
	m.MethodCall(m, "ExternalControllerInfo", controllerUUID)
	copied := m.info
	copied.Addrs = make([]string, len(m.info.Addrs))
	copy(copied.Addrs, m.info.Addrs)
	return &copied, m.NextErr()
}

func (m *mockExternalControllerUpdaterClient) SetExternalControllerInfo(info crossmodel.ControllerInfo) error {
	m.MethodCall(m, "SetExternalControllerInfo", info)
	return m.NextErr()
}

type mockExternalControllerWatcherClient struct {
	testing.Stub
	watcher *mockNotifyWatcher
	info    crosscontroller.ControllerInfo
}

func (m *mockExternalControllerWatcherClient) Close() error {
	m.MethodCall(m, "Close")
	return m.NextErr()
}

func (m *mockExternalControllerWatcherClient) WatchControllerInfo() (watcher.NotifyWatcher, error) {
	m.MethodCall(m, "WatchControllerInfo")
	return m.watcher, m.NextErr()
}

func (m *mockExternalControllerWatcherClient) ControllerInfo() (*crosscontroller.ControllerInfo, error) {
	m.MethodCall(m, "ControllerInfo")
	copied := m.info
	copied.Addrs = make([]string, len(m.info.Addrs))
	copy(copied.Addrs, m.info.Addrs)
	return &copied, m.NextErr()
}

type mockStringsWatcher struct {
	mockWatcher
	changes chan []string
}

func newMockStringsWatcher() *mockStringsWatcher {
	w := &mockStringsWatcher{changes: make(chan []string, 1)}
	w.tomb.Go(func() error {
		w.loop()
		return nil
	})
	return w
}

func (w *mockStringsWatcher) Changes() watcher.StringsChannel {
	return w.changes
}

type mockNotifyWatcher struct {
	mockWatcher
	changes chan struct{}
}

func newMockNotifyWatcher() *mockNotifyWatcher {
	w := &mockNotifyWatcher{changes: make(chan struct{}, 1)}
	w.tomb.Go(func() error {
		w.loop()
		return nil
	})
	return w
}

func (w *mockNotifyWatcher) Changes() watcher.NotifyChannel {
	return w.changes
}

type mockWatcher struct {
	tomb tomb.Tomb
}

func (w *mockWatcher) loop() {
	<-w.tomb.Dying()
}

func (w *mockWatcher) Stop() error {
	w.Kill()
	return w.Wait()
}

func (w *mockWatcher) Wait() error {
	return w.tomb.Wait()
}

func (w *mockWatcher) Kill() {
	w.tomb.Kill(nil)
}

func (w *mockWatcher) Err() error {
	return w.tomb.Err()
}
