// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package undertaker

import (
	"context"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/secrets/provider"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/uuid"
	"github.com/juju/juju/state"
)

// mockState implements State interface and allows inspection of called
// methods.
type mockState struct {
	model          *mockModel
	removed        bool
	isSystem       bool
	controllerUUID string

	watcher state.NotifyWatcher
}

var _ State = (*mockState)(nil)

func newMockState(modelOwner names.UserTag, modelName string, isSystem bool) *mockState {
	model := mockModel{
		owner: modelOwner,
		name:  modelName,
		uuid:  "9d3d3b19-2b0c-4a3f-acde-0b1645586a72",
		life:  state.Alive,
	}

	st := &mockState{
		model:          &model,
		isSystem:       isSystem,
		controllerUUID: uuid.MustNewUUID().String(),
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

func (m *mockState) Model() (Model, error) {
	return m.model, nil
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

func (m *mockState) ControllerUUID() string {
	return m.controllerUUID
}

// mockModel implements Model interface and allows inspection of called
// methods.
type mockModel struct {
	owner       names.UserTag
	life        state.Life
	name        string
	uuid        string
	forced      bool
	timeout     *time.Duration
	modelConfig config.Config

	status     status.Status
	statusInfo string
	statusData map[string]interface{}
}

var _ Model = (*mockModel)(nil)

func (m *mockModel) ControllerUUID() string {
	return coretesting.ControllerTag.Id()
}

func (m *mockModel) Owner() names.UserTag {
	return m.owner
}

func (m *mockModel) Life() state.Life {
	return m.life
}

func (m *mockModel) ForceDestroyed() bool {
	return m.forced
}

func (m *mockModel) DestroyTimeout() *time.Duration {
	return m.timeout
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

func (m *mockModel) WatchForModelConfigChanges() state.NotifyWatcher {
	return nil
}

func (m *mockModel) Watch() state.NotifyWatcher {
	return nil
}

func (m *mockModel) ModelConfig(context.Context) (*config.Config, error) {
	return &m.modelConfig, nil
}

type mockWatcher struct {
	state.NotifyWatcher
	changes chan struct{}
}

func (w *mockWatcher) Changes() <-chan struct{} {
	return w.changes
}

type mockSecrets struct {
	provider.SecretBackendProvider
	cleanedUUID string
}

func (m *mockSecrets) CleanupModel(_ context.Context, cfg *provider.ModelBackendConfig) error {
	if cfg.BackendType != "some-backend" {
		return errors.New("unknown backend " + cfg.BackendType)
	}
	m.cleanedUUID = cfg.ModelUUID
	return nil
}
