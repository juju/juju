// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package undertaker_test

import (
	"sync"
	"time"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/watcher"
)

type clientEnviron struct {
	Life                   state.Life
	TimeOfDeath            *time.Time
	UUID                   string
	IsSystem               bool
	HasMachinesAndServices bool
	Removed                bool
}

type mockClient struct {
	calls       chan string
	lock        sync.RWMutex
	mockEnviron clientEnviron
	watcher     watcher.NotifyWatcher
}

func (m *mockClient) mockCall(call string) {
	m.calls <- call
}

func (m *mockClient) ProcessDyingEnviron() error {
	defer m.mockCall("ProcessDyingEnviron")
	if m.mockEnviron.HasMachinesAndServices {
		return errors.Errorf("found documents for environment with uuid %s: 1 cleanups doc, 1 constraints doc, 1 envusers doc, 1 leases doc, 1 settings doc", m.mockEnviron.UUID)
	}
	m.mockEnviron.Life = state.Dead
	t := time.Now()
	m.mockEnviron.TimeOfDeath = &t

	return nil
}

func (m *mockClient) RemoveEnviron() error {
	defer m.mockCall("RemoveEnviron")
	m.mockEnviron.Removed = true
	return nil
}

func (m *mockClient) EnvironInfo() (params.UndertakerEnvironInfoResult, error) {
	defer m.mockCall("EnvironInfo")
	result := params.UndertakerEnvironInfo{
		Life:        params.Life(m.mockEnviron.Life.String()),
		UUID:        m.mockEnviron.UUID,
		Name:        "dummy",
		GlobalName:  "bob/dummy",
		IsSystem:    m.mockEnviron.IsSystem,
		TimeOfDeath: m.mockEnviron.TimeOfDeath,
	}
	return params.UndertakerEnvironInfoResult{Result: result}, nil
}

func (m *mockClient) WatchEnvironResources() (watcher.NotifyWatcher, error) {
	return m.watcher, nil
}

type mockEnvironResourceWatcher struct {
	events chan struct{}
	err    error
}

func (w *mockEnvironResourceWatcher) Changes() watcher.NotifyChannel {
	return w.events
}

func (w *mockEnvironResourceWatcher) Kill() {
	close(w.events)
}

func (w *mockEnvironResourceWatcher) Wait() error {
	return w.err
}
