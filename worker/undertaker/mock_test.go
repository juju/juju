// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package undertaker_test

import (
	"sync"
	"time"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
	"github.com/juju/juju/watcher"
)

type clientModel struct {
	Life                   state.Life
	TimeOfDeath            *time.Time
	UUID                   string
	IsSystem               bool
	HasMachinesAndServices bool
	Removed                bool
}

type mockClient struct {
	calls     chan string
	lock      sync.RWMutex
	mockModel clientModel
	watcher   watcher.NotifyWatcher
	cfg       *config.Config
}

func (m *mockClient) mockCall(call string) {
	m.calls <- call
}

func (m *mockClient) ProcessDyingModel() error {
	defer m.mockCall("ProcessDyingModel")
	if m.mockModel.HasMachinesAndServices {
		return errors.Errorf("found documents for model with uuid %s: 1 cleanups doc, 1 constraints doc, 1 leases doc, 1 modelusers doc, 1 settings doc", m.mockModel.UUID)
	}
	m.mockModel.Life = state.Dead
	t := time.Now()
	m.mockModel.TimeOfDeath = &t

	return nil
}

func (m *mockClient) RemoveModel() error {
	defer m.mockCall("RemoveModel")
	m.mockModel.Removed = true
	return nil
}

func (m *mockClient) ModelInfo() (params.UndertakerModelInfoResult, error) {
	defer m.mockCall("ModelInfo")
	result := params.UndertakerModelInfo{
		Life:        params.Life(m.mockModel.Life.String()),
		UUID:        m.mockModel.UUID,
		Name:        "dummy",
		GlobalName:  "bob/dummy",
		IsSystem:    m.mockModel.IsSystem,
		TimeOfDeath: m.mockModel.TimeOfDeath,
	}
	return params.UndertakerModelInfoResult{Result: result}, nil
}

func (m *mockClient) ModelConfig() (*config.Config, error) {
	return m.cfg, nil
}

func (m *mockClient) WatchModelResources() (watcher.NotifyWatcher, error) {
	return m.watcher, nil
}

type mockModelResourceWatcher struct {
	events    chan struct{}
	closeOnce sync.Once
	err       error
}

func (w *mockModelResourceWatcher) Changes() watcher.NotifyChannel {
	return w.events
}

func (w *mockModelResourceWatcher) Kill() {
	w.closeOnce.Do(func() { close(w.events) })
}

func (w *mockModelResourceWatcher) Wait() error {
	return w.err
}
