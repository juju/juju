// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoterelations_test

import (
	"sync"

	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	"gopkg.in/macaroon.v2"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/api"
	apitesting "github.com/juju/juju/api/testing"
	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
)

type mockRelationsFacade struct {
	mu                                 sync.Mutex
	stub                               *testing.Stub
	remoteApplicationsWatcher          *mockStringsWatcher
	remoteApplicationRelationsWatchers map[string]*mockStringsWatcher
	remoteApplications                 map[string]*mockRemoteApplication
	relations                          map[string]*mockRelation
	relationsEndpoints                 map[string]*relationEndpointInfo
	remoteRelationWatchers             map[string]*mockRemoteRelationWatcher
	controllerInfo                     map[string]*api.Info
}

func newMockRelationsFacade(stub *testing.Stub) *mockRelationsFacade {
	return &mockRelationsFacade{
		stub:                               stub,
		remoteApplications:                 make(map[string]*mockRemoteApplication),
		relations:                          make(map[string]*mockRelation),
		relationsEndpoints:                 make(map[string]*relationEndpointInfo),
		remoteApplicationsWatcher:          newMockStringsWatcher(),
		remoteApplicationRelationsWatchers: make(map[string]*mockStringsWatcher),
		remoteRelationWatchers:             make(map[string]*mockRemoteRelationWatcher),
		controllerInfo:                     make(map[string]*api.Info),
	}
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

func (m *mockRelationsFacade) removeRelation(key string) (*mockRemoteRelationWatcher, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	w, ok := m.remoteRelationWatchers[key]
	delete(m.relations, key)
	return w, ok
}

func (m *mockRelationsFacade) updateRelationLife(key string, life life.Value) (*mockRemoteRelationWatcher, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	w, ok := m.remoteRelationWatchers[key]
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

func (m *mockRelationsFacade) ExportEntities(entities []names.Tag) ([]params.TokenResult, error) {
	m.stub.MethodCall(m, "ExportEntities", entities)
	if err := m.stub.NextErr(); err != nil {
		return nil, err
	}
	result := make([]params.TokenResult, len(entities))
	for i, e := range entities {
		result[i] = params.TokenResult{
			Token: "token-" + e.Id(),
		}
	}
	return result, nil
}

func (m *mockRelationsFacade) ImportRemoteEntity(entity names.Tag, token string) error {
	m.stub.MethodCall(m, "ImportRemoteEntity", entity, token)
	return m.stub.NextErr()
}

func (m *mockRelationsFacade) SaveMacaroon(entity names.Tag, mac *macaroon.Macaroon) error {
	m.stub.MethodCall(m, "SaveMacaroon", entity, mac)
	return m.stub.NextErr()
}

func (m *mockRelationsFacade) GetToken(entity names.Tag) (string, error) {
	m.stub.MethodCall(m, "GetToken", entity)
	if err := m.stub.NextErr(); err != nil {
		return "", err
	}
	return "token-" + entity.Id(), nil
}

func (m *mockRelationsFacade) RemoteApplications(names []string) ([]params.RemoteApplicationResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stub.MethodCall(m, "RemoteApplications", names)
	if err := m.stub.NextErr(); err != nil {
		return nil, err
	}
	mac, err := apitesting.NewMacaroon("test")
	if err != nil {
		return nil, err
	}
	result := make([]params.RemoteApplicationResult, len(names))
	for i, name := range names {
		if app, ok := m.remoteApplications[name]; ok {
			result[i] = params.RemoteApplicationResult{
				Result: &params.RemoteApplication{
					Name:            app.name,
					OfferUUID:       app.offeruuid,
					Life:            app.life,
					ModelUUID:       app.modelUUID,
					IsConsumerProxy: app.registered,
					Macaroon:        mac,
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
				Id:        rel.id,
				Life:      rel.life,
				Suspended: rel.Suspended(),
				Key:       keys[i],
			}
			if epInfo, ok := m.relationsEndpoints[key]; ok {
				result[i].Result.RemoteEndpointName = epInfo.remoteEndpointName
				result[i].Result.Endpoint = epInfo.localEndpoint
				result[i].Result.ApplicationName = epInfo.localApplicationName
			}
		} else {
			result[i].Error = common.ServerError(errors.NotFoundf(key))
		}
	}
	return result, nil
}

func (m *mockRelationsFacade) remoteRelationWatcher(key string) (*mockRemoteRelationWatcher, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	w, ok := m.remoteRelationWatchers[key]
	return w, ok
}

func (m *mockRelationsFacade) WatchLocalRelationChanges(relationKey string) (apiwatcher.RemoteRelationWatcher, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stub.MethodCall(m, "WatchLocalRelationChanges", relationKey)
	if err := m.stub.NextErr(); err != nil {
		return nil, err
	}
	m.remoteRelationWatchers[relationKey] = newMockRemoteRelationWatcher()
	return m.remoteRelationWatchers[relationKey], nil
}

func (m *mockRelationsFacade) ConsumeRemoteRelationChange(change params.RemoteRelationChangeEvent) error {
	m.stub.MethodCall(m, "ConsumeRemoteRelationChange", change)
	if err := m.stub.NextErr(); err != nil {
		return err
	}
	return nil
}

func (m *mockRelationsFacade) ControllerAPIInfoForModel(modelUUID string) (*api.Info, error) {
	m.stub.MethodCall(m, "ControllerAPIInfoForModel", modelUUID)
	if err := m.stub.NextErr(); err != nil {
		return nil, err
	}
	return m.controllerInfo[modelUUID], nil
}

func (m *mockRelationsFacade) SetRemoteApplicationStatus(applicationName string, status status.Status, message string) error {
	m.stub.MethodCall(m, "SetRemoteApplicationStatus", applicationName, status.String(), message)
	return nil
}

func (m *mockRelationsFacade) UpdateControllerForModel(controller crossmodel.ControllerInfo, modelUUID string) error {
	m.stub.MethodCall(m, "UpdateControllerForModel", controller, modelUUID)
	return nil
}

type mockRemoteRelationsFacade struct {
	mu                      sync.Mutex
	stub                    *testing.Stub
	remoteRelationWatchers  map[string]*mockRemoteRelationWatcher
	relationsStatusWatchers map[string]*mockRelationStatusWatcher
	offersStatusWatchers    map[string]*mockOfferStatusWatcher
}

func newMockRemoteRelationsFacade(stub *testing.Stub) *mockRemoteRelationsFacade {
	return &mockRemoteRelationsFacade{
		stub:                    stub,
		remoteRelationWatchers:  make(map[string]*mockRemoteRelationWatcher),
		relationsStatusWatchers: make(map[string]*mockRelationStatusWatcher),
		offersStatusWatchers:    make(map[string]*mockOfferStatusWatcher),
	}
}

func (m *mockRemoteRelationsFacade) Close() error {
	m.stub.MethodCall(m, "Close")
	return nil
}

func (m *mockRemoteRelationsFacade) PublishRelationChange(change params.RemoteRelationChangeEvent) error {
	m.stub.MethodCall(m, "PublishRelationChange", change)
	if err := m.stub.NextErr(); err != nil {
		return err
	}
	return nil
}

func (m *mockRemoteRelationsFacade) RegisterRemoteRelations(relations ...params.RegisterRemoteRelationArg) ([]params.RegisterRemoteRelationResult, error) {
	m.stub.MethodCall(m, "RegisterRemoteRelations", relations)
	if err := m.stub.NextErr(); err != nil {
		return nil, err
	}
	result := make([]params.RegisterRemoteRelationResult, len(relations))
	mac, err := apitesting.NewMacaroon("apimac")
	if err != nil {
		return nil, err
	}
	for i, rel := range relations {
		result[i].Result = &params.RemoteRelationDetails{
			Token:    "token-" + rel.OfferUUID,
			Macaroon: mac,
		}
	}
	return result, nil
}

func (m *mockRemoteRelationsFacade) remoteRelationWatcher(key string) (*mockRemoteRelationWatcher, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	w, ok := m.remoteRelationWatchers[key]
	return w, ok
}

func (m *mockRemoteRelationsFacade) WatchRelationChanges(relationToken string, appToken string, mac macaroon.Slice) (apiwatcher.RemoteRelationWatcher, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stub.MethodCall(m, "WatchRelationChanges", relationToken, appToken, mac)
	if err := m.stub.NextErr(); err != nil {
		return nil, err
	}
	m.remoteRelationWatchers[relationToken] = newMockRemoteRelationWatcher()
	return m.remoteRelationWatchers[relationToken], nil
}

func (m *mockRemoteRelationsFacade) relationsStatusWatcher(key string) (*mockRelationStatusWatcher, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	w, ok := m.relationsStatusWatchers[key]
	return w, ok
}

func (m *mockRemoteRelationsFacade) WatchRelationSuspendedStatus(arg params.RemoteEntityArg) (watcher.RelationStatusWatcher, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stub.MethodCall(m, "WatchRelationSuspendedStatus", arg.Token, arg.Macaroons)
	if err := m.stub.NextErr(); err != nil {
		return nil, err
	}
	m.relationsStatusWatchers[arg.Token] = newMockRelationStatusWatcher()
	return m.relationsStatusWatchers[arg.Token], nil
}

func (m *mockRemoteRelationsFacade) WatchOfferStatus(arg params.OfferArg) (watcher.OfferStatusWatcher, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stub.MethodCall(m, "WatchOfferStatus", arg.OfferUUID, arg.Macaroons)
	if err := m.stub.NextErr(); err != nil {
		return nil, err
	}
	m.offersStatusWatchers[arg.OfferUUID] = newMockOfferStatusWatcher()
	return m.offersStatusWatchers[arg.OfferUUID], nil
}

// RelationUnitSettings returns the relation unit settings for the given relation units in the remote model.
func (m *mockRemoteRelationsFacade) RelationUnitSettings(relationUnits []params.RemoteRelationUnit) ([]params.SettingsResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
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

type mockWatcher struct {
	testing.Stub
	tomb.Tomb
	mu         sync.Mutex
	terminated bool
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
	w.Tomb.Go(func() error {
		<-w.Tomb.Dying()
		return nil
	})
	return w
}

func (w *mockStringsWatcher) Changes() watcher.StringsChannel {
	w.MethodCall(w, "Changes")
	return w.changes
}

type mockRemoteApplication struct {
	testing.Stub
	name       string
	offeruuid  string
	url        string
	life       life.Value
	modelUUID  string
	registered bool
}

type mockRemoteRelationWatcher struct {
	mockWatcher
	changes chan params.RemoteRelationChangeEvent
}

func newMockRemoteRelationWatcher() *mockRemoteRelationWatcher {
	w := &mockRemoteRelationWatcher{
		changes: make(chan params.RemoteRelationChangeEvent, 1),
	}
	w.Tomb.Go(func() error {
		<-w.Tomb.Dying()
		return nil
	})
	return w
}

func (w *mockRemoteRelationWatcher) Changes() <-chan params.RemoteRelationChangeEvent {
	w.MethodCall(w, "Changes")
	return w.changes
}

type mockRelationUnitsWatcher struct {
	mockWatcher
	changes chan watcher.RelationUnitsChange
}

func newMockRelationUnitsWatcher() *mockRelationUnitsWatcher {
	w := &mockRelationUnitsWatcher{
		changes: make(chan watcher.RelationUnitsChange, 1),
	}
	w.Tomb.Go(func() error {
		<-w.Tomb.Dying()
		return nil
	})
	return w
}

func (w *mockRelationUnitsWatcher) Changes() watcher.RelationUnitsChannel {
	w.MethodCall(w, "Changes")
	return w.changes
}

type mockRelationStatusWatcher struct {
	mockWatcher
	changes chan []watcher.RelationStatusChange
}

func newMockRelationStatusWatcher() *mockRelationStatusWatcher {
	w := &mockRelationStatusWatcher{
		changes: make(chan []watcher.RelationStatusChange, 1),
	}
	w.Tomb.Go(func() error {
		<-w.Tomb.Dying()
		return nil
	})
	return w
}

func (w *mockRelationStatusWatcher) Changes() watcher.RelationStatusChannel {
	w.MethodCall(w, "Changes")
	return w.changes
}

type mockOfferStatusWatcher struct {
	mockWatcher
	changes chan []watcher.OfferStatusChange
}

func newMockOfferStatusWatcher() *mockOfferStatusWatcher {
	w := &mockOfferStatusWatcher{
		changes: make(chan []watcher.OfferStatusChange, 1),
	}
	w.Tomb.Go(func() error {
		<-w.Tomb.Dying()
		return nil
	})
	return w
}

func (w *mockOfferStatusWatcher) Changes() watcher.OfferStatusChannel {
	w.MethodCall(w, "Changes")
	return w.changes
}

func newMockRemoteApplication(name, url string) *mockRemoteApplication {
	return &mockRemoteApplication{
		name: name, url: url, life: life.Alive, offeruuid: "offer-" + name + "-uuid",
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

func (r *mockRemoteApplication) Life() life.Value {
	r.MethodCall(r, "Life")
	return r.life
}

type mockRelation struct {
	testing.Stub
	sync.Mutex
	id        int
	life      life.Value
	suspended bool
}

func newMockRelation(id int) *mockRelation {
	return &mockRelation{
		id:   id,
		life: life.Alive,
	}
}

func (r *mockRelation) Id() int {
	r.MethodCall(r, "Id")
	return r.id
}

func (r *mockRelation) Life() life.Value {
	r.MethodCall(r, "Life")
	return r.life
}

func (r *mockRelation) Suspended() bool {
	r.Lock()
	defer r.Unlock()
	r.MethodCall(r, "Suspended")
	return r.suspended
}

func (r *mockRelation) SetSuspended(suspended bool) {
	r.Lock()
	defer r.Unlock()
	r.suspended = suspended
}
