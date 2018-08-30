// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperatorprovisioner_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	"gopkg.in/juju/names.v2"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/apiserver/common"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/caas/kubernetes/provider"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/poolmanager"
	coretesting "github.com/juju/juju/testing"
)

type mockState struct {
	testing.Stub
	common.AddressAndCertGetter
	applicationWatcher *mockStringsWatcher
	app                *mockApplication
	operatorImage      string
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

func (st *mockState) ControllerConfig() (controller.Config, error) {
	cfg := coretesting.FakeControllerConfig()
	cfg[controller.CAASOperatorImagePath] = st.operatorImage
	return cfg, nil
}

func (st *mockState) APIHostPortsForAgents() ([][]network.HostPort, error) {
	st.MethodCall(st, "APIHostPortsForAgents")
	return nil, nil
}

func (st *mockState) WatchAPIHostPortsForAgents() state.NotifyWatcher {
	st.MethodCall(st, "WatchAPIHostPortsForAgents")
	return apiservertesting.NewFakeNotifyWatcher()
}

type mockStorageProviderRegistry struct {
	testing.Stub
	storage.ProviderRegistry
}

func (m *mockStorageProviderRegistry) StorageProvider(providerType storage.ProviderType) (storage.Provider, error) {
	m.MethodCall(m, "StorageProvider", providerType)
	return nil, errors.NotSupportedf("StorageProvider")
}

type mockStoragePoolManager struct {
	testing.Stub
	poolmanager.PoolManager
}

func (m *mockStoragePoolManager) Get(name string) (*storage.Config, error) {
	m.MethodCall(m, "Get", name)
	if err := m.NextErr(); err != nil {
		return nil, err
	}
	return storage.NewConfig(name, provider.K8s_ProviderType, map[string]interface{}{"foo": "bar"})
}

type mockApplication struct {
	state.Authenticator
	tag      names.Tag
	password string
}

func (m *mockApplication) Tag() names.Tag {
	return m.tag
}

func (m *mockApplication) SetPassword(password string) error {
	m.password = password
	return nil
}

func (a *mockApplication) Life() state.Life {
	return state.Alive
}

type mockWatcher struct {
	testing.Stub
	tomb.Tomb
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
	w.Tomb.Go(func() error {
		<-w.Tomb.Dying()
		return nil
	})
	return w
}

func (w *mockStringsWatcher) Changes() <-chan []string {
	w.MethodCall(w, "Changes")
	return w.changes
}
