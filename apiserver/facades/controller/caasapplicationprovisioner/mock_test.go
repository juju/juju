// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasapplicationprovisioner_test

import (
	"github.com/juju/charm/v8"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facades/controller/caasapplicationprovisioner"
	k8sconstants "github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/poolmanager"
	coretesting "github.com/juju/juju/testing"
)

type mockState struct {
	testing.Stub

	common.AddressAndCertGetter
	model              *mockModel
	applicationWatcher *mockStringsWatcher
	app                *mockApplication
	operatorRepo       string
}

func newMockState() *mockState {
	st := &mockState{
		applicationWatcher: newMockStringsWatcher(),
	}
	st.model = &mockModel{state: st}
	return st
}

func (st *mockState) WatchApplications() state.StringsWatcher {
	st.MethodCall(st, "WatchApplications")
	return st.applicationWatcher
}

func (st *mockState) ControllerConfig() (controller.Config, error) {
	cfg := coretesting.FakeControllerConfig()
	cfg[controller.CAASImageRepo] = st.operatorRepo
	return cfg, nil
}

func (st *mockState) APIHostPortsForAgents() ([]network.SpaceHostPorts, error) {
	st.MethodCall(st, "APIHostPortsForAgents")
	return []network.SpaceHostPorts{
		network.NewSpaceHostPorts(1, "10.0.0.1"),
	}, nil
}

func (st *mockState) Application(appName string) (caasapplicationprovisioner.Application, error) {
	st.MethodCall(st, "Application", appName)
	if appName != "gitlab" {
		return nil, errors.NotFoundf("app %v", appName)
	}
	return st.app, nil
}

func (st *mockState) Model() (caasapplicationprovisioner.Model, error) {
	st.MethodCall(st, "Model")
	if err := st.NextErr(); err != nil {
		return nil, err
	}
	return st.model, nil
}

func (st *mockState) StateServingInfo() (controller.StateServingInfo, error) {
	st.MethodCall(st, "StateServingInfo")
	if err := st.NextErr(); err != nil {
		return controller.StateServingInfo{}, err
	}
	return controller.StateServingInfo{
		CAPrivateKey: coretesting.CAKey,
	}, nil
}

func (st *mockState) ResolveConstraints(cons constraints.Value) (constraints.Value, error) {
	st.MethodCall(st, "ResolveConstraints", cons)
	if err := st.NextErr(); err != nil {
		return constraints.Value{}, err
	}
	return cons, nil
}

type mockStorageRegistry struct {
	storage.ProviderRegistry
}

func (m *mockStorageRegistry) StorageProvider(p storage.ProviderType) (storage.Provider, error) {
	return nil, errors.NotFoundf("provider %q", p)
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
	return storage.NewConfig(name, k8sconstants.StorageProviderType, map[string]interface{}{"foo": "bar"})
}

type mockModel struct {
	testing.Stub
	state *mockState
}

func (m *mockModel) UUID() string {
	m.MethodCall(m, "UUID")
	return coretesting.ModelTag.Id()
}

func (m *mockModel) ModelConfig() (*config.Config, error) {
	m.MethodCall(m, "ModelConfig")
	attrs := coretesting.FakeConfig()
	attrs["operator-storage"] = "k8s-storage"
	attrs["agent-version"] = "2.6-beta3"
	return config.New(config.UseDefaults, attrs)
}

func (m *mockModel) Containers(providerIds ...string) ([]state.CloudContainer, error) {
	m.MethodCall(m, "Containers", providerIds)
	if err := m.NextErr(); err != nil {
		return nil, err
	}

	providerIdMap := map[string]struct{}{}
	for _, v := range providerIds {
		providerIdMap[v] = struct{}{}
	}

	containers := []state.CloudContainer(nil)
	for _, u := range m.state.app.units {
		if u.containerInfo == nil {
			continue
		}
		if _, ok := providerIdMap[u.containerInfo.providerId]; !ok {
			continue
		}
		containers = append(containers, u.containerInfo)
	}

	return containers, nil
}

type mockApplication struct {
	testing.Stub
	state.Authenticator
	life               state.Life
	tag                names.Tag
	password           string
	charm              caasapplicationprovisioner.Charm
	units              []*mockUnit
	constraints        constraints.Value
	storageConstraints map[string]state.StorageConstraints
	deviceConstraints  map[string]state.DeviceConstraints
}

func (a *mockApplication) Tag() names.Tag {
	a.MethodCall(a, "Tag")
	return a.tag
}

func (a *mockApplication) SetPassword(password string) error {
	a.MethodCall(a, "SetPassword", password)
	if err := a.NextErr(); err != nil {
		return err
	}
	a.password = password
	return nil
}

func (a *mockApplication) Life() state.Life {
	a.MethodCall(a, "Life")
	return a.life
}

func (a *mockApplication) Charm() (caasapplicationprovisioner.Charm, bool, error) {
	a.MethodCall(a, "Charm")
	if err := a.NextErr(); err != nil {
		return nil, false, err
	}
	return a.charm, false, nil
}

func (a *mockApplication) AllUnits() ([]caasapplicationprovisioner.Unit, error) {
	a.MethodCall(a, "AllUnits")
	if err := a.NextErr(); err != nil {
		return nil, err
	}
	units := []caasapplicationprovisioner.Unit(nil)
	for _, u := range a.units {
		units = append(units, u)
	}
	return units, nil
}

func (a *mockApplication) Constraints() (constraints.Value, error) {
	a.MethodCall(a, "Constraints")
	if err := a.NextErr(); err != nil {
		return constraints.Value{}, err
	}
	return a.constraints, nil
}

func (a *mockApplication) UpdateUnits(unitsOp *state.UpdateUnitsOperation) error {
	a.MethodCall(a, "UpdateUnits", unitsOp)
	return a.NextErr()
}

func (a *mockApplication) StorageConstraints() (map[string]state.StorageConstraints, error) {
	a.MethodCall(a, "StorageConstraints")
	if err := a.NextErr(); err != nil {
		return nil, err
	}
	return a.storageConstraints, nil
}

func (a *mockApplication) DeviceConstraints() (map[string]state.DeviceConstraints, error) {
	a.MethodCall(a, "DeviceConstraints")
	if err := a.NextErr(); err != nil {
		return nil, err
	}
	return a.deviceConstraints, nil
}

func (a *mockApplication) Name() string {
	a.MethodCall(a, "Name")
	return a.tag.Id()
}

func (a *mockApplication) SetOperatorStatus(statusInfo status.StatusInfo) error {
	a.MethodCall(a, "SetOperatorStatus", statusInfo)
	return a.NextErr()
}

type mockCharm struct {
	meta *charm.Meta
	url  *charm.URL
}

func (ch *mockCharm) Meta() *charm.Meta {
	return ch.meta
}

func (ch *mockCharm) URL() *charm.URL {
	return ch.url
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

type mockUnit struct {
	testing.Stub
	life          state.Life
	destroyOp     *state.DestroyUnitOperation
	containerInfo *mockCloudContainer
	tag           names.Tag
}

func (u *mockUnit) Tag() names.Tag {
	return u.tag
}

func (u *mockUnit) DestroyOperation() *state.DestroyUnitOperation {
	u.MethodCall(u, "DestroyOperation")
	return u.destroyOp
}

func (u *mockUnit) EnsureDead() error {
	u.MethodCall(u, "EnsureDead")
	return u.NextErr()
}

func (u *mockUnit) ContainerInfo() (state.CloudContainer, error) {
	u.MethodCall(u, "ContainerInfo")
	return u.containerInfo, u.NextErr()
}

type mockCloudContainer struct {
	testing.Stub
	unit       string
	providerId string
}

func (c *mockCloudContainer) Unit() string {
	return c.unit
}

func (c *mockCloudContainer) ProviderId() string {
	return c.providerId
}

func (c *mockCloudContainer) Address() *network.SpaceAddress {
	return nil
}

func (c *mockCloudContainer) Ports() []string {
	return nil
}
