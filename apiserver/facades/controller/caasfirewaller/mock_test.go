// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasfirewaller_test

import (
	"github.com/juju/testing"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/facades/controller/caasfirewaller"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
)

type mockState struct {
	testing.Stub
	application         mockApplication
	applicationsWatcher *statetesting.MockStringsWatcher
	appExposedWatcher   *statetesting.MockNotifyWatcher
}

func (st *mockState) WatchApplications() state.StringsWatcher {
	st.MethodCall(st, "WatchApplications")
	return st.applicationsWatcher
}

func (st *mockState) Application(name string) (caasfirewaller.Application, error) {
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
	return &st.application, nil
}

type mockApplication struct {
	testing.Stub
	life    state.Life
	exposed bool
}

func (*mockApplication) Tag() names.Tag {
	panic("should not be called")
}

func (a *mockApplication) Life() state.Life {
	a.MethodCall(a, "Life")
	return a.life
}

func (a *mockApplication) IsExposed() bool {
	a.MethodCall(a, "IsExposed")
	return a.exposed
}

func (a *mockApplication) Watch() state.NotifyWatcher {
	return apiservertesting.NewFakeNotifyWatcher()
}
