// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodelrelations_test

import (
	"fmt"
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/juju/testing"
	"gopkg.in/juju/names.v2"
	"gopkg.in/macaroon-bakery.v2-unstable/bakery/checkers"
	"gopkg.in/macaroon.v2-unstable"

	apitesting "github.com/juju/juju/api/testing"
	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/common"
	commoncrossmodel "github.com/juju/juju/apiserver/common/crossmodel"
	"github.com/juju/juju/apiserver/common/firewall"
	"github.com/juju/juju/apiserver/facades/controller/crossmodelrelations"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
)

type mockStatePool struct {
	st map[string]commoncrossmodel.Backend
}

func (st *mockStatePool) Get(modelUUID string) (commoncrossmodel.Backend, func(), error) {
	backend, ok := st.st[modelUUID]
	if !ok {
		return nil, nil, errors.NotFoundf("model for uuid %s", modelUUID)
	}
	return backend, func() {}, nil
}

type mockState struct {
	testing.Stub
	crossmodelrelations.CrossModelRelationsState
	relations             map[string]*mockRelation
	remoteApplications    map[string]*mockRemoteApplication
	applications          map[string]*mockApplication
	offers                map[string]*crossmodel.ApplicationOffer
	offerConnections      map[int]*mockOfferConnection
	offerConnectionsByKey map[string]*mockOfferConnection
	remoteEntities        map[names.Tag]string
	firewallRules         map[state.WellKnownServiceType]*state.FirewallRule
	ingressNetworks       map[string][]string
}

func newMockState() *mockState {
	return &mockState{
		relations:             make(map[string]*mockRelation),
		remoteApplications:    make(map[string]*mockRemoteApplication),
		applications:          make(map[string]*mockApplication),
		remoteEntities:        make(map[names.Tag]string),
		offers:                make(map[string]*crossmodel.ApplicationOffer),
		offerConnections:      make(map[int]*mockOfferConnection),
		offerConnectionsByKey: make(map[string]*mockOfferConnection),
		firewallRules:         make(map[state.WellKnownServiceType]*state.FirewallRule),
		ingressNetworks:       make(map[string][]string),
	}
}

func (st *mockState) ApplicationOfferForUUID(offerUUID string) (*crossmodel.ApplicationOffer, error) {
	offer, ok := st.offers[offerUUID]
	if !ok {
		return nil, errors.NotFoundf("offer %v", offerUUID)
	}
	return offer, nil
}

func (st *mockState) ApplicationOffer(offerName string) (*crossmodel.ApplicationOffer, error) {
	for _, offer := range st.offers {
		if offer.OfferName == offerName {
			return offer, nil
		}
	}
	return nil, errors.NotFoundf("offer %v", offerName)
}

func (st *mockState) ModelUUID() string {
	return coretesting.ModelTag.Id()
}

func (st *mockState) Model() (crossmodelrelations.Model, error) {
	return &mockModel{}, nil
}

func (st *mockState) AddRelation(eps ...state.Endpoint) (commoncrossmodel.Relation, error) {
	rel := &mockRelation{
		id:  len(st.relations),
		key: fmt.Sprintf("%v:%v %v:%v", eps[0].ApplicationName, eps[0].Name, eps[1].ApplicationName, eps[1].Name),
	}
	if _, ok := st.relations[rel.key]; ok {
		return nil, errors.AlreadyExistsf("relation %q", rel.key)
	}
	st.relations[rel.key] = rel
	return rel, nil
}

func (st *mockState) AddOfferConnection(arg state.AddOfferConnectionParams) (crossmodelrelations.OfferConnection, error) {
	if _, ok := st.offerConnections[arg.RelationId]; ok {
		return nil, errors.AlreadyExistsf("offer connection for relation %d", arg.RelationId)
	}
	oc := &mockOfferConnection{
		sourcemodelUUID: arg.SourceModelUUID,
		relationId:      arg.RelationId,
		relationKey:     arg.RelationKey,
		username:        arg.Username,
		offerUUID:       arg.OfferUUID,
	}
	st.offerConnections[arg.RelationId] = oc
	st.offerConnectionsByKey[arg.RelationKey] = oc
	return oc, nil
}

func (st *mockState) FirewallRule(service state.WellKnownServiceType) (*state.FirewallRule, error) {
	if r, ok := st.firewallRules[service]; ok {
		return r, nil
	}
	return nil, errors.NotFoundf("firewall rule for %v", service)
}

func (st *mockState) SaveIngressNetworks(relationKey string, cidrs []string) (state.RelationNetworks, error) {
	st.ingressNetworks[relationKey] = cidrs
	return nil, nil
}

func (st *mockState) OfferConnectionForRelation(relationKey string) (crossmodelrelations.OfferConnection, error) {
	oc, ok := st.offerConnectionsByKey[relationKey]
	if !ok {
		return nil, errors.NotFoundf("offer connection details for relation %v", relationKey)
	}
	return oc, nil
}

func (st *mockState) EndpointsRelation(eps ...state.Endpoint) (commoncrossmodel.Relation, error) {
	key := fmt.Sprintf("%v:%v %v:%v", eps[0].ApplicationName, eps[0].Name, eps[1].ApplicationName, eps[1].Name)
	if rel, ok := st.relations[key]; ok {
		return rel, nil
	}
	return nil, errors.NotFoundf("relation with key %q", key)
}

func (st *mockState) AddRemoteApplication(params state.AddRemoteApplicationParams) (commoncrossmodel.RemoteApplication, error) {
	app := &mockRemoteApplication{
		sourceModelUUID: params.SourceModel.Id(),
		consumerproxy:   params.IsConsumerProxy}
	st.remoteApplications[params.Name] = app
	return app, nil
}

func (st *mockState) ImportRemoteEntity(entity names.Tag, token string) error {
	st.MethodCall(st, "ImportRemoteEntity", entity, token)
	if err := st.NextErr(); err != nil {
		return err
	}
	if _, ok := st.remoteEntities[entity]; ok {
		return errors.AlreadyExistsf(entity.Id())
	}
	st.remoteEntities[entity] = token
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

func (st *mockState) GetRemoteEntity(token string) (names.Tag, error) {
	st.MethodCall(st, "GetRemoteEntity", token)
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

func (st *mockState) KeyRelation(key string) (commoncrossmodel.Relation, error) {
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

func (st *mockState) RemoteApplication(id string) (commoncrossmodel.RemoteApplication, error) {
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

func (st *mockState) Application(id string) (commoncrossmodel.Application, error) {
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

type mockFirewallState struct {
	firewall.State
}

type mockWatcher struct {
	mu      sync.Mutex
	stopped chan struct{}
}

func (w *mockWatcher) Kill() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.Stopped() {
		close(w.stopped)
	}
}

func (w *mockWatcher) Stop() error {
	return nil
}

func (w *mockWatcher) Wait() error {
	<-w.stopped
	return nil
}

func (w *mockWatcher) Err() error {
	return nil
}

func (w *mockWatcher) Stopped() bool {
	select {
	case <-w.stopped:
		return true
	default:
		return false
	}
}

type mockRelationStatusWatcher struct {
	*mockWatcher
	changes chan []string
}

func (w *mockRelationStatusWatcher) Changes() <-chan []string {
	return w.changes
}

type mockOfferStatusWatcher struct {
	*mockWatcher
	offerUUID string
	changes   chan struct{}
}

func (w *mockOfferStatusWatcher) Changes() <-chan struct{} {
	return w.changes
}

func (w *mockOfferStatusWatcher) OfferUUID() string {
	return w.offerUUID
}

type mockModel struct {
}

func (m *mockModel) Name() string {
	return "prod"
}

func (m *mockModel) Owner() names.UserTag {
	return names.NewUserTag("fred")
}

type mockRelation struct {
	commoncrossmodel.Relation
	testing.Stub
	id              int
	key             string
	suspended       bool
	suspendedReason string
	status          status.Status
	message         string
	units           map[string]commoncrossmodel.RelationUnit
}

func newMockRelation(id int) *mockRelation {
	return &mockRelation{
		id:    id,
		units: make(map[string]commoncrossmodel.RelationUnit),
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

func (r *mockRelation) Destroy() error {
	r.MethodCall(r, "Destroy")
	return r.NextErr()
}

func (r *mockRelation) Life() state.Life {
	return state.Alive
}

func (r *mockRelation) SetStatus(statusInfo status.StatusInfo) error {
	r.MethodCall(r, "SetStatus")
	r.status = statusInfo.Status
	r.message = statusInfo.Message
	return nil
}

func (r *mockRelation) SetSuspended(suspended bool, reason string) error {
	r.MethodCall(r, "SetSuspended")
	r.suspended = suspended
	r.suspendedReason = reason
	return nil
}

func (r *mockRelation) Suspended() bool {
	r.MethodCall(r, "Suspended")
	return r.suspended
}

func (r *mockRelation) SuspendedReason() string {
	r.MethodCall(r, "SuspendedReason")
	return r.suspendedReason
}

func (r *mockRelation) RemoteUnit(unitId string) (commoncrossmodel.RelationUnit, error) {
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

func (r *mockRelation) AllRemoteUnits(appName string) ([]commoncrossmodel.RelationUnit, error) {
	r.MethodCall(r, "AllRemoteUnits", appName)
	if err := r.NextErr(); err != nil {
		return nil, err
	}
	var result []commoncrossmodel.RelationUnit
	for _, ru := range r.units {
		result = append(result, ru)
	}
	return result, nil
}

func (r *mockRelation) Unit(unitId string) (commoncrossmodel.RelationUnit, error) {
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

func (u *mockRelationUnit) Settings() (map[string]interface{}, error) {
	u.MethodCall(u, "Settings")
	return u.settings, u.NextErr()
}

type mockRemoteApplication struct {
	commoncrossmodel.RemoteApplication
	testing.Stub
	consumerproxy   bool
	sourceModelUUID string
}

func (r *mockRemoteApplication) IsConsumerProxy() bool {
	r.MethodCall(r, "IsConsumerProxy")
	return r.consumerproxy
}

func (r *mockRemoteApplication) Destroy() error {
	r.MethodCall(r, "Destroy")
	return r.NextErr()
}

type mockApplication struct {
	commoncrossmodel.Application
	testing.Stub
	life state.Life
	eps  []state.Endpoint
}

func (a *mockApplication) Endpoints() ([]state.Endpoint, error) {
	a.MethodCall(a, "Endpoints")
	return a.eps, nil
}

func (a *mockApplication) Life() state.Life {
	a.MethodCall(a, "Life")
	return a.life
}

func (a *mockApplication) Status() (status.StatusInfo, error) {
	a.MethodCall(a, "Status")
	return status.StatusInfo{}, nil
}

type mockOfferConnection struct {
	crossmodelrelations.OfferConnection
	sourcemodelUUID string
	relationId      int
	relationKey     string
	username        string
	offerUUID       string
}

func (m *mockOfferConnection) OfferUUID() string {
	return m.offerUUID
}

type mockRelationUnit struct {
	commoncrossmodel.RelationUnit
	testing.Stub
	inScope  bool
	settings map[string]interface{}
}

func newMockRelationUnit() *mockRelationUnit {
	return &mockRelationUnit{
		settings: make(map[string]interface{}),
	}
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

type mockBakeryService struct {
	testing.Stub
	authentication.ExpirableStorageBakeryService
}

func (s *mockBakeryService) NewMacaroon(caveats []checkers.Caveat) (*macaroon.Macaroon, error) {
	s.MethodCall(s, "NewMacaroon", caveats)
	mac, err := apitesting.NewMacaroon("id")
	if err != nil {
		return nil, err
	}
	for _, cav := range caveats {
		if err := mac.AddFirstPartyCaveat(cav.Condition); err != nil {
			return nil, err
		}
	}
	return mac, nil
}

func (s *mockBakeryService) CheckAny(ms []macaroon.Slice, assert map[string]string, checker checkers.Checker) (map[string]string, error) {
	if len(ms) != 1 {
		return nil, errors.New("unexpected macaroons")
	}
	if len(ms[0]) == 0 {
		return nil, errors.New("no macaroons")
	}
	declared := checkers.InferDeclared(ms[0])
	for k, v := range assert {
		if declared[k] != v {
			return nil, common.ErrPerm
		}
	}
	return declared, nil
}

func (s *mockBakeryService) ExpireStorageAfter(when time.Duration) (authentication.ExpirableStorageBakeryService, error) {
	s.MethodCall(s, "ExpireStorageAfter", when)
	return s, nil
}
