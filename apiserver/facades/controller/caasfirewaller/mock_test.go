// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasfirewaller_test

import (
	"github.com/juju/names/v5"
	"github.com/juju/testing"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/apiserver/facades/controller/caasfirewaller"
	"github.com/juju/juju/core/config"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/state"
)

type mockState struct {
	testing.Stub
	application         mockApplication
	applicationsWatcher *watchertest.MockStringsWatcher
	appExposedWatcher   *watchertest.MockNotifyWatcher
}

func (st *mockState) WatchApplications() state.StringsWatcher {
	st.MethodCall(st, "WatchApplications")
	return st.applicationsWatcher
}

func (st *mockState) Application(name string) (caasfirewaller.Application, error) {
	st.MethodCall(st, "Application", name)
	if err := st.NextErr(); err != nil {
		return nil, err
	}
	return &st.application, nil
}

func (st *mockState) FindEntity(tag names.Tag) (state.Entity, error) {
	st.MethodCall(st, "FindEntity", tag)
	if err := st.NextErr(); err != nil {
		return nil, err
	}
	return &st.application, nil
}

func (st *mockState) Charm(curl string) (*state.Charm, error) {
	st.MethodCall(st, "Charm", curl)
	if err := st.NextErr(); err != nil {
		return nil, err
	}
	return nil, nil
}

func (st *mockState) Model() (*state.Model, error) {
	st.MethodCall(st, "Model")
	if err := st.NextErr(); err != nil {
		return nil, err
	}
	return nil, nil
}

type mockApplication struct {
	testing.Stub
	state.Entity // Pull in Tag method (which tests don't use)
	exposed      bool
	watcher      state.NotifyWatcher

	appPortRanges network.GroupedPortRanges
}

func (a *mockApplication) IsExposed() bool {
	a.MethodCall(a, "IsExposed")
	return a.exposed
}

func (a *mockApplication) ApplicationConfig() (config.ConfigAttributes, error) {
	a.MethodCall(a, "ApplicationConfig")
	return config.ConfigAttributes{"foo": "bar"}, a.NextErr()
}

func (a *mockApplication) Watch() state.NotifyWatcher {
	a.MethodCall(a, "Watch")
	return a.watcher
}

func (a *mockApplication) OpenedPortRanges() (network.GroupedPortRanges, error) {
	a.MethodCall(a, "OpenedPortRanges")
	return a.appPortRanges, nil
}

type mockWatcher struct {
	testing.Stub
	tomb.Tomb
}

func (w *mockWatcher) Kill() {
	w.MethodCall(w, "Kill")
	w.Tomb.Kill(nil)
}

func (w *mockWatcher) Stop() error {
	w.MethodCall(w, "Stop")
	if err := w.NextErr(); err != nil {
		return err
	}
	w.Tomb.Kill(nil)
	return w.Tomb.Wait()
}

func (w *mockWatcher) Err() error {
	w.MethodCall(w, "Err")
	return w.Tomb.Err()
}

type mockStringsWatcher struct {
	mockWatcher
	changes chan []string
}

func newMockStringsWatcher() *mockStringsWatcher {
	w := &mockStringsWatcher{changes: make(chan []string, 1)}
	w.Tomb.Go(func() error {
		<-w.Tomb.Dying()
		return nil
	})
	return w
}

func (w *mockStringsWatcher) Changes() <-chan []string {
	w.MethodCall(w, "Changes")
	return w.changes
}
