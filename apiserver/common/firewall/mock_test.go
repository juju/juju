// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewall_test

import (
	"context"
	"sync"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/testing"
	"gopkg.in/macaroon.v2"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/apiserver/common/firewall"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/relation"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/state"
)

type mockState struct {
	// TODO - implement when remaining firewaller tests become unit tests
	state.ModelMachinesWatcher

	testing.Stub
	modelUUID      string
	remoteEntities map[names.Tag]string
	macaroons      map[names.Tag]*macaroon.Macaroon
	applications   map[string]*mockApplication
	units          map[string]*mockUnit
	machines       map[string]*mockMachine
	relations      map[string]*mockRelation
	controllerInfo map[string]*mockControllerInfo
	subnetsWatcher *mockStringsWatcher
	modelWatcher   *mockNotifyWatcher
	configAttrs    map[string]interface{}
}

func newMockState(modelUUID string) *mockState {
	return &mockState{
		modelUUID:      modelUUID,
		relations:      make(map[string]*mockRelation),
		applications:   make(map[string]*mockApplication),
		units:          make(map[string]*mockUnit),
		machines:       make(map[string]*mockMachine),
		remoteEntities: make(map[names.Tag]string),
		macaroons:      make(map[names.Tag]*macaroon.Macaroon),
		controllerInfo: make(map[string]*mockControllerInfo),
		subnetsWatcher: newMockStringsWatcher(),
		modelWatcher:   newMockNotifyWatcher(),
		configAttrs:    coretesting.FakeConfig(),
	}
}

func (st *mockState) WatchForModelConfigChanges() state.NotifyWatcher {
	return st.modelWatcher
}

func (st *mockState) ModelConfig(ctx context.Context) (*config.Config, error) {
	return config.New(config.UseDefaults, st.configAttrs)
}

func (st *mockState) ControllerConfig() (controller.Config, error) {
	return nil, errors.NotImplementedf("ControllerConfig")
}

func (st *mockState) ControllerInfo(modelUUID string) ([]string, string, error) {
	if info, ok := st.controllerInfo[modelUUID]; !ok {
		return nil, "", errors.NotFoundf("controller info for %v", modelUUID)
	} else {
		return info.ControllerInfo().Addrs, info.ControllerInfo().CACert, nil
	}
}

func (st *mockState) GetMacaroon(model names.ModelTag, entity names.Tag) (*macaroon.Macaroon, error) {
	st.MethodCall(st, "GetMacaroon", model, entity)
	if err := st.NextErr(); err != nil {
		return nil, err
	}
	mac, ok := st.macaroons[entity]
	if !ok {
		return nil, errors.NotFoundf("macaroon for %v", entity)
	}
	return mac, nil
}

func (st *mockState) ModelUUID() string {
	return st.modelUUID
}

func (st *mockState) Application(id string) (firewall.Application, error) {
	st.MethodCall(st, "Application", id)
	if err := st.NextErr(); err != nil {
		return nil, err
	}
	a, ok := st.applications[id]
	if !ok {
		return nil, errors.NotFoundf("application %q", id)
	}
	return a, nil
}

func (st *mockState) Unit(name string) (firewall.Unit, error) {
	st.MethodCall(st, "Unit", name)
	if err := st.NextErr(); err != nil {
		return nil, err
	}
	u, ok := st.units[name]
	if !ok {
		return nil, errors.NotFoundf("unit %q", name)
	}
	return u, nil
}

func (st *mockState) Machine(id string) (firewall.Machine, error) {
	st.MethodCall(st, "Machine", id)
	if err := st.NextErr(); err != nil {
		return nil, err
	}
	m, ok := st.machines[id]
	if !ok {
		return nil, errors.NotFoundf("machine %q", id)
	}
	return m, nil
}

func (st *mockState) WatchSubnets(func(id interface{}) bool) state.StringsWatcher {
	st.MethodCall(st, "WatchSubnets")
	return st.subnetsWatcher
}

func (st *mockState) WatchOpenedPorts() state.StringsWatcher {
	st.MethodCall(st, "WatchOpenedPorts")
	// TODO - implement when remaining firewaller tests become unit tests
	return nil
}

func (st *mockState) FindEntity(tag names.Tag) (state.Entity, error) {
	st.MethodCall(st, "FindEntity")
	// TODO - implement when remaining firewaller tests become unit tests
	return nil, errors.NotImplementedf("FindEntity")
}

func (st *mockState) GetModel(tag names.ModelTag) (*state.Model, error) {
	st.MethodCall(st, "GetModel", tag)
	// TODO - implement when remaining firewaller tests become unit tests
	return nil, errors.NotImplementedf("GetModel")
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

func (w *mockWatcher) Err() error {
	w.MethodCall(w, "Err")
	return w.Tomb.Err()
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

func newMockNotifyWatcher() *mockNotifyWatcher {
	w := &mockNotifyWatcher{changes: make(chan struct{}, 1)}
	// Initial event
	w.changes <- struct{}{}
	w.Tomb.Go(func() error {
		<-w.Tomb.Dying()
		return nil
	})
	return w
}

type mockNotifyWatcher struct {
	mockWatcher
	changes chan struct{}
}

func (w *mockNotifyWatcher) Changes() <-chan struct{} {
	w.MethodCall(w, "Changes")
	return w.changes
}

type mockApplication struct {
	testing.Stub
	name  string
	units []*mockUnit
}

func newMockApplication(name string) *mockApplication {
	return &mockApplication{
		name: name,
	}
}

func (a *mockApplication) Name() string {
	a.MethodCall(a, "Name")
	return a.name
}

func (a *mockApplication) AllUnits() (results []firewall.Unit, err error) {
	a.MethodCall(a, "AllUnits")
	for _, unit := range a.units {
		results = append(results, unit)
	}
	return results, a.NextErr()
}

type mockControllerInfo struct {
	uuid string
	info crossmodel.ControllerInfo
}

func (c *mockControllerInfo) Id() string {
	return c.uuid
}

func (c *mockControllerInfo) ControllerInfo() crossmodel.ControllerInfo {
	return c.info
}

type mockRelation struct {
	testing.Stub
	firewall.Relation
	id        int
	endpoints []relation.Endpoint
	ruw       *mockRelationUnitsWatcher
	ew        *mockStringsWatcher
	ruwApp    string
}

func newMockRelation(id int) *mockRelation {
	return &mockRelation{
		id:  id,
		ruw: newMockRelationUnitsWatcher(),
		ew:  newMockStringsWatcher(),
	}
}

func (r *mockRelation) Id() int {
	r.MethodCall(r, "Id")
	return r.id
}

func (r *mockRelation) Endpoints() []relation.Endpoint {
	r.MethodCall(r, "Endpoints")
	return r.endpoints
}

func (r *mockRelation) WatchUnits(applicationName string) (state.RelationUnitsWatcher, error) {
	if r.ruwApp != applicationName {
		return nil, errors.Errorf("unexpected app %v", applicationName)
	}
	return r.ruw, nil
}

func (r *mockRelation) WatchRelationEgressNetworks() state.StringsWatcher {
	return r.ew
}

func newMockRelationUnitsWatcher() *mockRelationUnitsWatcher {
	w := &mockRelationUnitsWatcher{changes: make(chan watcher.RelationUnitsChange, 1)}
	w.Tomb.Go(func() error {
		<-w.Tomb.Dying()
		return nil
	})
	return w
}

type mockRelationUnitsWatcher struct {
	mockWatcher
	changes chan watcher.RelationUnitsChange
}

func (w *mockRelationUnitsWatcher) Changes() watcher.RelationUnitsChannel {
	return w.changes
}

func (st *mockState) KeyRelation(key string) (firewall.Relation, error) {
	st.MethodCall(st, "KeyRelation", key)
	if err := st.NextErr(); err != nil {
		return nil, err
	}
	r, ok := st.relations[key]
	if !ok {
		return nil, errors.NotFoundf("relation %q", key)
	}
	return r, nil
}

type mockUnit struct {
	testing.Stub
	mu            sync.Mutex
	name          string
	assigned      bool
	publicAddress network.SpaceAddress
	machineId     string
}

func newMockUnit(name string) *mockUnit {
	return &mockUnit{
		name:     name,
		assigned: true,
	}
}

func (u *mockUnit) Name() string {
	u.MethodCall(u, "Name")
	return u.name
}

func (u *mockUnit) PublicAddress() (network.SpaceAddress, error) {
	u.MethodCall(u, "PublicAddress")
	u.mu.Lock()
	defer u.mu.Unlock()

	if err := u.NextErr(); err != nil {
		return network.SpaceAddress{}, err
	}
	if !u.assigned {
		return network.SpaceAddress{}, errors.NotAssignedf(u.name)
	}
	if u.publicAddress.Value == "" {
		return network.SpaceAddress{}, network.NoAddressError("public")
	}
	return u.publicAddress, nil
}

func (u *mockUnit) AssignedMachineId() (string, error) {
	u.MethodCall(u, "AssignedMachineId")
	if err := u.NextErr(); err != nil {
		return "", err
	}
	if !u.assigned {
		return "", errors.NotAssignedf(u.name)
	}
	return u.machineId, nil
}

func (u *mockUnit) updateAddress(value string) {
	u.mu.Lock()
	defer u.mu.Unlock()

	u.publicAddress = network.NewSpaceAddress(value)
}

type mockMachine struct {
	firewall.Machine

	testing.Stub
	id      string
	watcher *mockAddressWatcher
}

func newMockMachine(id string) *mockMachine {
	return &mockMachine{
		id:      id,
		watcher: newMockAddressWatcher(),
	}
}

func (m *mockMachine) Id() string {
	m.MethodCall(m, "Id")
	return m.id
}

func (m *mockMachine) WatchAddresses() state.NotifyWatcher {
	m.MethodCall(m, "WatchAddresses")
	return m.watcher
}

type mockAddressWatcher struct {
	mockWatcher
	changes chan struct{}
}

func newMockAddressWatcher() *mockAddressWatcher {
	w := &mockAddressWatcher{changes: make(chan struct{}, 1)}
	w.Tomb.Go(func() error {
		<-w.Tomb.Dying()
		return nil
	})
	return w
}

func (w *mockAddressWatcher) Changes() <-chan struct{} {
	w.MethodCall(w, "Changes")
	return w.changes
}
