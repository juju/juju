// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasapplication_test

import (
	"github.com/juju/charm/v9"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	"github.com/juju/version/v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facades/agent/caasapplication"
	"github.com/juju/juju/caas"
	_ "github.com/juju/juju/caas/kubernetes/provider"
	jujucontroller "github.com/juju/juju/controller"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/state"
	jtesting "github.com/juju/juju/testing"
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
	units := make(map[string]*mockUnit)
	st := &mockState{
		units: units,
		model: mockModel{
			agentVersion:  version.MustParse("1.9.99"),
			controllerTag: names.NewControllerTag("ffffffff-ffff-ffff-ffff-ffffffffffff"),
			tag:           names.NewModelTag("ffffffff-ffff-ffff-ffff-ffffffffffff"),
		},
		app: mockApplication{
			units: units,
			name:  "gitlab",
			life:  state.Alive,
			charm: mockCharm{
				url:    charm.MustParseURL("cs:gitlab-1"),
				sha256: "fake-sha256",
				manifest: &charm.Manifest{
					// charm.FormatV2.
					Bases: []charm.Base{
						{
							Name: "ubuntu",
							Channel: charm.Channel{
								Risk:  "stable",
								Track: "20.04",
							},
						},
					},
				},
				meta: &charm.Meta{
					Deployment: &charm.Deployment{
						DeploymentType: charm.DeploymentStateful,
					},
				},
			},
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

func (st *mockState) APIHostPortsForAgents() ([]network.SpaceHostPorts, error) {
	st.MethodCall(st, "APIHostPortsForAgents")
	addrs := network.NewSpaceAddresses("52.7.1.1", "10.0.2.1")
	ctlr1 := network.SpaceAddressesWithPort(addrs, 17070)
	return []network.SpaceHostPorts{ctlr1}, nil
}

type mockModel struct {
	testing.Stub
	containers    []state.CloudContainer
	agentVersion  version.Number
	controllerTag names.ControllerTag
	tag           names.Tag
}

func (st *mockModel) Containers(providerIds ...string) ([]state.CloudContainer, error) {
	st.MethodCall(st, "Containers", providerIds)
	return st.containers, st.NextErr()
}

func (st *mockModel) AgentVersion() (version.Number, error) {
	st.MethodCall(st, "AgentVersion")
	if err := st.NextErr(); err != nil {
		return version.Zero, err
	}
	return st.agentVersion, nil
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
	life         state.Life
	charm        mockCharm
	forceUpgrade bool
	name         string
	newUnit      caasapplication.Unit
	units        map[string]*mockUnit
	scale        int
}

func (*mockApplication) Tag() names.Tag {
	return names.NewApplicationTag("gitlab")
}

func (a *mockApplication) Life() state.Life {
	a.MethodCall(a, "Life")
	return a.life
}

func (a *mockApplication) Charm() (caasapplication.Charm, bool, error) {
	a.MethodCall(a, "Charm")
	if err := a.NextErr(); err != nil {
		return nil, false, err
	}
	return &a.charm, a.forceUpgrade, nil
}

func (a *mockApplication) AllUnits() ([]caasapplication.Unit, error) {
	a.MethodCall(a, "AllUnits")
	if err := a.NextErr(); err != nil {
		return nil, err
	}
	var units []caasapplication.Unit
	for _, v := range a.units {
		units = append(units, v)
	}
	return units, nil
}

func (a *mockApplication) Name() string {
	a.MethodCall(a, "Name")
	return a.name
}

func (a *mockApplication) UpdateUnits(unitsOp *state.UpdateUnitsOperation) error {
	a.MethodCall(a, "UpdateUnits", unitsOp)
	return a.NextErr()
}

func (a *mockApplication) AddUnit(args state.AddUnitParams) (unit caasapplication.Unit, err error) {
	a.MethodCall(a, "AddUnit", args)
	if err := a.NextErr(); err != nil {
		return nil, err
	}
	return a.newUnit, nil
}

func (a *mockApplication) GetScale() int {
	a.MethodCall(a, "GetScale")
	return a.scale
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

type mockCharm struct {
	url      *charm.URL
	sha256   string
	meta     *charm.Meta
	manifest *charm.Manifest
}

func (ch *mockCharm) URL() *charm.URL {
	return ch.url
}

func (ch *mockCharm) BundleSha256() string {
	return ch.sha256
}

func (ch *mockCharm) Meta() *charm.Meta {
	return ch.meta
}

func (ch *mockCharm) Manifest() *charm.Manifest {
	return ch.manifest
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

type mockLeadershipRevoker struct {
	revoked set.Strings
}

func (s *mockLeadershipRevoker) RevokeLeadership(applicationId, unitId string) error {
	s.revoked.Add(unitId)
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
