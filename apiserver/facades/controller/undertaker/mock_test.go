// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package undertaker_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/utils/v3"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/facades/controller/undertaker"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package undertaker_test -destination generatedmocks_test.go github.com/juju/juju/apiserver/facades/controller/undertaker State,Model
//go:generate go run go.uber.org/mock/mockgen -typed -package undertaker_test -destination secretsmocks_test.go github.com/juju/juju/secrets/provider SecretBackendProvider

// mockState implements State interface and allows inspection of called
// methods.
type mockState struct {
	*MockState

	model          *mockModel
	removed        bool
	isSystem       bool
	controllerUUID string

	watcher state.NotifyWatcher
}

var _ undertaker.State = (*mockState)(nil)

func newMockState(
	ctrl *gomock.Controller, modelOwner names.UserTag, modelName string,
	isSystem bool, modelCfg config.Config,
) *mockState {
	model := mockModel{
		MockModel:   NewMockModel(ctrl),
		owner:       modelOwner,
		name:        modelName,
		uuid:        "9d3d3b19-2b0c-4a3f-acde-0b1645586a72",
		life:        state.Alive,
		modelConfig: modelCfg,
	}
	model.legacy(ctrl)

	st := &mockState{
		MockState:      NewMockState(ctrl),
		model:          &model,
		isSystem:       isSystem,
		controllerUUID: utils.MustNewUUID().String(),
		watcher: &mockWatcher{
			changes: make(chan struct{}, 1),
		},
	}
	st.legacy(ctrl)

	return st
}

func (m *mockState) EnsureModelRemoved() error {
	if !m.removed {
		return errors.New("found documents for model")
	}
	return nil
}

func (m *mockState) legacy(ctrl *gomock.Controller) {
	m.MockState.EXPECT().RemoveDyingModel().DoAndReturn(func() error {
		if m.model.life == state.Alive {
			return errors.New("model not dying or dead")
		}
		m.removed = true
		return nil
	}).AnyTimes()

	m.MockState.EXPECT().ProcessDyingModel().DoAndReturn(func() error {
		if m.model.life != state.Dying {
			return errors.New("model is not dying")
		}
		m.model.life = state.Dead
		return nil
	}).AnyTimes()

	m.MockState.EXPECT().IsController().DoAndReturn(func() bool {
		return m.isSystem
	}).AnyTimes()

	m.MockState.EXPECT().Model().DoAndReturn(func() (undertaker.Model, error) {
		return m.model, nil
	}).AnyTimes()

	m.MockState.EXPECT().FindEntity(gomock.Any()).DoAndReturn(func(tag names.Tag) (state.Entity, error) {
		if tag.Kind() == names.ModelTagKind && tag.Id() == m.model.UUID() {
			return m.model, nil
		}
		return nil, errors.NotFoundf("entity with tag %q", tag.String())
	}).AnyTimes()

	m.MockState.EXPECT().WatchModelEntityReferences(gomock.Any()).DoAndReturn(func(mUUID string) state.NotifyWatcher {
		return m.watcher
	}).AnyTimes()

	m.MockState.EXPECT().ModelUUID().DoAndReturn(func() string {
		return m.model.UUID()
	}).AnyTimes()

	m.MockState.EXPECT().ControllerUUID().DoAndReturn(func() string {
		return m.controllerUUID
	}).AnyTimes()
}

// mockModel implements Model interface and allows inspection of called
// methods.
type mockModel struct {
	*MockModel

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

var _ undertaker.Model = (*mockModel)(nil)

func (m *mockModel) ControllerUUID() string {
	return coretesting.ControllerTag.Id()
}

func (m *mockModel) Cloud() (cloud.Cloud, error) {
	return cloud.Cloud{}, errors.NotImplemented
}

func (m *mockModel) Tag() names.Tag {
	return names.NewModelTag(m.uuid)
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

func (m *mockModel) legacy(ctrl *gomock.Controller) {
	m.MockModel.EXPECT().Owner().DoAndReturn(func() names.UserTag {
		return m.owner
	}).AnyTimes()

	m.MockModel.EXPECT().Life().DoAndReturn(func() state.Life {
		return m.life
	}).AnyTimes()

	m.MockModel.EXPECT().ForceDestroyed().DoAndReturn(func() bool {
		return m.forced
	}).AnyTimes()

	m.MockModel.EXPECT().DestroyTimeout().DoAndReturn(func() *time.Duration {
		return m.timeout
	}).AnyTimes()

	m.MockModel.EXPECT().Name().DoAndReturn(func() string {
		return m.name
	}).AnyTimes()

	m.MockModel.EXPECT().UUID().DoAndReturn(func() string {
		return m.uuid
	}).AnyTimes()

	m.MockModel.EXPECT().WatchForModelConfigChanges().DoAndReturn(func() state.NotifyWatcher {
		return nil
	}).AnyTimes()

	m.MockModel.EXPECT().Watch().DoAndReturn(func() state.NotifyWatcher {
		return nil
	}).AnyTimes()

	m.MockModel.EXPECT().ModelConfig().DoAndReturn(func() (*config.Config, error) {
		return &m.modelConfig, nil
	}).AnyTimes()
}

type mockWatcher struct {
	state.NotifyWatcher
	changes chan struct{}
}

func (w *mockWatcher) Changes() <-chan struct{} {
	return w.changes
}
