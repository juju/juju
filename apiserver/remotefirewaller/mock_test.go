// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remotefirewaller_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	"gopkg.in/juju/names.v2"
	"gopkg.in/tomb.v1"

	"github.com/juju/juju/apiserver/remotefirewaller"
	"github.com/juju/juju/state"
)

type mockState struct {
	testing.Stub
	modelUUID      string
	remoteEntities map[names.Tag]string
	applications   map[string]*mockApplication
	relations      map[string]*mockRelation
	subnets        []remotefirewaller.Subnet
	subnetsWatcher *mockStringsWatcher
}

func newMockState(modelUUID string) *mockState {
	return &mockState{
		modelUUID:      modelUUID,
		relations:      make(map[string]*mockRelation),
		applications:   make(map[string]*mockApplication),
		remoteEntities: make(map[names.Tag]string),
		subnetsWatcher: newMockStringsWatcher(),
	}
}

func (st *mockState) ModelUUID() string {
	return st.modelUUID
}

func (st *mockState) Application(id string) (remotefirewaller.Application, error) {
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

func (st *mockState) WatchSubnets() state.StringsWatcher {
	st.MethodCall(st, "WatchSubnets")
	return st.subnetsWatcher
}

func (st *mockState) AllSubnets() ([]remotefirewaller.Subnet, error) {
	st.MethodCall(st, "AllSubnets")
	return st.subnets, nil
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

type mockApplication struct {
	testing.Stub
	name string
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

type mockRelation struct {
	testing.Stub
	id        int
	key       string
	endpoints []state.Endpoint
}

func newMockRelation(id int) *mockRelation {
	return &mockRelation{
		id: id,
	}
}

func (r *mockRelation) Id() int {
	r.MethodCall(r, "Id")
	return r.id
}

func (r *mockRelation) Endpoints() []state.Endpoint {
	r.MethodCall(r, "Endpoints")
	return r.endpoints
}

type mockSubnet struct {
	cidr string
}

func (a *mockSubnet) CIDR() string {
	return a.cidr
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

func (st *mockState) KeyRelation(key string) (remotefirewaller.Relation, error) {
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
