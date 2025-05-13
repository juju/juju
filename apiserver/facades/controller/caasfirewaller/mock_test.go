// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasfirewaller_test

import (
	"github.com/juju/names/v6"

	"github.com/juju/juju/apiserver/facades/controller/caasfirewaller"
	"github.com/juju/juju/core/config"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/state"
)

type mockState struct {
	testhelpers.Stub
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

func (st *mockState) Model() (*state.Model, error) {
	st.MethodCall(st, "Model")
	if err := st.NextErr(); err != nil {
		return nil, err
	}
	return nil, nil
}

type mockApplication struct {
	testhelpers.Stub
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
