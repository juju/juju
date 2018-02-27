// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasunitprovisioner_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/facades/controller/caasunitprovisioner"
	"github.com/juju/juju/core/application"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/status"
)

type mockState struct {
	testing.Stub
	application         mockApplication
	applicationsWatcher *statetesting.MockStringsWatcher
	model               mockModel
	unit                mockUnit
}

func (st *mockState) WatchApplications() state.StringsWatcher {
	st.MethodCall(st, "WatchApplications")
	return st.applicationsWatcher
}

func (st *mockState) Application(name string) (caasunitprovisioner.Application, error) {
	st.MethodCall(st, "Application", name)
	if name != st.application.tag.Id() {
		return nil, errors.NotFoundf("application %v", name)
	}
	return &st.application, st.NextErr()
}

func (st *mockState) FindEntity(tag names.Tag) (state.Entity, error) {
	st.MethodCall(st, "FindEntity", tag)
	if err := st.NextErr(); err != nil {
		return nil, err
	}
	switch tag.(type) {
	case names.ApplicationTag:
		return &st.application, nil
	case names.UnitTag:
		return &st.unit, nil
	default:
		return nil, errors.NotFoundf("%s", names.ReadableString(tag))
	}
}

func (st *mockState) Model() (caasunitprovisioner.Model, error) {
	st.MethodCall(st, "Model")
	if err := st.NextErr(); err != nil {
		return nil, err
	}
	return &st.model, nil
}

type mockModel struct {
	testing.Stub
	containerSpecWatcher *statetesting.MockNotifyWatcher
}

func (m *mockModel) ContainerSpec(tag names.Tag) (string, error) {
	m.MethodCall(m, "ContainerSpec", tag)
	if err := m.NextErr(); err != nil {
		return "", err
	}
	return "spec(" + tag.Id() + ")", nil
}

func (m *mockModel) WatchContainerSpec(tag names.Tag) (state.NotifyWatcher, error) {
	m.MethodCall(m, "WatchContainerSpec", tag)
	if err := m.NextErr(); err != nil {
		return nil, err
	}
	return m.containerSpecWatcher, nil
}

type mockApplication struct {
	testing.Stub
	life             state.Life
	unitsWatcher     *statetesting.MockStringsWatcher
	jujuManagedUnits bool

	tag        names.Tag
	units      []caasunitprovisioner.Unit
	ops        *state.UpdateUnitsOperation
	providerId string
	addresses  []network.Address
}

func (*mockApplication) Tag() names.Tag {
	panic("should not be called")
}

func (a *mockApplication) Life() state.Life {
	a.MethodCall(a, "Life")
	return a.life
}

func (a *mockApplication) WatchUnits() state.StringsWatcher {
	a.MethodCall(a, "WatchUnits")
	return a.unitsWatcher
}

func (a *mockApplication) ApplicationConfig() (application.ConfigAttributes, error) {
	a.MethodCall(a, "ApplicationConfig")
	return application.ConfigAttributes{"foo": "bar", "juju-managed-units": a.jujuManagedUnits}, a.NextErr()
}

func (m *mockApplication) AllUnits() (units []caasunitprovisioner.Unit, err error) {
	return m.units, nil
}

func (m *mockApplication) UpdateUnits(ops *state.UpdateUnitsOperation) error {
	m.ops = ops
	return nil
}

func (m *mockApplication) UpdateCloudService(providerId string, addreses []network.Address) error {
	m.providerId = providerId
	m.addresses = addreses
	return nil
}

var addOp = &state.AddUnitOperation{}

func (m *mockApplication) AddOperation(props state.UnitUpdateProperties) *state.AddUnitOperation {
	m.MethodCall(m, "AddOperation", props)
	return addOp
}

type mockContainerInfo struct {
	state.CloudContainer
	providerId string
}

func (m *mockContainerInfo) ProviderId() string {
	return m.providerId
}

type mockUnit struct {
	testing.Stub
	name          string
	life          state.Life
	containerInfo *mockContainerInfo
}

func (*mockUnit) Tag() names.Tag {
	panic("should not be called")
}

func (u *mockUnit) UnitTag() names.UnitTag {
	return names.NewUnitTag(u.name)
}

func (u *mockUnit) Life() state.Life {
	u.MethodCall(u, "Life")
	return u.life
}

func (m *mockUnit) Name() string {
	return m.name
}

func (m *mockUnit) ContainerInfo() (state.CloudContainer, error) {
	if m.containerInfo == nil {
		return nil, errors.NotFoundf("container info")
	}
	return m.containerInfo, nil
}

func (m *mockUnit) AgentStatus() (status.StatusInfo, error) {
	return status.StatusInfo{Status: status.Allocating}, nil
}

var updateOp = &state.UpdateUnitOperation{}

func (m *mockUnit) UpdateOperation(props state.UnitUpdateProperties) *state.UpdateUnitOperation {
	m.MethodCall(m, "UpdateOperation", props)
	return updateOp
}

var destroyOp = &state.DestroyUnitOperation{}

func (m *mockUnit) DestroyOperation() *state.DestroyUnitOperation {
	m.MethodCall(m, "DestroyOperation")
	return destroyOp
}
