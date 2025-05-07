// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machiner_test

import (
	"context"

	"github.com/juju/names/v6"

	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/machiner"
	"github.com/juju/juju/rpc/params"
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
	testhelpers.Stub
	watcher mockWatcher
	life    life.Value
}

func (m *mockMachine) Refresh(context.Context) error {
	m.MethodCall(m, "Refresh")
	return m.NextErr()
}

func (m *mockMachine) Life() life.Value {
	m.MethodCall(m, "Life")
	return m.life
}

func (m *mockMachine) EnsureDead(context.Context) error {
	m.MethodCall(m, "EnsureDead")
	return m.NextErr()
}

func (m *mockMachine) SetMachineAddresses(_ context.Context, addresses []network.MachineAddress) error {
	m.MethodCall(m, "SetMachineAddresses", addresses)
	return m.NextErr()
}

func (m *mockMachine) SetObservedNetworkConfig(_ context.Context, netConfig []params.NetworkConfig) error {
	m.MethodCall(m, "SetObservedNetworkConfig", netConfig)
	return m.NextErr()
}

func (m *mockMachine) SetStatus(_ context.Context, status status.Status, info string, data map[string]interface{}) error {
	m.MethodCall(m, "SetStatus", status, info, data)
	return m.NextErr()
}

func (m *mockMachine) Watch(_ context.Context) (watcher.NotifyWatcher, error) {
	m.MethodCall(m, "Watch")
	if err := m.NextErr(); err != nil {
		return nil, err
	}
	return &m.watcher, nil
}

type mockMachineAccessor struct {
	testhelpers.Stub
	machine mockMachine
}

func (a *mockMachineAccessor) Machine(_ context.Context, tag names.MachineTag) (machiner.Machine, error) {
	a.MethodCall(a, "Machine", tag)
	if err := a.NextErr(); err != nil {
		return nil, err
	}
	return &a.machine, nil
}
