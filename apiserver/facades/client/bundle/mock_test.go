// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bundle_test

import (
	"github.com/juju/testing"
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/facades/client/bundle"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
)

type mockState struct {
	testing.Stub
	bundle.Backend
	entities map[string]state.Entity
	app      mockApplication
	unit     mockUnit
}

func newMockState() *mockState {
	unitsChanges := make(chan []string, 1)
	appChanges := make(chan struct{}, 1)
	st := &mockState{
		entities: make(map[string]state.Entity),
		app: mockApplication{
			life: state.Alive,
			charm: mockCharm{
				url:    charm.MustParseURL("cs:gitlab-1"),
				sha256: "fake-sha256",
			},
			unitsChanges: unitsChanges,
			appChanges:   appChanges,
			unitsWatcher: statetesting.NewMockStringsWatcher(unitsChanges),
			watcher:      statetesting.NewMockNotifyWatcher(appChanges),
		},
		unit: mockUnit{
			life: state.Dying,
		},
	}
	st.entities[st.app.Tag().String()] = &st.app
	st.entities[st.unit.Tag().String()] = &st.unit
	return st
}

type mockApplication struct {
	testing.Stub
	life         state.Life
	charm        mockCharm
	forceUpgrade bool
	unitsChanges chan []string
	unitsWatcher *statetesting.MockStringsWatcher
	appChanges   chan struct{}
	watcher      *statetesting.MockNotifyWatcher
}

func (*mockApplication) Tag() names.Tag {
	return names.NewApplicationTag("gitlab")
}

type mockUnit struct {
	testing.Stub
	life state.Life
}

func (*mockUnit) Tag() names.Tag {
	return names.NewUnitTag("gitlab/0")
}

type mockCharm struct {
	url    *charm.URL
	sha256 string
}
