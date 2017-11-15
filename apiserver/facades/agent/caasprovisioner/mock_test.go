// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasprovisioner_test

import (
	"github.com/juju/testing"
	"gopkg.in/tomb.v1"

	"github.com/juju/juju/apiserver/facades/agent/caasprovisioner"
	"github.com/juju/juju/state"
)

type mockState struct {
	testing.Stub
	applicationWatcher *mockStringsWatcher
	caasModel          *mockCAASModel
}

func newMockState() *mockState {
	return &mockState{
		applicationWatcher: newMockStringsWatcher(),
	}
}

func (st *mockState) CAASModel() (caasprovisioner.CAASModel, error) {
	st.MethodCall(st, "CAASModel")
	return st.caasModel, nil
}

func (st *mockState) WatchApplications() state.StringsWatcher {
	st.MethodCall(st, "WatchApplications")
	return st.applicationWatcher
}

type mockCAASModel struct {
	connectionCfg state.CAASConnectionConfig
}

func (m *mockCAASModel) ConnectionConfig() (state.CAASConnectionConfig, error) {
	return m.connectionCfg, nil
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
