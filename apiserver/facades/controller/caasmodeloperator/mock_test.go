// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasmodeloperator_test

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/v2/apiserver/common"
	"github.com/juju/juju/v2/apiserver/facades/controller/caasmodeloperator"
	"github.com/juju/juju/v2/controller"
	"github.com/juju/juju/v2/core/network"
	"github.com/juju/juju/v2/environs/config"
	"github.com/juju/juju/v2/state"
	coretesting "github.com/juju/juju/v2/testing"
)

type mockModel struct {
	password string
	tag      names.Tag
}

type mockState struct {
	common.APIAddressAccessor
	operatorRepo string
	model        *mockModel
}

func newMockState() *mockState {
	return &mockState{
		model: &mockModel{},
	}
}

func (st *mockState) APIHostPortsForAgents() ([]network.SpaceHostPorts, error) {
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

func (m *mockModel) ModelConfig() (*config.Config, error) {
	attrs := coretesting.FakeConfig()
	attrs["operator-storage"] = "k8s-storage"
	attrs["agent-version"] = "2.6-beta3"
	return config.New(config.UseDefaults, attrs)
}
