// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasapplication_test

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/testing"
	"github.com/juju/version/v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facades/agent/caasapplication"
	"github.com/juju/juju/caas"
	_ "github.com/juju/juju/caas/kubernetes/provider"
	"github.com/juju/juju/controller"
	jujucontroller "github.com/juju/juju/controller"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/charm"
	jtesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/state"
)

type mockState struct {
	testing.Stub
	common.APIAddressAccessor
	app              mockApplication
	model            mockModel
	units            map[string]*mockUnit
	controllerConfig jujucontroller.Config
}

func newMockState() *mockState {
	st := &mockState{
		model: mockModel{
			controllerTag: names.NewControllerTag("ffffffff-ffff-ffff-ffff-ffffffffffff"),
			tag:           names.NewModelTag("ffffffff-ffff-ffff-ffff-ffffffffffff"),
		},
		controllerConfig: jujucontroller.Config{
			jujucontroller.CACertKey: jtesting.CACert,
		},
	}
	return st
}

func (st *mockState) Application(id string) (caasapplication.Application, error) {
	st.MethodCall(st, "Application", id)
	if err := st.NextErr(); err != nil {
		return nil, err
	}
	return &st.app, nil
}

func (st *mockState) Model() (caasapplication.Model, error) {
	st.MethodCall(st, "Model")
	if err := st.NextErr(); err != nil {
		return nil, err
	}
	return &st.model, nil
}

func (st *mockState) Unit(name string) (caasapplication.Unit, error) {
	st.MethodCall(st, "Unit", name)
	if err := st.NextErr(); err != nil {
		return nil, err
	}
	unit, ok := st.units[name]
	if !ok {
		return nil, errors.NotFoundf("unit %s", name)
	}
	return unit, nil
}

func (st *mockState) ControllerConfig() (jujucontroller.Config, error) {
	st.MethodCall(st, "ControllerConfig")
	if err := st.NextErr(); err != nil {
		return nil, err
	}
	return st.controllerConfig, nil
}

func (st *mockState) APIHostPortsForAgents(_ controller.Config) ([]network.SpaceHostPorts, error) {
	st.MethodCall(st, "APIHostPortsForAgents")
	addrs := network.NewSpaceAddresses("52.7.1.1", "10.0.2.1")
	ctlr1 := network.SpaceAddressesWithPort(addrs, 17070)
	return []network.SpaceHostPorts{ctlr1}, nil
}

type mockModel struct {
	testing.Stub
	containers    []state.CloudContainer
	controllerTag names.ControllerTag
	tag           names.Tag
}

func (st *mockModel) Config() (*config.Config, error) {
	attr := jtesting.FakeConfig()
	return config.New(config.UseDefaults, attr)
}

func (st *mockModel) Containers(providerIds ...string) ([]state.CloudContainer, error) {
	st.MethodCall(st, "Containers", providerIds)
	return st.containers, st.NextErr()
}

func (st *mockModel) ControllerTag() names.ControllerTag {
	st.MethodCall(st, "ControllerTag")
	return st.controllerTag
}

func (st *mockModel) Tag() names.Tag {
	st.MethodCall(st, "Tag")
	return st.tag
}

type mockApplication struct {
	testing.Stub
	unit *mockUnit
}

func (a *mockApplication) UpsertCAASUnit(
	modelConfigService common.ModelConfigService,
	args state.UpsertCAASUnitParams,
) (caasapplication.Unit, error) {
	a.MethodCall(a, "UpsertCAASUnit", modelConfigService, args)
	return a.unit, a.NextErr()
}

type mockUnit struct {
	testing.Stub
	life          state.Life
	containerInfo state.CloudContainer
	updateOp      *state.UpdateUnitOperation
}

func (*mockUnit) Tag() names.Tag {
	return names.NewUnitTag("gitlab/0")
}

func (u *mockUnit) Life() state.Life {
	u.MethodCall(u, "Life")
	return u.life
}

func (u *mockUnit) ContainerInfo() (state.CloudContainer, error) {
	u.MethodCall(u, "ContainerInfo")
	if err := u.NextErr(); err != nil {
		return nil, err
	}
	return u.containerInfo, nil
}

func (u *mockUnit) Refresh() error {
	u.MethodCall(u, "Refresh")
	return u.NextErr()
}

func (u *mockUnit) UpdateOperation(props state.UnitUpdateProperties) *state.UpdateUnitOperation {
	u.MethodCall(u, "UpdateOperation", props)
	return u.updateOp
}

func (u *mockUnit) SetPassword(password string) error {
	u.MethodCall(u, "SetPassword", password)
	return u.NextErr()
}

func (u *mockUnit) ApplicationName() string {
	u.MethodCall(u, "ApplicationName")
	return "gitlab"
}

type mockModelAgent struct {
	testing.Stub

	agentVersion version.Number
}

func (m *mockModelAgent) GetModelTargetAgentVersion(ctx context.Context) (version.Number, error) {
	m.MethodCall(m, "GetModelAgentVersion")
	if err := m.NextErr(); err != nil {
		return version.Zero, err
	}
	return m.agentVersion, nil
}

type mockBroker struct {
	testing.Stub
	app *mockCAASApplication
}

func (b *mockBroker) Application(appName string, deploymentType caas.DeploymentType) caas.Application {
	b.MethodCall(b, "Application", appName, deploymentType)
	return b.app
}

type mockCloudContainer struct {
	unit       string
	providerID string
}

func (cc *mockCloudContainer) Unit() string {
	return cc.unit
}

func (cc *mockCloudContainer) ProviderId() string {
	return cc.providerID
}

func (cc *mockCloudContainer) Address() *network.SpaceAddress {
	return nil
}

func (cc *mockCloudContainer) Ports() []string {
	return nil
}

type mockCAASApplication struct {
	testing.Stub
	caas.Application

	state caas.ApplicationState
	units []caas.Unit
}

func (a *mockCAASApplication) State() (caas.ApplicationState, error) {
	a.MethodCall(a, "State")
	return a.state, a.NextErr()
}

func (a *mockCAASApplication) Units() ([]caas.Unit, error) {
	a.MethodCall(a, "Units")
	return a.units, a.NextErr()
}

type stubCharm struct {
	charm.Charm
}

func (s *stubCharm) Meta() *charm.Meta {
	return &charm.Meta{
		Name: "gitlab",
	}
}

func (s *stubCharm) Manifest() *charm.Manifest {
	return &charm.Manifest{}
}

func (s *stubCharm) Config() *charm.Config {
	return &charm.Config{}
}

func (s *stubCharm) Actions() *charm.Actions {
	return &charm.Actions{}
}

func (s *stubCharm) Revision() int {
	return 1
}
