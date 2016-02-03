// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machiner_test

import (
	"github.com/juju/names"
	gitjujutesting "github.com/juju/testing"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/network"
	"github.com/juju/juju/watcher"
	"github.com/juju/juju/worker/machiner"
)

type mockWatcher struct {
	changes chan struct{}
}

func (w *mockWatcher) Changes() watcher.NotifyChannel {
	return w.changes
}

func (w *mockWatcher) Kill() {}

func (w *mockWatcher) Wait() error {
	return nil
}

type mockMachine struct {
	machiner.Machine
	gitjujutesting.Stub
	watcher mockWatcher
	life    params.Life
}

func (m *mockMachine) Refresh() error {
	m.MethodCall(m, "Refresh")
	return m.NextErr()
}

func (m *mockMachine) Life() params.Life {
	m.MethodCall(m, "Life")
	return m.life
}

func (m *mockMachine) EnsureDead() error {
	m.MethodCall(m, "EnsureDead")
	return m.NextErr()
}

func (m *mockMachine) SetMachineAddresses(addresses []network.Address) error {
	m.MethodCall(m, "SetMachineAddresses", addresses)
	return m.NextErr()
}

func (m *mockMachine) SetStatus(status params.Status, info string, data map[string]interface{}) error {
	m.MethodCall(m, "SetStatus", status, info, data)
	return m.NextErr()
}

func (m *mockMachine) Watch() (watcher.NotifyWatcher, error) {
	m.MethodCall(m, "Watch")
	if err := m.NextErr(); err != nil {
		return nil, err
	}
	return &m.watcher, nil
}

type mockMachineAccessor struct {
	gitjujutesting.Stub
	machine mockMachine
}

func (a *mockMachineAccessor) Machine(tag names.MachineTag) (machiner.Machine, error) {
	a.MethodCall(a, "Machine", tag)
	if err := a.NextErr(); err != nil {
		return nil, err
	}
	return &a.machine, nil
}
