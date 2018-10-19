// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasunitprovisioner_test

import (
	"sync"
	"time"

	"github.com/juju/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base"
	apicaasunitprovisioner "github.com/juju/juju/api/caasunitprovisioner"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/network"
	coretesting "github.com/juju/juju/testing"
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
	caas.ContainerEnvironProvider
	ensured chan<- struct{}
	deleted chan<- struct{}
	podSpec *caas.PodSpec
}

func (m *mockServiceBroker) Provider() caas.ContainerEnvironProvider {
	return m
}

func (m *mockServiceBroker) ParsePodSpec(in string) (*caas.PodSpec, error) {
	return m.podSpec, nil
}

func (m *mockServiceBroker) EnsureService(appName string, statusCallback caas.StatusCallbackFunc, params *caas.ServiceParams, numUnits int, config application.ConfigAttributes) error {
	m.MethodCall(m, "EnsureService", appName, params, numUnits, config)
	statusCallback(appName, status.Waiting, "ensuring", map[string]interface{}{"foo": "bar"})
	m.ensured <- struct{}{}
	return m.NextErr()
}

func (m *mockServiceBroker) EnsureCustomResourceDefinition(appName string, podSpec *caas.PodSpec) error {
	m.MethodCall(m, "EnsureCustomResourceDefinition", appName, podSpec)
	return m.NextErr()
}

func (m *mockServiceBroker) Service(appName string) (*caas.Service, error) {
	m.MethodCall(m, "Service", appName)
	return &caas.Service{Id: "id", Addresses: []network.Address{{Value: "10.0.0.1"}}}, m.NextErr()
}

func (m *mockServiceBroker) DeleteService(appName string) error {
	m.MethodCall(m, "DeleteService", appName)
	m.deleted <- struct{}{}
	return m.NextErr()
}

func (m *mockServiceBroker) UnexposeService(appName string) error {
	m.MethodCall(m, "UnexposeService", appName)
	return m.NextErr()
}

type mockContainerBroker struct {
	testing.Stub
	caas.ContainerEnvironProvider
	unitsWatcher           *watchertest.MockNotifyWatcher
	operatorWatcher        *watchertest.MockNotifyWatcher
	reportedUnitStatus     status.Status
	reportedOperatorStatus status.Status
	podSpec                *caas.PodSpec
}

func (m *mockContainerBroker) Provider() caas.ContainerEnvironProvider {
	return m
}

func (m *mockContainerBroker) ParsePodSpec(in string) (*caas.PodSpec, error) {
	return m.podSpec, nil
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
				Status:  status.StatusInfo{Status: m.reportedUnitStatus},
				FilesystemInfo: []caas.FilesystemInfo{
					{MountPoint: "/path-to-here", ReadOnly: true, StorageName: "database",
						Size: 100, FilesystemId: "fs-id",
						Status: status.StatusInfo{Status: status.Attaching, Message: "not ready"},
						Volume: caas.VolumeInfo{VolumeId: "vol-id", Size: 200, Persistent: true,
							Status: status.StatusInfo{Status: status.Error, Message: "vol not ready"}},
					},
				},
			},
		},
		m.NextErr()
}

func (m *mockContainerBroker) Operator(appName string) (*caas.Operator, error) {
	m.MethodCall(m, "Operator", appName)
	return &caas.Operator{
		Dying: false,
		Status: status.StatusInfo{
			Status:  m.reportedOperatorStatus,
			Message: "testing 1. 2. 3.",
			Data:    map[string]interface{}{"zip": "zap"},
		},
	}, nil
}

func (m *mockContainerBroker) WatchOperator(appName string) (watcher.NotifyWatcher, error) {
	m.MethodCall(m, "WatchOperator", appName)
	return m.operatorWatcher, m.NextErr()
}

type mockApplicationGetter struct {
	testing.Stub
	watcher      *watchertest.MockStringsWatcher
	scaleWatcher *watchertest.MockNotifyWatcher
	scale        int
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
	return application.ConfigAttributes{
		"juju-external-hostname": "exthost",
	}, a.NextErr()
}

func (a *mockApplicationGetter) WatchApplicationScale(application string) (watcher.NotifyWatcher, error) {
	a.MethodCall(a, "WatchApplicationScale", application)
	if err := a.NextErr(); err != nil {
		return nil, err
	}
	return a.scaleWatcher, nil
}

func (a *mockApplicationGetter) ApplicationScale(application string) (int, error) {
	a.MethodCall(a, "ApplicationScale", application)
	if err := a.NextErr(); err != nil {
		return 0, err
	}
	return a.scale, nil
}

type mockApplicationUpdater struct {
	testing.Stub
	updated chan<- struct{}
}

func (m *mockApplicationUpdater) UpdateApplicationService(arg params.UpdateApplicationServiceArg) error {
	m.MethodCall(m, "UpdateApplicationService", arg)
	m.updated <- struct{}{}
	return m.NextErr()
}

type mockProvisioningInfoGetterGetter struct {
	testing.Stub
	provisioningInfo apicaasunitprovisioner.ProvisioningInfo
	watcher          *watchertest.MockNotifyWatcher
	specRetrieved    chan struct{}
}

func (m *mockProvisioningInfoGetterGetter) setProvisioningInfo(provisioningInfo apicaasunitprovisioner.ProvisioningInfo) {
	m.provisioningInfo = provisioningInfo
	m.specRetrieved = make(chan struct{}, 2)
}

func (m *mockProvisioningInfoGetterGetter) assertSpecRetrieved(c *gc.C) {
	select {
	case <-m.specRetrieved:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for pod spec to be retrieved")
	}
}

func (m *mockProvisioningInfoGetterGetter) ProvisioningInfo(appName string) (*apicaasunitprovisioner.ProvisioningInfo, error) {
	m.MethodCall(m, "ProvisioningInfo", appName)
	if err := m.NextErr(); err != nil {
		return nil, err
	}
	provisioningInfo := m.provisioningInfo
	select {
	case m.specRetrieved <- struct{}{}:
	default:
	}
	return &provisioningInfo, nil
}

func (m *mockProvisioningInfoGetterGetter) WatchPodSpec(appName string) (watcher.NotifyWatcher, error) {
	m.MethodCall(m, "WatchPodSpec", appName)
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

type mockProvisioningStatusSetter struct {
	testing.Stub
}

func (m *mockProvisioningStatusSetter) SetOperatorStatus(appName string, status status.Status, message string, data map[string]interface{}) error {
	m.MethodCall(m, "SetOperatorStatus", appName, status, message, data)
	if err := m.NextErr(); err != nil {
		return err
	}
	return nil
}
