// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENSE file for details.

package statemetrics_test

import (
	"github.com/juju/testing"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/permission"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/statemetrics"
	"github.com/juju/juju/status"
	coretesting "github.com/juju/juju/testing"
)

type mockStatePool struct {
	testing.Stub
	system *mockState
}

func (m *mockStatePool) SystemState() statemetrics.State {
	return m.system
}

func (m *mockStatePool) Get(modelUUID string) (statemetrics.State, state.StatePoolReleaser, error) {
	m.MethodCall(m, "Get", modelUUID)
	if err := m.NextErr(); err != nil {
		return nil, nil, err
	}
	for _, m := range m.system.models {
		if m.tag.Id() == modelUUID {
			st := mockModelState{mockModel: m}
			return st, st.release, nil
		}
	}
	panic("model not found")
}

type mockState struct {
	statemetrics.State

	testing.Stub
	models []*mockModel
	users  []*mockUser
}

func (m *mockState) AllModels() ([]statemetrics.Model, error) {
	m.MethodCall(m, "AllModels")
	if err := m.NextErr(); err != nil {
		return nil, err
	}
	out := make([]statemetrics.Model, len(m.models))
	for i, m := range m.models {
		out[i] = m
	}
	return out, nil
}

func (m *mockState) AllUsers() ([]statemetrics.User, error) {
	m.MethodCall(m, "AllUsers")
	if err := m.NextErr(); err != nil {
		return nil, err
	}
	out := make([]statemetrics.User, len(m.users))
	for i, u := range m.users {
		out[i] = u
	}
	return out, nil
}

func (m *mockState) ControllerTag() names.ControllerTag {
	m.MethodCall(m, "ControllerTag")
	return coretesting.ControllerTag
}

func (m *mockState) UserAccess(subject names.UserTag, object names.Tag) (permission.UserAccess, error) {
	m.MethodCall(m, "UserAccess", subject, object)
	if err := m.NextErr(); err != nil {
		return permission.UserAccess{}, err
	}
	for _, u := range m.users {
		if u.tag == subject {
			return permission.UserAccess{Access: u.controllerAccess}, nil
		}
	}
	panic("subject not found")
}

type mockModelState struct {
	statemetrics.State
	*mockModel
}

func (m *mockModelState) release() bool {
	m.MethodCall(m, "release")
	return false
}

func (m mockModelState) AllMachines() ([]statemetrics.Machine, error) {
	m.MethodCall(m, "AllMachines")
	if err := m.NextErr(); err != nil {
		return nil, err
	}
	out := make([]statemetrics.Machine, len(m.machines))
	for i, m := range m.machines {
		out[i] = m
	}
	return out, nil
}

type mockModel struct {
	testing.Stub
	tag      names.ModelTag
	life     state.Life
	status   status.StatusInfo
	machines []*mockMachine
}

func (m *mockModel) Life() state.Life {
	m.MethodCall(m, "Life")
	return m.life
}

func (m *mockModel) ModelTag() names.ModelTag {
	m.MethodCall(m, "ModelTag")
	return m.tag
}

func (m *mockModel) Status() (status.StatusInfo, error) {
	m.MethodCall(m, "Status")
	if err := m.NextErr(); err != nil {
		return status.StatusInfo{}, err
	}
	return m.status, nil
}

type mockUser struct {
	testing.Stub
	tag              names.UserTag
	deleted          bool
	disabled         bool
	controllerAccess permission.Access
}

func (u *mockUser) UserTag() names.UserTag {
	u.MethodCall(u, "UserTag")
	return u.tag
}

func (u *mockUser) IsDeleted() bool {
	u.MethodCall(u, "IsDeleted")
	return u.deleted
}

func (u *mockUser) IsDisabled() bool {
	u.MethodCall(u, "IsDisabled")
	return u.disabled
}

type mockMachine struct {
	testing.Stub
	instanceStatus status.StatusInfo
	agentStatus    status.StatusInfo
	life           state.Life
}

func (m *mockMachine) Life() state.Life {
	m.MethodCall(m, "Life")
	return m.life
}

func (m *mockMachine) InstanceStatus() (status.StatusInfo, error) {
	m.MethodCall(m, "InstanceStatus")
	if err := m.NextErr(); err != nil {
		return status.StatusInfo{}, err
	}
	return m.instanceStatus, nil
}

func (m *mockMachine) Status() (status.StatusInfo, error) {
	m.MethodCall(m, "Status")
	if err := m.NextErr(); err != nil {
		return status.StatusInfo{}, err
	}
	return m.agentStatus, nil
}
