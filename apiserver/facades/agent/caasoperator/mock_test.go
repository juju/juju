// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperator_test

import (
	"github.com/juju/testing"
	"gopkg.in/juju/charm.v6"

	"github.com/juju/juju/apiserver/facades/agent/caasoperator"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/status"
)

type mockState struct {
	testing.Stub
	app mockApplication
}

func newMockState() *mockState {
	ch := make(chan struct{}, 1)
	ch <- struct{}{}
	settingsWatcher := statetesting.NewMockNotifyWatcher(ch)
	return &mockState{
		app: mockApplication{
			charm: mockCharm{
				url:    charm.MustParseURL("cs:gitlab-1"),
				sha256: "fake-sha256",
			},
			settings:        charm.Settings{"k": 123},
			settingsWatcher: settingsWatcher,
		},
	}
}

func (st *mockState) Application(id string) (caasoperator.Application, error) {
	st.MethodCall(st, "Application", id)
	if err := st.NextErr(); err != nil {
		return nil, err
	}
	return &st.app, nil
}

type mockApplication struct {
	testing.Stub
	charm           mockCharm
	forceUpgrade    bool
	settings        charm.Settings
	settingsWatcher *statetesting.MockNotifyWatcher
}

func (app *mockApplication) SetStatus(info status.StatusInfo) error {
	app.MethodCall(app, "SetStatus", info)
	return app.NextErr()
}

func (app *mockApplication) Charm() (caasoperator.Charm, bool, error) {
	app.MethodCall(app, "Charm")
	if err := app.NextErr(); err != nil {
		return nil, false, err
	}
	return &app.charm, app.forceUpgrade, nil
}

func (app *mockApplication) ConfigSettings() (charm.Settings, error) {
	app.MethodCall(app, "ConfigSettings")
	if err := app.NextErr(); err != nil {
		return nil, err
	}
	return app.settings, nil
}

func (app *mockApplication) WatchConfigSettings() (state.NotifyWatcher, error) {
	app.MethodCall(app, "WatchConfigSettings")
	if err := app.NextErr(); err != nil {
		return nil, err
	}
	return app.settingsWatcher, nil
}

type mockCharm struct {
	url    *charm.URL
	sha256 string
}

func (ch *mockCharm) URL() *charm.URL {
	return ch.url
}

func (ch *mockCharm) BundleSha256() string {
	return ch.sha256
}
