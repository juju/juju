// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasmodeloperator_test

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facades/controller/caasmodeloperator"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
)

type mockModel struct {
	password           string
	tag                names.Tag
	modelConfigChanged state.NotifyWatcher
}

type mockState struct {
	common.APIAddressAccessor
	operatorRepo                 string
	model                        *mockModel
	controllerConfigWatcher      state.NotifyWatcher
	apiHostPortsForAgentsWatcher state.NotifyWatcher
}

func newMockState() *mockState {
	return &mockState{
		model: &mockModel{},
	}
}

func (st *mockState) APIHostPortsForAgents(controllerConf controller.Config) ([]network.SpaceHostPorts, error) {
	return []network.SpaceHostPorts{
		network.NewSpaceHostPorts(1, "10.0.0.1"),
	}, nil
}

func (st *mockState) ModelUUID() string {
	return st.model.UUID()
}

func (st *mockState) ControllerConfig() (controller.Config, error) {
	cfg := coretesting.FakeControllerConfig()
	cfg[controller.CAASImageRepo] = st.operatorRepo
	return cfg, nil
}

func (st *mockState) FindEntity(tag names.Tag) (state.Entity, error) {
	if st.model.tag == tag {
		return st.model, nil
	}
	return nil, errors.NotFoundf("entity %v", tag)
}

func (st *mockState) Model() (caasmodeloperator.Model, error) {
	return st.model, nil
}

func (m *mockState) WatchControllerConfig() state.NotifyWatcher {
	return m.controllerConfigWatcher
}

func (m *mockState) WatchAPIHostPortsForAgents() state.NotifyWatcher {
	return m.apiHostPortsForAgentsWatcher
}

func (m *mockModel) Tag() names.Tag {
	return m.tag
}

func (m *mockModel) SetPassword(password string) error {
	m.password = password
	return nil
}

func (m *mockModel) UUID() string {
	return coretesting.ModelTag.Id()
}

func (m *mockModel) ModelConfig(_ context.Context) (*config.Config, error) {
	attrs := coretesting.FakeConfig()
	attrs["operator-storage"] = "k8s-storage"
	attrs["agent-version"] = "2.6-beta3"
	return config.New(config.UseDefaults, attrs)
}

func (m *mockModel) WatchForModelConfigChanges() state.NotifyWatcher {
	return m.modelConfigChanged
}
