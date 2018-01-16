// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperatorprovisioner_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	"gopkg.in/juju/names.v2"
	"gopkg.in/tomb.v1"

	"github.com/juju/juju/apiserver/facades/controller/caasoperatorprovisioner"
	"github.com/juju/juju/state"
	"github.com/juju/juju/status"
)

type mockState struct {
	testing.Stub
	applicationWatcher *mockStringsWatcher
	app                *mockApplication
}

func newMockState() *mockState {
	return &mockState{
		applicationWatcher: newMockStringsWatcher(),
	}
}

func (st *mockState) WatchApplications() state.StringsWatcher {
	st.MethodCall(st, "WatchApplications")
	return st.applicationWatcher
}

func (st *mockState) FindEntity(tag names.Tag) (state.Entity, error) {
	if st.app.tag == tag {
		return st.app, nil
	}
	return nil, errors.NotFoundf("entity %v", tag)
}

func (st *mockState) Application(name string) (caasoperatorprovisioner.Application, error) {
	if st.app.tag.Id() == name {
		return st.app, nil
	}
	return nil, errors.NotFoundf("application %v", name)
}

type mockApplication struct {
	testing.Stub

	state.Authenticator
	tag      names.Tag
	password string
	units    []caasoperatorprovisioner.Unit
	ops      *state.UpdateUnitsOperation
}

func (m *mockApplication) Tag() names.Tag {
	return m.tag
}

func (m *mockApplication) SetPassword(password string) error {
	m.password = password
	return nil
}

func (m *mockApplication) AllUnits() (units []caasoperatorprovisioner.Unit, err error) {
	return m.units, nil
}

func (m *mockApplication) UpdateUnits(ops *state.UpdateUnitsOperation) error {
	m.ops = ops
	return nil
}

var addOp = &state.AddUnitOperation{}

func (m *mockApplication) AddOperation(props state.UnitUpdateProperties) *state.AddUnitOperation {
	m.MethodCall(m, "AddOperation", props)
	return addOp
}

type mockUnit struct {
	testing.Stub
	name       string
	providerId string
}

func (m *mockUnit) Name() string {
	return m.name
}

func (m *mockUnit) Life() state.Life {
	return state.Alive
}

func (m *mockUnit) ProviderId() string {
	return m.providerId
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

type mockWatcher struct {
	testing.Stub
	tomb.Tomb
}

func (w *mockWatcher) doneWhenDying() {
	<-w.Tomb.Dying()
	w.Tomb.Done()
}

func (w *mockWatcher) Kill() {
	w.MethodCall(w, "Kill")
	w.Tomb.Kill(nil)
}

func (w *mockWatcher) Stop() error {
	w.MethodCall(w, "Stop")
	if err := w.NextErr(); err != nil {
		return err
	}
	w.Tomb.Kill(nil)
	return w.Tomb.Wait()
}

type mockStringsWatcher struct {
	mockWatcher
	changes chan []string
}

func newMockStringsWatcher() *mockStringsWatcher {
	w := &mockStringsWatcher{changes: make(chan []string, 1)}
	go w.doneWhenDying()
	return w
}

func (w *mockStringsWatcher) Changes() <-chan []string {
	w.MethodCall(w, "Changes")
	return w.changes
}
