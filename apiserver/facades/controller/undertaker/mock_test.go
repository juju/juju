// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package undertaker_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/facades/controller/undertaker"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
)

// mockState implements State interface and allows inspection of called
// methods.
type mockState struct {
	model    *mockModel
	removed  bool
	isSystem bool

	watcher state.NotifyWatcher
}

var _ undertaker.State = (*mockState)(nil)

func newMockState(modelOwner names.UserTag, modelName string, isSystem bool) *mockState {
	model := mockModel{
		owner: modelOwner,
		name:  modelName,
		uuid:  "9d3d3b19-2b0c-4a3f-acde-0b1645586a72",
		life:  state.Alive,
	}

	st := &mockState{
		model:    &model,
		isSystem: isSystem,
		watcher: &mockWatcher{
			changes: make(chan struct{}, 1),
		},
	}
	return st
}

func (m *mockState) EnsureModelRemoved() error {
	if !m.removed {
		return errors.New("found documents for model")
	}
	return nil
}

func (m *mockState) RemoveDyingModel() error {
	if m.model.life == state.Alive {
		return errors.New("model not dying or dead")
	}
	m.removed = true
	return nil
}

func (m *mockState) ProcessDyingModel() error {
	if m.model.life != state.Dying {
		return errors.New("model is not dying")
	}
	m.model.life = state.Dead
	return nil
}

func (m *mockState) IsController() bool {
	return m.isSystem
}

func (m *mockState) Model() (undertaker.Model, error) {
	return m.model, nil
}

func (m *mockState) ModelConfig() (*config.Config, error) {
	return &config.Config{}, nil
}

func (m *mockState) FindEntity(tag names.Tag) (state.Entity, error) {
	if tag.Kind() == names.ModelTagKind && tag.Id() == m.model.UUID() {
		return m.model, nil
	}
	return nil, errors.NotFoundf("entity with tag %q", tag.String())
}

func (m *mockState) WatchModelEntityReferences(mUUID string) state.NotifyWatcher {
	return m.watcher
}

func (m *mockState) ModelUUID() string {
	return m.model.UUID()
}

// mockModel implements Model interface and allows inspection of called
// methods.
type mockModel struct {
	tod    time.Time
	owner  names.UserTag
	life   state.Life
	name   string
	uuid   string
	forced bool

	status     status.Status
	statusInfo string
	statusData map[string]interface{}
}

var _ undertaker.Model = (*mockModel)(nil)

func (m *mockModel) Owner() names.UserTag {
	return m.owner
}

func (m *mockModel) Life() state.Life {
	return m.life
}

func (m *mockModel) ForceDestroyed() bool {
	return m.forced
}

func (m *mockModel) Tag() names.Tag {
	return names.NewModelTag(m.uuid)
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

func (m *mockModel) SetStatus(sInfo status.StatusInfo) error {
	m.status = sInfo.Status
	m.statusInfo = sInfo.Message
	m.statusData = sInfo.Data
	return nil
}

type mockWatcher struct {
	state.NotifyWatcher
	changes chan struct{}
}

func (w *mockWatcher) Changes() <-chan struct{} {
	return w.changes
}
