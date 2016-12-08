// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoterelations_test

import (
	"sync"

	"github.com/juju/errors"
	"github.com/juju/testing"
	"gopkg.in/juju/names.v2"
	"gopkg.in/tomb.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/watcher"
	"github.com/juju/juju/worker/remoterelations"
)

type mockRelationsFacade struct {
	mu   sync.Mutex
	stub *testing.Stub
	remoterelations.RemoteRelationsFacade
	remoteApplicationsWatcher          *mockStringsWatcher
	remoteApplicationRelationsWatchers map[string]*mockStringsWatcher
	remoteApplications                 map[string]*mockRemoteApplication
	relations                          map[string]*mockRelation
	relationsEndpoints                 map[string]*relationEndpointInfo
	relationsUnitsWatchers             map[string]*mockRelationUnitsWatcher
}

func newMockRelationsFacade(stub *testing.Stub) *mockRelationsFacade {
	return &mockRelationsFacade{
		stub:                               stub,
		remoteApplications:                 make(map[string]*mockRemoteApplication),
		relations:                          make(map[string]*mockRelation),
		relationsEndpoints:                 make(map[string]*relationEndpointInfo),
		remoteApplicationsWatcher:          newMockStringsWatcher(),
		remoteApplicationRelationsWatchers: make(map[string]*mockStringsWatcher),
		relationsUnitsWatchers:             make(map[string]*mockRelationUnitsWatcher),
	}
}

func (m *mockRelationsFacade) Close() error {
	m.stub.MethodCall(m, "Close")
	return nil
}

func (m *mockRelationsFacade) WatchRemoteApplications() (watcher.StringsWatcher, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stub.MethodCall(m, "WatchRemoteApplications")
	if err := m.stub.NextErr(); err != nil {
		return nil, err
	}
	return m.remoteApplicationsWatcher, nil
}

func (m *mockRelationsFacade) remoteApplicationRelationsWatcher(name string) (*mockStringsWatcher, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	w, ok := m.remoteApplicationRelationsWatchers[name]
	return w, ok
}

func (m *mockRelationsFacade) removeApplication(name string) (*mockStringsWatcher, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	w, ok := m.remoteApplicationRelationsWatchers[name]
	delete(m.remoteApplications, name)
	return w, ok
}

func (m *mockRelationsFacade) relationsUnitsWatcher(key string) (*mockRelationUnitsWatcher, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	w, ok := m.relationsUnitsWatchers[key]
	return w, ok
}

func (m *mockRelationsFacade) removeRelation(key string) (*mockRelationUnitsWatcher, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	w, ok := m.relationsUnitsWatchers[key]
	delete(m.relations, key)
	return w, ok
}

func (m *mockRelationsFacade) updateRelationLife(key string, life params.Life) (*mockRelationUnitsWatcher, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	w, ok := m.relationsUnitsWatchers[key]
	m.relations[key].life = life
	return w, ok
}

func (m *mockRelationsFacade) WatchRemoteApplicationRelations(application string) (watcher.StringsWatcher, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stub.MethodCall(m, "WatchRemoteApplicationRelations", application)
	if err := m.stub.NextErr(); err != nil {
		return nil, err
	}
	m.remoteApplicationRelationsWatchers[application] = newMockStringsWatcher()
	return m.remoteApplicationRelationsWatchers[application], nil
}

func (m *mockRelationsFacade) ExportEntities(entities []names.Tag) ([]params.RemoteEntityIdResult, error) {
	m.stub.MethodCall(m, "ExportEntities", entities)
	if err := m.stub.NextErr(); err != nil {
		return nil, err
	}
	result := make([]params.RemoteEntityIdResult, len(entities))
	for i, e := range entities {
		result[i] = params.RemoteEntityIdResult{
			Result: &params.RemoteEntityId{
				ModelUUID: "model-uuid",
				Token:     "token-" + e.Id(),
			},
		}
	}
	return result, nil
}

func (m *mockRelationsFacade) GetToken(modelUUID string, entity names.Tag) (string, error) {
	m.stub.MethodCall(m, "GetToken", modelUUID, entity)
	if err := m.stub.NextErr(); err != nil {
		return "", err
	}
	return "token-" + entity.Id(), nil
}

func (m *mockRelationsFacade) RelationUnitSettings(relationUnits []params.RelationUnit) ([]params.SettingsResult, error) {
	m.stub.MethodCall(m, "RelationUnitSettings", relationUnits)
	if err := m.stub.NextErr(); err != nil {
		return nil, err
	}
	result := make([]params.SettingsResult, len(relationUnits))
	for i := range relationUnits {
		result[i].Settings = map[string]string{
			"foo": "bar",
		}
	}
	return result, nil
}

func (m *mockRelationsFacade) PublishLocalRelationChange(change params.RemoteRelationChangeEvent) error {
	m.stub.MethodCall(m, "PublishLocalRelationChange", change)
	if err := m.stub.NextErr(); err != nil {
		return err
	}
	return nil
}

func (m *mockRelationsFacade) RegisterRemoteRelation(rel params.RegisterRemoteRelation) error {
	m.stub.MethodCall(m, "RegisterRemoteRelation", rel)
	if err := m.stub.NextErr(); err != nil {
		return err
	}
	return nil
}

func (m *mockRelationsFacade) RemoteApplications(names []string) ([]params.RemoteApplicationResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stub.MethodCall(m, "RemoteApplications", names)
	if err := m.stub.NextErr(); err != nil {
		return nil, err
	}
	result := make([]params.RemoteApplicationResult, len(names))
	for i, name := range names {
		if app, ok := m.remoteApplications[name]; ok {
			result[i] = params.RemoteApplicationResult{
				Result: &params.RemoteApplication{
					Name:       app.name,
					Life:       app.life,
					Status:     app.status,
					ModelUUID:  app.modelUUID,
					Registered: app.registered,
				},
			}
		} else {
			result[i] = params.RemoteApplicationResult{
				Error: common.ServerError(errors.NotFoundf(name))}
		}
	}
	return result, nil
}

type relationEndpointInfo struct {
	localApplicationName string
	localEndpoint        params.RemoteEndpoint
	remoteEndpointName   string
}

func (m *mockRelationsFacade) Relations(keys []string) ([]params.RemoteRelationResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stub.MethodCall(m, "Relations", keys)
	if err := m.stub.NextErr(); err != nil {
		return nil, err
	}
	result := make([]params.RemoteRelationResult, len(keys))
	for i, key := range keys {
		if rel, ok := m.relations[key]; ok {
			result[i].Result = &params.RemoteRelation{
				Id:   rel.id,
				Life: rel.life,
				Key:  keys[i],
			}
			if epInfo, ok := m.relationsEndpoints[key]; ok {
				result[i].Result.RemoteEndpointName = epInfo.remoteEndpointName
				result[i].Result.LocalEndpoint = epInfo.localEndpoint
			}
		} else {
			result[i].Error = common.ServerError(errors.NotFoundf(key))
		}
	}
	return result, nil
}

func (m *mockRelationsFacade) WatchLocalRelationUnits(relationKey string) (watcher.RelationUnitsWatcher, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stub.MethodCall(m, "WatchLocalRelationUnits", relationKey)
	if err := m.stub.NextErr(); err != nil {
		return nil, err
	}
	m.relationsUnitsWatchers[relationKey] = newMockRelationUnitsWatcher()
	return m.relationsUnitsWatchers[relationKey], nil
}

type mockWatcher struct {
	testing.Stub
	tomb.Tomb
	mu         sync.Mutex
	terminated bool
}

func (w *mockWatcher) doneWhenDying() {
	<-w.Tomb.Dying()
	w.Tomb.Done()
}

func (w *mockWatcher) killed() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.terminated
}

func (w *mockWatcher) Kill() {
	w.MethodCall(w, "Kill")
	w.Tomb.Kill(nil)
	w.mu.Lock()
	defer w.mu.Unlock()
	w.terminated = true
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
	w := &mockStringsWatcher{changes: make(chan []string, 5)}
	go w.doneWhenDying()
	return w
}

func (w *mockStringsWatcher) Changes() watcher.StringsChannel {
	w.MethodCall(w, "Changes")
	return w.changes
}

type mockRemoteApplication struct {
	testing.Stub
	name       string
	url        string
	life       params.Life
	status     string
	modelUUID  string
	registered bool
}

type mockRelationUnitsWatcher struct {
	mockWatcher
	changes chan watcher.RelationUnitsChange
}

func newMockRelationUnitsWatcher() *mockRelationUnitsWatcher {
	w := &mockRelationUnitsWatcher{
		changes: make(chan watcher.RelationUnitsChange, 1),
	}
	go w.doneWhenDying()
	return w
}

func (w *mockRelationUnitsWatcher) Changes() watcher.RelationUnitsChannel {
	w.MethodCall(w, "Changes")
	return w.changes
}

func newMockRemoteApplication(name, url string) *mockRemoteApplication {
	return &mockRemoteApplication{
		name: name, url: url, life: params.Alive,
		modelUUID: "remote-model-uuid",
	}
}

func (r *mockRemoteApplication) Name() string {
	r.MethodCall(r, "Name")
	return r.name
}

func (r *mockRemoteApplication) SourceModel() names.ModelTag {
	r.MethodCall(r, "SourceModel")
	return names.NewModelTag(r.modelUUID)
}

func (r *mockRemoteApplication) Life() params.Life {
	r.MethodCall(r, "Life")
	return r.life
}

type mockRelation struct {
	testing.Stub
	id   int
	life params.Life
}

func newMockRelation(id int) *mockRelation {
	return &mockRelation{
		id:   id,
		life: params.Alive,
	}
}

func (r *mockRelation) Id() int {
	r.MethodCall(r, "Id")
	return r.id
}

func (r *mockRelation) Life() params.Life {
	r.MethodCall(r, "Life")
	return r.life
}
