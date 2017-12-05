// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasunitprovisioner_test

import (
	"github.com/juju/testing"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/watcher"
	"github.com/juju/juju/watcher/watchertest"
	"github.com/juju/juju/worker/caasunitprovisioner"
)

type fakeAPICaller struct {
	base.APICaller
}

type fakeBroker struct {
	caas.Broker
}

type fakeClient struct {
	caasunitprovisioner.Client
}

type mockContainerBroker struct {
	testing.Stub
}

type mockApplicationGetter struct {
	testing.Stub
	watcher *watchertest.MockStringsWatcher
}

func (m *mockApplicationGetter) WatchApplications() (watcher.StringsWatcher, error) {
	m.MethodCall(m, "WatchApplications")
	if err := m.NextErr(); err != nil {
		return nil, err
	}
	return m.watcher, nil
}

type mockContainerSpecGetter struct {
	testing.Stub
	spec    string
	watcher *watchertest.MockNotifyWatcher
}

func (m *mockContainerSpecGetter) ContainerSpec(entityName string) (string, error) {
	m.MethodCall(m, "ContainerSpec", entityName)
	if err := m.NextErr(); err != nil {
		return "", err
	}
	return m.spec, nil
}

func (m *mockContainerSpecGetter) WatchContainerSpec(entityName string) (watcher.NotifyWatcher, error) {
	m.MethodCall(m, "WatchContainerSpec", entityName)
	if err := m.NextErr(); err != nil {
		return nil, err
	}
	return m.watcher, nil
}

type mockLifeGetter struct {
	testing.Stub
	life life.Value
}

func (m *mockLifeGetter) Life(entityName string) (life.Value, error) {
	m.MethodCall(m, "Life", entityName)
	if err := m.NextErr(); err != nil {
		return "", err
	}
	return m.life, nil
}

type mockUnitGetter struct {
	testing.Stub
	watcher *watchertest.MockStringsWatcher
}

func (m *mockUnitGetter) WatchUnits(application string) (watcher.StringsWatcher, error) {
	m.MethodCall(m, "WatchUnits", application)
	if err := m.NextErr(); err != nil {
		return nil, err
	}
	return m.watcher, nil
}
