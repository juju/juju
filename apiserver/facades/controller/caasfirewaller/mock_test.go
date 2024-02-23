// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasfirewaller_test

import (
	"github.com/juju/juju/charm"
	"github.com/juju/names/v5"
	"github.com/juju/testing"

	charmscommon "github.com/juju/juju/apiserver/common/charms"
	"github.com/juju/juju/apiserver/facades/controller/caasfirewaller"
	"github.com/juju/juju/core/config"
	"github.com/juju/juju/core/network"
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
	life         state.Life
	exposed      bool
	watcher      state.NotifyWatcher

	charm         mockCharm
	appPortRanges network.GroupedPortRanges
}

func (a *mockApplication) Life() state.Life {
	a.MethodCall(a, "Life")
	return a.life
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

func (a *mockApplication) Charm() (charmscommon.Charm, bool, error) {
	a.MethodCall(a, "Charm")
	return &a.charm, false, nil
}

type mockCharm struct {
	testing.Stub
	charmscommon.Charm // Override only the methods the tests use
	meta               *charm.Meta
	manifest           *charm.Manifest
	url                string
}

func (s *mockCharm) Meta() *charm.Meta {
	s.MethodCall(s, "Meta")
	return s.meta
}

func (s *mockCharm) Manifest() *charm.Manifest {
	s.MethodCall(s, "Manifest")
	return s.manifest
}

func (s *mockCharm) URL() string {
	s.MethodCall(s, "URL")
	return s.url
}

type mockCommonStateShim struct {
	*mockState
}

func (s *mockCommonStateShim) Model() (charmscommon.Model, error) {
	return s.mockState.Model()
}

func (s *mockCommonStateShim) Charm(curl string) (charmscommon.Charm, error) {
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
