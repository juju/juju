// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller_test

import (
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	"gopkg.in/macaroon.v2"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/apiserver/common/cloudspec"
	"github.com/juju/juju/apiserver/common/firewall"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/network"
	corefirewall "github.com/juju/juju/core/network/firewall"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
)

type mockCloudSpecAPI struct {
	// TODO - implement when remaining firewaller tests become unit tests
	cloudspec.CloudSpecAPI
}

type mockState struct {
	firewall.State

	testing.Stub
	modelUUID      string
	remoteEntities map[names.Tag]string
	macaroons      map[names.Tag]*macaroon.Macaroon
	relations      map[string]*mockRelation
	machines       map[string]*mockMachine
	controllerInfo map[string]*mockControllerInfo
	firewallRules  map[corefirewall.WellKnownServiceType]*state.FirewallRule
	subnetsWatcher *mockStringsWatcher
	modelWatcher   *mockNotifyWatcher
	configAttrs    map[string]interface{}

	spaceInfos                  network.SpaceInfos
	applicationEndpointBindings map[string]map[string]string
}

func newMockState(modelUUID string) *mockState {
	return &mockState{
		modelUUID:      modelUUID,
		relations:      make(map[string]*mockRelation),
		machines:       make(map[string]*mockMachine),
		remoteEntities: make(map[names.Tag]string),
		macaroons:      make(map[names.Tag]*macaroon.Macaroon),
		controllerInfo: make(map[string]*mockControllerInfo),
		firewallRules:  make(map[corefirewall.WellKnownServiceType]*state.FirewallRule),
		subnetsWatcher: newMockStringsWatcher(),
		modelWatcher:   newMockNotifyWatcher(),
		configAttrs:    coretesting.FakeConfig(),

		applicationEndpointBindings: make(map[string]map[string]string),
	}
}

func (st *mockState) WatchForModelConfigChanges() state.NotifyWatcher {
	return st.modelWatcher
}

func (st *mockState) ModelConfig() (*config.Config, error) {
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

func (st *mockState) GetMacaroon(entity names.Tag) (*macaroon.Macaroon, error) {
	st.MethodCall(st, "GetMacaroon", entity)
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

func (st *mockState) WatchSubnets(func(id interface{}) bool) state.StringsWatcher {
	st.MethodCall(st, "WatchSubnets")
	return st.subnetsWatcher
}

func (st *mockState) Machine(id string) (firewall.Machine, error) {
	st.MethodCall(st, "Machine", id)
	if err := st.NextErr(); err != nil {
		return nil, err
	}
	mach, ok := st.machines[id]
	if !ok {
		return nil, errors.NotFoundf("machine %q", id)
	}
	return mach, nil
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

func (st *mockState) FirewallRule(service corefirewall.WellKnownServiceType) (*state.FirewallRule, error) {
	r, ok := st.firewallRules[service]
	if !ok {
		return nil, errors.NotFoundf("firewall rule for %q", service)
	}
	return r, nil
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
	key       string
	endpoints []state.Endpoint
	ruw       *mockRelationUnitsWatcher
	ruwApp    string
	inScope   set.Strings
	status    status.StatusInfo
}

func newMockRelation(id int) *mockRelation {
	return &mockRelation{
		id:      id,
		ruw:     newMockRelationUnitsWatcher(),
		inScope: make(set.Strings),
	}
}

func (r *mockRelation) Id() int {
	r.MethodCall(r, "Id")
	return r.id
}

func (r *mockRelation) WatchRelationIngressNetworks() state.StringsWatcher {
	w := newMockStringsWatcher()
	w.changes <- []string{"1.2.3.4/32"}
	return w
}

func (r *mockRelation) SetStatus(info status.StatusInfo) error {
	r.status = info
	return nil
}

func newMockRelationUnitsWatcher() *mockRelationUnitsWatcher {
	w := &mockRelationUnitsWatcher{changes: make(chan params.RelationUnitsChange, 1)}
	w.Tomb.Go(func() error {
		<-w.Tomb.Dying()
		return nil
	})
	return w
}

type mockRelationUnitsWatcher struct {
	mockWatcher
	changes chan params.RelationUnitsChange
}

func (w *mockRelationUnitsWatcher) Changes() <-chan params.RelationUnitsChange {
	return w.changes
}

func (st *mockState) GetRemoteEntity(sourceModel names.ModelTag, token string) (names.Tag, error) {
	st.MethodCall(st, "GetRemoteEntity", sourceModel, token)
	if err := st.NextErr(); err != nil {
		return nil, err
	}
	for e, t := range st.remoteEntities {
		if t == token {
			return e, nil
		}
	}
	return nil, errors.NotFoundf("token %v", token)
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

func (st *mockState) AllEndpointBindings() (map[string]map[string]string, error) {
	st.MethodCall(st, "AllEndpointBindings")
	if err := st.NextErr(); err != nil {
		return nil, err
	}
	return st.applicationEndpointBindings, nil
}

func (st *mockState) SpaceInfos() (network.SpaceInfos, error) {
	st.MethodCall(st, "SpaceInfos")
	if err := st.NextErr(); err != nil {
		return nil, err
	}
	return st.spaceInfos, nil
}

type mockMachine struct {
	testing.Stub
	firewall.Machine

	id               string
	openedPortRanges *mockMachinePortRanges
	isManual         bool
}

func newMockMachine(id string) *mockMachine {
	return &mockMachine{
		id: id,
	}
}

func (st *mockMachine) Id() string {
	st.MethodCall(st, "Id")
	return st.id
}

func (st *mockMachine) IsManual() (bool, error) {
	st.MethodCall(st, "IsManual")
	if err := st.NextErr(); err != nil {
		return false, err
	}
	return st.isManual, nil
}

func (st *mockMachine) OpenedPortRanges() (state.MachinePortRanges, error) {
	st.MethodCall(st, "OpenedPortRanges")
	if err := st.NextErr(); err != nil {
		return nil, err
	}
	if st.openedPortRanges == nil {
		return nil, errors.NotFoundf("opened port ranges for machine %q", st.id)
	}
	return st.openedPortRanges, nil
}

type mockMachinePortRanges struct {
	state.MachinePortRanges

	byUnit map[string]*mockUnitPortRanges
}

func newMockMachinePortRanges(unitRanges ...*mockUnitPortRanges) *mockMachinePortRanges {
	byUnit := make(map[string]*mockUnitPortRanges)
	for _, upr := range unitRanges {
		byUnit[upr.unitName] = upr
	}

	return &mockMachinePortRanges{
		byUnit: byUnit,
	}
}

func (st *mockMachinePortRanges) ByUnit() map[string]state.UnitPortRanges {
	out := make(map[string]state.UnitPortRanges)
	for k, v := range st.byUnit {
		out[k] = v
	}

	return out
}

type mockUnitPortRanges struct {
	state.UnitPortRanges

	unitName   string
	byEndpoint network.GroupedPortRanges
}

func newMockUnitPortRanges(unitName string, byEndpoint network.GroupedPortRanges) *mockUnitPortRanges {
	return &mockUnitPortRanges{
		unitName:   unitName,
		byEndpoint: byEndpoint,
	}
}

func (st *mockUnitPortRanges) ByEndpoint() network.GroupedPortRanges {
	return st.byEndpoint
}
