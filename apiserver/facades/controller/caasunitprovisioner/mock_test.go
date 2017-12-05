// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasunitprovisioner_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/facades/controller/caasunitprovisioner"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
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
	if err := st.NextErr(); err != nil {
		return nil, err
	}
	return &st.application, nil
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
	life         state.Life
	unitsWatcher *statetesting.MockStringsWatcher
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

type mockUnit struct {
	testing.Stub
	life state.Life
}

func (*mockUnit) Tag() names.Tag {
	panic("should not be called")
}

func (u *mockUnit) Life() state.Life {
	u.MethodCall(u, "Life")
	return u.life
}
