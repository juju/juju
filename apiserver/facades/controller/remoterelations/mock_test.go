// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoterelations_test

import (
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/testing"
	"gopkg.in/macaroon.v2"
	"gopkg.in/tomb.v2"

	common "github.com/juju/juju/apiserver/common/crossmodel"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/domain/relation"
	"github.com/juju/juju/state"
)

type mockRelation struct {
	common.Relation
	testing.Stub
	id                    int
	key                   string
	life                  state.Life
	suspended             bool
	units                 map[string]common.RelationUnit
	remoteUnits           map[string]common.RelationUnit
	endpoints             []relation.Endpoint
	endpointUnitsWatchers map[string]*mockRelationUnitsWatcher
	appSettings           map[string]map[string]interface{}
}

func newMockRelation(id int) *mockRelation {
	return &mockRelation{
		id:                    id,
		life:                  state.Alive,
		units:                 make(map[string]common.RelationUnit),
		remoteUnits:           make(map[string]common.RelationUnit),
		endpointUnitsWatchers: make(map[string]*mockRelationUnitsWatcher),
		appSettings:           make(map[string]map[string]interface{}),
	}
}

func (r *mockRelation) Id() int {
	r.MethodCall(r, "Id")
	return r.id
}

func (r *mockRelation) Tag() names.Tag {
	r.MethodCall(r, "Tag")
	return names.NewRelationTag(r.key)
}

func (r *mockRelation) Life() state.Life {
	r.MethodCall(r, "Life")
	return r.life
}

func (r *mockRelation) UnitCount() int {
	r.MethodCall(r, "UnitCount")
	return 666
}

func (r *mockRelation) Suspended() bool {
	r.MethodCall(r, "Suspended")
	return r.suspended
}

func (r *mockRelation) Unit(unitId string) (common.RelationUnit, error) {
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

func (r *mockRelation) RemoteUnit(unitId string) (common.RelationUnit, error) {
	r.MethodCall(r, "RemoteUnit", unitId)
	if err := r.NextErr(); err != nil {
		return nil, err
	}
	u, ok := r.remoteUnits[unitId]
	if !ok {
		return nil, errors.NotFoundf("remote unit %q", unitId)
	}
	return u, nil
}

func (r *mockRelation) Endpoints() []relation.Endpoint {
	r.MethodCall(r, "Endpoints")
	return r.endpoints
}

func (r *mockRelation) WatchUnits(applicationName string) (state.RelationUnitsWatcher, error) {
	r.MethodCall(r, "WatchUnits", applicationName)
	if err := r.NextErr(); err != nil {
		return nil, err
	}
	w, ok := r.endpointUnitsWatchers[applicationName]
	if !ok {
		return nil, errors.NotFoundf("application %q", applicationName)
	}
	return w, nil
}

func (r *mockRelation) ApplicationSettings(appName string) (map[string]interface{}, error) {
	r.MethodCall(r, "ApplicationSettings", appName)
	if err := r.NextErr(); err != nil {
		return nil, err
	}
	settings, found := r.appSettings[appName]
	if !found {
		return nil, errors.NotFoundf("fake settings for %q", appName)
	}
	return settings, nil
}

type mockOfferConnection struct {
	offerUUID string
	username  string
}

func (o *mockOfferConnection) OfferUUID() string {
	return o.offerUUID
}

func (o *mockOfferConnection) UserName() string {
	return o.username
}

type mockRemoteApplication struct {
	common.RemoteApplication
	testing.Stub
	name          string
	alias         string
	url           string
	life          state.Life
	status        status.Status
	terminated    bool
	message       string
	consumerproxy bool
}

func newMockRemoteApplication(name, url string) *mockRemoteApplication {
	return &mockRemoteApplication{
		name: name, alias: name + "-alias", url: url, life: state.Alive,
	}
}

func (r *mockRemoteApplication) Name() string {
	r.MethodCall(r, "Name")
	return r.name
}

func (r *mockRemoteApplication) OfferUUID() string {
	r.MethodCall(r, "OfferUUID")
	return r.name + "-uuid"
}

func (r *mockRemoteApplication) Tag() names.Tag {
	r.MethodCall(r, "Tag")
	return names.NewApplicationTag(r.name)
}

func (r *mockRemoteApplication) IsConsumerProxy() bool {
	r.MethodCall(r, "IsConsumerProxy")
	return r.consumerproxy
}

func (r *mockRemoteApplication) Life() state.Life {
	r.MethodCall(r, "Life")
	return r.life
}

func (r *mockRemoteApplication) Status() (status.StatusInfo, error) {
	r.MethodCall(r, "Status")
	return status.StatusInfo{Status: r.status}, nil
}

func (r *mockRemoteApplication) URL() (string, bool) {
	r.MethodCall(r, "URL")
	return r.url, r.url != ""
}

func (r *mockRemoteApplication) SourceModel() names.ModelTag {
	r.MethodCall(r, "SourceModel")
	return names.NewModelTag("model-uuid")
}

func (r *mockRemoteApplication) Macaroon() (*macaroon.Macaroon, error) {
	r.MethodCall(r, "Macaroon")
	return macaroon.New(nil, []byte("test"), "", macaroon.LatestVersion)
}

func (r *mockRemoteApplication) SetStatus(info status.StatusInfo) error {
	r.MethodCall(r, "SetStatus")
	r.status = info.Status
	r.message = info.Message
	return nil
}

func (a *mockRemoteApplication) ConsumeVersion() int {
	a.MethodCall(a, "ConsumeVersion")
	return 666
}

func (r *mockRemoteApplication) TerminateOperation(message string) state.ModelOperation {
	r.MethodCall(r, "TerminateOperation", message)
	r.terminated = true
	return &mockOperation{message: message}
}

type mockOperation struct {
	state.ModelOperation
	message string
}

type mockApplication struct {
	common.Application
	testing.Stub
	name string
	life state.Life
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

func (a *mockApplication) Tag() names.Tag {
	a.MethodCall(a, "Tag")
	return names.NewApplicationTag(a.name)
}

func (a *mockApplication) Life() state.Life {
	a.MethodCall(a, "Life")
	return a.life
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

type mockRelationUnit struct {
	common.RelationUnit
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
