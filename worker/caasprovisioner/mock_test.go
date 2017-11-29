// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasprovisioner_test

import (
	"sync"

	"github.com/juju/testing"
	"gopkg.in/juju/names.v2"
	"gopkg.in/tomb.v1"

	"github.com/juju/juju/agent"
	apicaasprovisioner "github.com/juju/juju/api/caasprovisioner"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/caas"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/watcher"
	"github.com/juju/juju/worker/caasprovisioner"
)

type mockProvisionerFacade struct {
	mu   sync.Mutex
	stub *testing.Stub
	caasprovisioner.CAASProvisionerFacade
	applicationsWatcher *mockStringsWatcher
	passwords           []apicaasprovisioner.ApplicationPassword
}

func newMockProvisionerFacade(stub *testing.Stub) *mockProvisionerFacade {
	return &mockProvisionerFacade{
		stub:                stub,
		applicationsWatcher: newMockStringsWatcher(),
	}
}

func (m *mockProvisionerFacade) WatchApplications() (watcher.StringsWatcher, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stub.MethodCall(m, "WatchApplications")
	if err := m.stub.NextErr(); err != nil {
		return nil, err
	}
	return m.applicationsWatcher, nil
}

func (m *mockProvisionerFacade) SetPasswords(passwords []apicaasprovisioner.ApplicationPassword) (params.ErrorResults, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stub.MethodCall(m, "SetPasswords", passwords)
	if err := m.stub.NextErr(); err != nil {
		return params.ErrorResults{}, err
	}
	m.passwords = passwords
	return params.ErrorResults{}, nil
}

type mockAgentConfig struct {
	agent.Config
}

func (m *mockAgentConfig) Controller() names.ControllerTag {
	return coretesting.ControllerTag
}

func (m *mockAgentConfig) DataDir() string {
	return "/var/lib/juju"
}

func (m *mockAgentConfig) LogDir() string {
	return "/var/log/juju"
}

func (m *mockAgentConfig) OldPassword() string {
	return "old password"
}

func (m *mockAgentConfig) CACert() string {
	return coretesting.CACert
}

func (m *mockAgentConfig) APIAddresses() ([]string, error) {
	return []string{"10.0.0.1:17070"}, nil
}

type mockBroker struct {
	appName   string
	agentPath string
	config    *caas.OperatorConfig
}

func (m *mockBroker) EnsureOperator(appName, agentPath string, newConfig caas.NewOperatorConfigFunc) error {
	m.appName = appName
	m.agentPath = agentPath
	config, err := newConfig()
	if err != nil {
		return err
	}
	m.config = config
	return nil
}

type mockWatcher struct {
	testing.Stub
	tomb.Tomb
	mu         sync.Mutex
	terminated bool
}

func (w *mockWatcher) doneWhenDying() {
	<-w.Tomb.Dying()
	w.Tomb.Done()
}

func (w *mockWatcher) killed() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.terminated
}

func (w *mockWatcher) Kill() {
	w.MethodCall(w, "Kill")
	w.Tomb.Kill(nil)
	w.mu.Lock()
	defer w.mu.Unlock()
	w.terminated = true
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
	w := &mockStringsWatcher{changes: make(chan []string, 5)}
	go w.doneWhenDying()
	return w
}

func (w *mockStringsWatcher) Changes() watcher.StringsChannel {
	return w.changes
}
