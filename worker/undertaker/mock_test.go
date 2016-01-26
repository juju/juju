// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package undertaker_test

import (
	"sync"
	"time"

	"github.com/juju/errors"

	"github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
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
	cfg         *config.Config
}

func (m *mockClient) mockCall(call string) {
	m.calls <- call
}

func (m *mockClient) ProcessDyingModel() error {
	defer m.mockCall("ProcessDyingModel")
	if m.mockEnviron.HasMachinesAndServices {
		return errors.Errorf("found documents for model with uuid %s: 1 cleanups doc, 1 constraints doc, 1 modelusers doc, 1 leases doc, 1 settings doc", m.mockEnviron.UUID)
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

func (m *mockClient) ModelInfo() (params.UndertakerModelInfoResult, error) {
	defer m.mockCall("ModelInfo")
	result := params.UndertakerEnvironInfo{
		Life:        params.Life(m.mockEnviron.Life.String()),
		UUID:        m.mockEnviron.UUID,
		Name:        "dummy",
		GlobalName:  "bob/dummy",
		IsSystem:    m.mockEnviron.IsSystem,
		TimeOfDeath: m.mockEnviron.TimeOfDeath,
	}
	return params.UndertakerModelInfoResult{Result: result}, nil
}

func (m *mockClient) EnvironConfig() (*config.Config, error) {
	return m.cfg, nil
}

func (m *mockClient) WatchEnvironResources() (watcher.NotifyWatcher, error) {
	return m.watcher, nil
}

type mockEnvironResourceWatcher struct {
	events chan struct{}
	err    error
}

func (w *mockEnvironResourceWatcher) Changes() <-chan struct{} {
	return w.events
}

func (w *mockEnvironResourceWatcher) Err() error {
	return w.err
}

func (w *mockEnvironResourceWatcher) Stop() error {
	close(w.events)
	return nil
}
