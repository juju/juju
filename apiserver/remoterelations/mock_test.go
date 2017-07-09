// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoterelations_test

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/testing"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/names.v2"
	"gopkg.in/macaroon.v1"
	"gopkg.in/tomb.v1"

	common "github.com/juju/juju/apiserver/common/crossmodel"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/apiserver/remoterelations"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/state"
	"github.com/juju/juju/status"
	coretesting "github.com/juju/juju/testing"
)

type mockState struct {
	remoterelations.RemoteRelationsState
	testing.Stub
	relations                    map[string]*mockRelation
	remoteApplications           map[string]*mockRemoteApplication
	applications                 map[string]*mockApplication
	remoteApplicationsWatcher    *mockStringsWatcher
	remoteRelationsWatcher       *mockStringsWatcher
	applicationRelationsWatchers map[string]*mockStringsWatcher
	remoteEntities               map[names.Tag]string
	controllerInfo               map[string]*mockControllerInfo
}

func newMockState() *mockState {
	return &mockState{
		relations:                    make(map[string]*mockRelation),
		remoteApplications:           make(map[string]*mockRemoteApplication),
		applications:                 make(map[string]*mockApplication),
		remoteApplicationsWatcher:    newMockStringsWatcher(),
		remoteRelationsWatcher:       newMockStringsWatcher(),
		applicationRelationsWatchers: make(map[string]*mockStringsWatcher),
		remoteEntities:               make(map[names.Tag]string),
		controllerInfo:               make(map[string]*mockControllerInfo),
	}
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

func (st *mockState) ModelUUID() string {
	return coretesting.ModelTag.Id()
}

func (st *mockState) AddRelation(eps ...state.Endpoint) (common.Relation, error) {
	rel := &mockRelation{
		key: fmt.Sprintf("%v:%v %v:%v", eps[0].ApplicationName, eps[0].Name, eps[1].ApplicationName, eps[1].Name)}
	st.relations[rel.key] = rel
	return rel, nil
}

func (st *mockState) ImportRemoteEntity(sourceModel names.ModelTag, entity names.Tag, token string) error {
	st.MethodCall(st, "ImportRemoteEntity", sourceModel, entity, token)
	if err := st.NextErr(); err != nil {
		return err
	}
	if _, ok := st.remoteEntities[entity]; ok {
		return errors.AlreadyExistsf(entity.Id())
	}
	st.remoteEntities[entity] = token
	return nil
}

func (st *mockState) RemoveRemoteEntity(sourceModel names.ModelTag, entity names.Tag) error {
	st.MethodCall(st, "RemoveRemoteEntity", sourceModel, entity)
	if err := st.NextErr(); err != nil {
		return err
	}
	delete(st.remoteEntities, entity)
	return nil
}

func (st *mockState) ExportLocalEntity(entity names.Tag) (string, error) {
	st.MethodCall(st, "ExportLocalEntity", entity)
	if err := st.NextErr(); err != nil {
		return "", err
	}
	if token, ok := st.remoteEntities[entity]; ok {
		return token, errors.AlreadyExistsf(entity.Id())
	}
	token := "token-" + entity.Id()
	st.remoteEntities[entity] = token
	return token, nil
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

func (st *mockState) GetToken(sourceModel names.ModelTag, entity names.Tag) (string, error) {
	st.MethodCall(st, "GetToken", sourceModel, entity)
	if err := st.NextErr(); err != nil {
		return "", err
	}
	return "token-" + entity.String(), nil
}

func (st *mockState) KeyRelation(key string) (common.Relation, error) {
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

func (st *mockState) Relation(id int) (common.Relation, error) {
	st.MethodCall(st, "Relation", id)
	if err := st.NextErr(); err != nil {
		return nil, err
	}
	for _, r := range st.relations {
		if r.id == id {
			return r, nil
		}
	}
	return nil, errors.NotFoundf("relation %d", id)
}

func (st *mockState) RemoteApplication(id string) (common.RemoteApplication, error) {
	st.MethodCall(st, "RemoteApplication", id)
	if err := st.NextErr(); err != nil {
		return nil, err
	}
	a, ok := st.remoteApplications[id]
	if !ok {
		return nil, errors.NotFoundf("remote application %q", id)
	}
	return a, nil
}

func (st *mockState) Application(id string) (common.Application, error) {
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

func (st *mockState) WatchRemoteApplications() state.StringsWatcher {
	st.MethodCall(st, "WatchRemoteApplications")
	return st.remoteApplicationsWatcher
}

func (st *mockState) WatchRemoteApplicationRelations(applicationName string) (state.StringsWatcher, error) {
	st.MethodCall(st, "WatchRemoteApplicationRelations", applicationName)
	if err := st.NextErr(); err != nil {
		return nil, err
	}
	w, ok := st.applicationRelationsWatchers[applicationName]
	if !ok {
		return nil, errors.NotFoundf("application %q", applicationName)
	}
	return w, nil
}

func (st *mockState) WatchRemoteRelations() state.StringsWatcher {
	st.MethodCall(st, "WatchRemoteRelations")
	return st.remoteRelationsWatcher
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
	common.Relation
	testing.Stub
	id                    int
	key                   string
	life                  state.Life
	units                 map[string]common.RelationUnit
	remoteUnits           map[string]common.RelationUnit
	endpoints             []state.Endpoint
	endpointUnitsWatchers map[string]*mockRelationUnitsWatcher
}

func newMockRelation(id int) *mockRelation {
	return &mockRelation{
		id:                    id,
		life:                  state.Alive,
		units:                 make(map[string]common.RelationUnit),
		remoteUnits:           make(map[string]common.RelationUnit),
		endpointUnitsWatchers: make(map[string]*mockRelationUnitsWatcher),
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

func (r *mockRelation) Endpoints() []state.Endpoint {
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

type mockRemoteApplication struct {
	common.RemoteApplication
	testing.Stub
	name          string
	alias         string
	url           string
	life          state.Life
	status        status.Status
	eps           []charm.Relation
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

func (r *mockRemoteApplication) OfferName() string {
	r.MethodCall(r, "OfferName")
	return r.alias
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
	return macaroon.New(nil, "test", "")
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
	changes chan params.RelationUnitsChange
}

func newMockRelationUnitsWatcher() *mockRelationUnitsWatcher {
	w := &mockRelationUnitsWatcher{
		changes: make(chan params.RelationUnitsChange, 1),
	}
	go w.doneWhenDying()
	return w
}

func (w *mockRelationUnitsWatcher) Changes() <-chan params.RelationUnitsChange {
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
