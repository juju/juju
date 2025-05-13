// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasunitprovisioner_test

import (
	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/apiserver/facades/controller/caasunitprovisioner"
	coreconfig "github.com/juju/juju/core/config"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/state"
)

type mockState struct {
	testhelpers.Stub
	application mockApplication
}

func (st *mockState) Application(name string) (caasunitprovisioner.Application, error) {
	st.MethodCall(st, "Application", name)
	if name != st.application.tag.Id() {
		return nil, errors.NotFoundf("application %v", name)
	}
	return &st.application, st.NextErr()
}

type mockApplication struct {
	testhelpers.Stub
	life            state.Life
	settingsWatcher *watchertest.MockStringsWatcher

	tag   names.Tag
	scale int
}

func (a *mockApplication) Tag() names.Tag {
	return a.tag
}

func (a *mockApplication) Name() string {
	a.MethodCall(a, "Name")
	return a.tag.Id()
}

func (a *mockApplication) Life() state.Life {
	a.MethodCall(a, "Life")
	return a.life
}

func (a *mockApplication) WatchConfigSettingsHash() state.StringsWatcher {
	a.MethodCall(a, "WatchConfigSettingsHash")
	return a.settingsWatcher
}

func (a *mockApplication) ApplicationConfig() (coreconfig.ConfigAttributes, error) {
	a.MethodCall(a, "ApplicationConfig")
	return coreconfig.ConfigAttributes{"foo": "bar"}, a.NextErr()
}
