// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoterelations_test

import (
	"launchpad.net/tomb"

	"github.com/juju/errors"
	"github.com/juju/juju/apiserver/remoterelations"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/testing"
)

type mockState struct {
	testing.Stub
	relations                map[string]*mockRelation
	remoteServices           map[string]*mockRemoteService
	remoteServicesWatcher    *mockStringsWatcher
	serviceRelationsWatchers map[string]*mockStringsWatcher
}

func newMockState() *mockState {
	return &mockState{
		relations:                make(map[string]*mockRelation),
		remoteServices:           make(map[string]*mockRemoteService),
		remoteServicesWatcher:    newMockStringsWatcher(),
		serviceRelationsWatchers: make(map[string]*mockStringsWatcher),
	}
}

func (st *mockState) KeyRelation(key string) (remoterelations.Relation, error) {
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

func (st *mockState) Relation(id int) (remoterelations.Relation, error) {
	st.MethodCall(st, "Relation", id)
	if err := st.NextErr(); err != nil {
		return nil, err
	}
	for _, r := range st.relations {
		if r.Id() == id {
			return r, nil
		}
	}
	return nil, errors.NotFoundf("relation %d", id)
}

func (st *mockState) RemoteService(id string) (remoterelations.RemoteService, error) {
	st.MethodCall(st, "RemoteService", id)
	if err := st.NextErr(); err != nil {
		return nil, err
	}
	s, ok := st.remoteServices[id]
	if !ok {
		return nil, errors.NotFoundf("remote service %q", id)
	}
	return s, nil
}

func (st *mockState) WatchRemoteServices() state.StringsWatcher {
	st.MethodCall(st, "WatchRemoteServices")
	return st.remoteServicesWatcher
}

func (st *mockState) WatchRemoteServiceRelations(serviceName string) (state.StringsWatcher, error) {
	st.MethodCall(st, "WatchRemoteServiceRelations", serviceName)
	if err := st.NextErr(); err != nil {
		return nil, err
	}
	w, ok := st.serviceRelationsWatchers[serviceName]
	if !ok {
		return nil, errors.NotFoundf("service %q", serviceName)
	}
	return w, nil
}

type mockRelation struct {
	testing.Stub
	id                    int
	life                  state.Life
	units                 map[string]remoterelations.RelationUnit
	endpointUnitsWatchers map[string]*mockRelationUnitsWatcher
}

func newMockRelation(id int) *mockRelation {
	return &mockRelation{
		id:    id,
		life:  state.Alive,
		units: make(map[string]remoterelations.RelationUnit),
		endpointUnitsWatchers: make(map[string]*mockRelationUnitsWatcher),
	}
}

func (r *mockRelation) Id() int {
	r.MethodCall(r, "Id")
	return r.id
}

func (r *mockRelation) Life() state.Life {
	r.MethodCall(r, "Life")
	return r.life
}

func (r *mockRelation) Destroy() error {
	r.MethodCall(r, "Destroy")
	return r.NextErr()
}

func (r *mockRelation) RemoteUnit(unitId string) (remoterelations.RelationUnit, error) {
	r.MethodCall(r, "RemoteUnit", unitId)
	if err := r.NextErr(); err != nil {
		return nil, err
	}
	u, ok := r.units[unitId]
	if !ok {
		return nil, errors.NotFoundf("unit %q", unitId)
	}
	return u, nil
}

func (r *mockRelation) Unit(unitId string) (remoterelations.RelationUnit, error) {
	r.MethodCall(r, "Unit", unitId)
	if err := r.NextErr(); err != nil {
		return nil, err
	}
	u, ok := r.units[unitId]
	if !ok {
		return nil, errors.NotFoundf("unit %q", unitId)
	}
	return u, nil
}

func (r *mockRelation) WatchCounterpartEndpointUnits(serviceName string) (state.RelationUnitsWatcher, error) {
	r.MethodCall(r, "WatchCounterpartEndpointUnits", serviceName)
	if err := r.NextErr(); err != nil {
		return nil, err
	}
	w, ok := r.endpointUnitsWatchers[serviceName]
	if !ok {
		return nil, errors.NotFoundf("service %q", serviceName)
	}
	return w, nil
}

type mockRemoteService struct {
	testing.Stub
	name string
	url  string
}

func newMockRemoteService(name, url string) *mockRemoteService {
	return &mockRemoteService{
		name: name, url: url,
	}
}

func (r *mockRemoteService) Name() string {
	return r.name
}

func (r *mockRemoteService) URL() string {
	return r.url
}

func (r *mockRemoteService) Destroy() error {
	r.MethodCall(r, "Destroy")
	return r.NextErr()
}

type mockWatcher struct {
	testing.Stub
	tomb.Tomb
}

func (w *mockWatcher) doneWhenDying() {
	<-w.Tomb.Dying()
	w.Tomb.Done()
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
	go w.doneWhenDying()
	return w
}

func (w *mockStringsWatcher) Changes() <-chan []string {
	w.MethodCall(w, "Changes")
	return w.changes
}

type mockRelationUnitsWatcher struct {
	mockWatcher
	changes chan multiwatcher.RelationUnitsChange
}

func newMockRelationUnitsWatcher() *mockRelationUnitsWatcher {
	w := &mockRelationUnitsWatcher{
		changes: make(chan multiwatcher.RelationUnitsChange, 1),
	}
	go w.doneWhenDying()
	return w
}

func (w *mockRelationUnitsWatcher) Changes() <-chan multiwatcher.RelationUnitsChange {
	w.MethodCall(w, "Changes")
	return w.changes
}

type mockRelationUnit struct {
	testing.Stub
	inScope  bool
	settings map[string]interface{}
}

func newMockRelationUnit() *mockRelationUnit {
	return &mockRelationUnit{
		settings: make(map[string]interface{}),
	}
}

func (u *mockRelationUnit) Settings() (map[string]interface{}, error) {
	u.MethodCall(u, "Settings")
	return u.settings, u.NextErr()
}

func (u *mockRelationUnit) InScope() (bool, error) {
	u.MethodCall(u, "InScope")
	return u.inScope, u.NextErr()
}

func (u *mockRelationUnit) LeaveScope() error {
	u.MethodCall(u, "LeaveScope")
	if err := u.NextErr(); err != nil {
		return err
	}
	u.inScope = false
	return nil
}

func (u *mockRelationUnit) EnterScope(settings map[string]interface{}) error {
	u.MethodCall(u, "EnterScope", settings)
	if err := u.NextErr(); err != nil {
		return err
	}
	u.inScope = true
	u.settings = make(map[string]interface{})
	for k, v := range settings {
		u.settings[k] = v
	}
	return nil
}

func (u *mockRelationUnit) ReplaceSettings(settings map[string]interface{}) error {
	u.MethodCall(u, "ReplaceSettings", settings)
	if err := u.NextErr(); err != nil {
		return err
	}
	u.settings = make(map[string]interface{})
	for k, v := range settings {
		u.settings[k] = v
	}
	return nil
}
