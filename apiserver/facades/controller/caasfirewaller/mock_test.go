// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasfirewaller_test

import (
	"github.com/juju/charm/v8"
	"github.com/juju/names/v4"
	"github.com/juju/testing"

	"github.com/juju/juju/apiserver/common"
	charmscommon "github.com/juju/juju/apiserver/common/charms"
	"github.com/juju/juju/apiserver/facades/controller/caasfirewaller"
	"github.com/juju/juju/core/application"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
)

type mockState struct {
	testing.Stub
	application         mockApplication
	applicationsWatcher *statetesting.MockStringsWatcher
	openPortsWatcher    *statetesting.MockStringsWatcher
	appExposedWatcher   *statetesting.MockNotifyWatcher
}

func (st *mockState) WatchApplications() state.StringsWatcher {
	st.MethodCall(st, "WatchApplications")
	return st.applicationsWatcher
}

func (st *mockState) WatchOpenedPorts() state.StringsWatcher {
	st.MethodCall(st, "WatchOpenedPorts")
	return st.openPortsWatcher
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

func (st *mockState) Charm(curl *charm.URL) (*state.Charm, error) {
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
	life         state.Life
	exposed      bool
	watcher      state.NotifyWatcher

	charm mockAppWatcherCharm
}

func (a *mockApplication) Life() state.Life {
	a.MethodCall(a, "Life")
	return a.life
}

func (a *mockApplication) IsExposed() bool {
	a.MethodCall(a, "IsExposed")
	return a.exposed
}

func (a *mockApplication) ApplicationConfig() (application.ConfigAttributes, error) {
	a.MethodCall(a, "ApplicationConfig")
	return application.ConfigAttributes{"foo": "bar"}, a.NextErr()
}

func (a *mockApplication) Watch() state.NotifyWatcher {
	a.MethodCall(a, "Watch")
	return a.watcher
}

func (a *mockApplication) Charm() (charmscommon.Charm, bool, error) {
	a.MethodCall(a, "Charm")
	return &a.charm, false, nil
}

type mockAppWatcherState struct {
	testing.Stub
	app     *mockAppWatcherApplication
	watcher *statetesting.MockStringsWatcher
}

func (s *mockAppWatcherState) WatchApplications() state.StringsWatcher {
	s.MethodCall(s, "WatchApplications")
	return s.watcher
}

func (s *mockAppWatcherState) Application(name string) (common.AppWatcherApplication, error) {
	s.MethodCall(s, "Application", name)
	return s.app, nil
}

type mockAppWatcherApplication struct {
	testing.Stub
	force bool
	charm mockAppWatcherCharm
}

func (s *mockAppWatcherApplication) Charm() (charm.CharmMeta, bool, error) {
	s.MethodCall(s, "Charm")
	err := s.NextErr()
	if err != nil {
		return nil, false, err
	}
	return &s.charm, s.force, nil
}

type mockAppWatcherCharm struct {
	testing.Stub
	charmscommon.Charm // Override only the methods the tests use
	meta               *charm.Meta
	manifest           *charm.Manifest
	url                *charm.URL
}

func (s *mockAppWatcherCharm) Meta() *charm.Meta {
	s.MethodCall(s, "Meta")
	return s.meta
}

func (s *mockAppWatcherCharm) Manifest() *charm.Manifest {
	s.MethodCall(s, "Manifest")
	return s.manifest
}

func (s *mockAppWatcherCharm) URL() *charm.URL {
	s.MethodCall(s, "URL")
	return s.url
}

type mockCommonStateShim struct {
	*mockState
}

func (s *mockCommonStateShim) Model() (charmscommon.Model, error) {
	return s.mockState.Model()
}

func (s *mockCommonStateShim) Charm(curl *charm.URL) (charmscommon.Charm, error) {
	return s.mockState.Charm(curl)
}

func (s *mockCommonStateShim) Application(id string) (charmscommon.Application, error) {
	app, err := s.mockState.Application(id)
	if err != nil {
		return nil, err
	}
	return &mockCommonApplicationShim{app}, nil
}

type mockCommonApplicationShim struct {
	caasfirewaller.Application
}

func (a *mockCommonApplicationShim) Charm() (st charmscommon.Charm, force bool, err error) {
	return a.Application.Charm()
}
