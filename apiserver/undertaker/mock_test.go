// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package undertaker_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/undertaker"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
)

// mockState implements State interface and allows inspection of called
// methods.
type mockState struct {
	env      *mockModel
	removed  bool
	isSystem bool
	machines []undertaker.Machine
	services []undertaker.Service
}

var _ undertaker.State = (*mockState)(nil)

func newMockState(envOwner names.UserTag, envName string, isSystem bool) *mockState {
	machine := &mockMachine{
		watcher: &mockWatcher{
			changes: make(chan struct{}, 1),
		},
	}
	service := &mockService{
		watcher: &mockWatcher{
			changes: make(chan struct{}, 1),
		},
	}

	env := mockModel{
		owner: envOwner,
		name:  envName,
		life:  state.Alive,
	}

	m := &mockState{
		env:      &env,
		isSystem: isSystem,
		machines: []undertaker.Machine{machine},
		services: []undertaker.Service{service},
	}
	return m
}

func (m *mockState) EnsureModelRemoved() error {
	if !m.removed {
		return errors.New("found documents for model")
	}
	return nil
}

func (m *mockState) RemoveAllModelDocs() error {
	if m.env.life != state.Dead {
		return errors.New("transaction aborted")
	}
	m.removed = true
	return nil
}

func (m *mockState) ProcessDyingModel() error {
	if m.env.life != state.Dying {
		return errors.New("model is not dying")
	}
	m.env.life = state.Dead
	return nil
}

func (m *mockState) AllMachines() ([]undertaker.Machine, error) {
	return m.machines, nil
}

func (m *mockState) AllServices() ([]undertaker.Service, error) {
	return m.services, nil
}

func (m *mockState) IsController() bool {
	return m.isSystem
}

func (m *mockState) Model() (undertaker.Model, error) {
	return m.env, nil
}

func (m *mockState) ModelConfig() (*config.Config, error) {
	return &config.Config{}, nil
}

// mockModel implements Model interface and allows inspection of called
// methods.
type mockModel struct {
	tod   time.Time
	owner names.UserTag
	life  state.Life
	name  string
	uuid  string
}

var _ undertaker.Model = (*mockModel)(nil)

func (m *mockModel) TimeOfDeath() time.Time {
	return m.tod
}

func (m *mockModel) Owner() names.UserTag {
	return m.owner
}

func (m *mockModel) Life() state.Life {
	return m.life
}

func (m *mockModel) Name() string {
	return m.name
}

func (m *mockModel) UUID() string {
	return m.uuid
}

func (m *mockModel) Destroy() error {
	m.life = state.Dying
	return nil
}

type mockMachine struct {
	watcher state.NotifyWatcher
	err     error
}

func (m *mockMachine) Watch() state.NotifyWatcher {
	return m.watcher
}

type mockService struct {
	watcher state.NotifyWatcher
	err     error
}

func (s *mockService) Watch() state.NotifyWatcher {
	return s.watcher
}

type mockWatcher struct {
	state.NotifyWatcher
	changes chan struct{}
}

func (w *mockWatcher) Changes() <-chan struct{} {
	return w.changes
}
