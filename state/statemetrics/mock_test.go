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
	models []*mockModel
}

func (p *mockStatePool) SystemState() statemetrics.State {
	return p.system
}

func (p *mockStatePool) Get(modelUUID string) (statemetrics.State, state.StatePoolReleaser, error) {
	p.MethodCall(p, "Get", modelUUID)
	if err := p.NextErr(); err != nil {
		return nil, nil, err
	}
	for _, m := range p.models {
		if m.tag.Id() == modelUUID {
			st := &mockState{
				model:      m,
				modelUUIDs: p.modelUUIDs(),
			}
			return st, st.release, nil
		}
	}
	panic("model not found")
}

func (p *mockStatePool) GetModel(modelUUID string) (statemetrics.Model, state.StatePoolReleaser, error) {
	p.MethodCall(p, "GetModel", modelUUID)
	if err := p.NextErr(); err != nil {
		return nil, nil, err
	}
	for _, m := range p.models {
		if m.tag.Id() == modelUUID {
			return m, func() bool { return true }, nil
		}
	}
	panic("model not found")
}

func (p *mockStatePool) modelUUIDs() []string {
	out := make([]string, len(p.models))
	for i, m := range p.models {
		out[i] = m.tag.Id()
	}
	return out
}

type mockState struct {
	statemetrics.State
	testing.Stub

	model      *mockModel
	modelUUIDs []string
	users      []*mockUser
}

func (m *mockState) AllModelUUIDs() ([]string, error) {
	m.MethodCall(m, "AllModelUUIDs")
	if err := m.NextErr(); err != nil {
		return nil, err
	}
	return m.modelUUIDs, nil
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

func (m *mockState) release() bool {
	m.MethodCall(m, "release")
	return false
}

func (m *mockState) AllMachines() ([]statemetrics.Machine, error) {
	m.MethodCall(m, "AllMachines")
	if err := m.NextErr(); err != nil {
		return nil, err
	}
	out := make([]statemetrics.Machine, len(m.model.machines))
	for i, machine := range m.model.machines {
		out[i] = machine
	}
	return out, nil
}

func (m *mockState) Model() (statemetrics.Model, error) {
	return m.model, nil
}

type mockModel struct {
	testing.Stub
	tag      names.ModelTag
	life     state.Life
	status   status.StatusInfo
	machines []*mockMachine
	users    []*mockUser
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

func (m *mockModel) UserAccess(subject names.UserTag, object names.Tag) (permission.UserAccess, error) {
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
