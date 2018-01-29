// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasunitprovisioner_test

import (
	"sync"
	"time"

	"github.com/juju/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/status"
	coretesting "github.com/juju/juju/testing"
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

type mockServiceBroker struct {
	testing.Stub
	ensured chan<- struct{}
}

func (m *mockServiceBroker) EnsureService(appName string, unitSpec *caas.ContainerSpec, numUnits int, config application.ConfigAttributes) error {
	m.MethodCall(m, "EnsureService", appName, unitSpec, numUnits, config)
	m.ensured <- struct{}{}
	return m.NextErr()
}

func (m *mockServiceBroker) DeleteService(appName string) error {
	m.MethodCall(m, "DeleteService", appName)
	return m.NextErr()
}

type mockContainerBroker struct {
	testing.Stub
	ensured      chan<- struct{}
	unitDeleted  chan<- struct{}
	unitsWatcher *watchertest.MockNotifyWatcher
}

func (m *mockContainerBroker) EnsureUnit(appName, unitName string, spec *caas.ContainerSpec) error {
	m.MethodCall(m, "EnsureUnit", appName, unitName, spec)
	m.ensured <- struct{}{}
	return m.NextErr()
}

func (m *mockContainerBroker) DeleteUnit(unitName string) error {
	m.MethodCall(m, "DeleteUnit", unitName)
	m.unitDeleted <- struct{}{}
	return m.NextErr()
}

func (m *mockContainerBroker) WatchUnits(appName string) (watcher.NotifyWatcher, error) {
	m.MethodCall(m, "WatchUnits", appName)
	return m.unitsWatcher, m.NextErr()
}

func (m *mockContainerBroker) Units(appName string) ([]caas.Unit, error) {
	m.MethodCall(m, "Units", appName)
	return []caas.Unit{
		{
			Id:      "u1",
			Address: "10.0.0.1",
			Status:  status.StatusInfo{Status: status.Allocating},
		},
	}, m.NextErr()
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

func (a *mockApplicationGetter) ApplicationConfig(appName string) (application.ConfigAttributes, error) {
	a.MethodCall(a, "ApplicationConfig", appName)
	return application.ConfigAttributes{"juju-external-hostname": "exthost"}, a.NextErr()
}

type mockContainerSpecGetter struct {
	testing.Stub
	spec          string
	watcher       *watchertest.MockNotifyWatcher
	specRetrieved chan struct{}
}

func (m *mockContainerSpecGetter) setSpec(spec string) {
	m.spec = spec
	m.specRetrieved = make(chan struct{}, 2)
}

func (m *mockContainerSpecGetter) assertSpecRetrieved(c *gc.C) {
	select {
	case <-m.specRetrieved:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for container spec to be retrieved")
	}
}

func (m *mockContainerSpecGetter) ContainerSpec(entityName string) (string, error) {
	m.MethodCall(m, "ContainerSpec", entityName)
	if err := m.NextErr(); err != nil {
		return "", err
	}
	spec := m.spec
	select {
	case m.specRetrieved <- struct{}{}:
	default:
	}
	return spec, nil
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
	mu            sync.Mutex
	life          life.Value
	lifeRetrieved chan struct{}
}

func (m *mockLifeGetter) setLife(life life.Value) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.life = life
	m.lifeRetrieved = make(chan struct{}, 1)
}

func (m *mockLifeGetter) assertLifeRetrieved(c *gc.C) {
	select {
	case <-m.lifeRetrieved:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for life to be retrieved")
	}
}

func (m *mockLifeGetter) Life(entityName string) (life.Value, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.MethodCall(m, "Life", entityName)
	if err := m.NextErr(); err != nil {
		return "", err
	}
	life := m.life
	select {
	case m.lifeRetrieved <- struct{}{}:
	default:
	}
	return life, nil
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

type mockUnitUpdater struct {
	testing.Stub
}

func (m *mockUnitUpdater) UpdateUnits(arg params.UpdateApplicationUnits) error {
	m.MethodCall(m, "UpdateUnits", arg)
	if err := m.NextErr(); err != nil {
		return err
	}
	return nil
}
