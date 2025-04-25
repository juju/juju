// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodelrelations_test

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/checkers"
	"github.com/juju/errors"
	"github.com/juju/mgo/v3/txn"
	"github.com/juju/names/v6"
	"github.com/juju/testing"
	jujutxn "github.com/juju/txn/v3"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/apiserver/authentication"
	commoncrossmodel "github.com/juju/juju/apiserver/common/crossmodel"
	"github.com/juju/juju/apiserver/common/firewall"
	"github.com/juju/juju/apiserver/facades/controller/crossmodelrelations"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/objectstore"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/domain/relation"
	"github.com/juju/juju/environs/config"
	internalmacaroon "github.com/juju/juju/internal/macaroon"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/state"
)

type mockState struct {
	testing.Stub
	crossmodelrelations.CrossModelRelationsState
	modelConfig           coretesting.Attrs
	relations             map[string]*mockRelation
	remoteApplications    map[string]*mockRemoteApplication
	applications          map[string]*mockApplication
	offers                map[string]*crossmodel.ApplicationOffer
	offerUUIDs            map[string]string
	offerConnections      map[int]*mockOfferConnection
	offerConnectionsByKey map[string]*mockOfferConnection
	remoteEntities        map[names.Tag]string
	ingressNetworks       map[string][]string
	migrationActive       bool
}

func newMockState() *mockState {
	return &mockState{
		modelConfig:           make(coretesting.Attrs),
		relations:             make(map[string]*mockRelation),
		remoteApplications:    make(map[string]*mockRemoteApplication),
		applications:          make(map[string]*mockApplication),
		remoteEntities:        make(map[names.Tag]string),
		offers:                make(map[string]*crossmodel.ApplicationOffer),
		offerUUIDs:            make(map[string]string),
		offerConnections:      make(map[int]*mockOfferConnection),
		offerConnectionsByKey: make(map[string]*mockOfferConnection),
		ingressNetworks:       make(map[string][]string),
	}
}

func (st *mockState) ControllerTag() names.ControllerTag {
	return coretesting.ControllerTag
}

func (st *mockState) ApplicationOfferForUUID(offerUUID string) (*crossmodel.ApplicationOffer, error) {
	st.MethodCall(st, "ApplicationOfferForUUID", offerUUID)
	offer, ok := st.offers[offerUUID]
	if !ok {
		return nil, errors.NotFoundf("offer %v", offerUUID)
	}
	return offer, nil
}

func (st *mockState) ModelTag() names.ModelTag {
	return coretesting.ModelTag
}

func (st *mockState) ModelUUID() string {
	return coretesting.ModelTag.Id()
}

func (st *mockState) Model() (crossmodelrelations.Model, error) {
	return &mockModel{}, nil
}

func (st *mockState) ModelConfig(_ context.Context) (*config.Config, error) {
	attrs := coretesting.FakeConfig().Merge(st.modelConfig)
	return config.New(config.NoDefaults, attrs)
}

func (st *mockState) ApplyOperation(state.ModelOperation) error {
	return nil
}

func (st *mockState) AddRelation(eps ...relation.Endpoint) (commoncrossmodel.Relation, error) {
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

func (st *mockState) AddOfferConnection(arg commoncrossmodel.AddOfferConnectionParams) (commoncrossmodel.OfferConnection, error) {
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

func (st *mockState) SaveIngressNetworks(relationKey string, cidrs []string) (commoncrossmodel.RelationNetworks, error) {
	st.ingressNetworks[relationKey] = cidrs
	return nil, nil
}

func (st *mockState) OfferConnectionForRelation(relationKey string) (commoncrossmodel.OfferConnection, error) {
	oc, ok := st.offerConnectionsByKey[relationKey]
	if !ok {
		return nil, errors.NotFoundf("offer connection details for relation %v", relationKey)
	}
	return oc, nil
}

func (st *mockState) EndpointsRelation(eps ...relation.Endpoint) (commoncrossmodel.Relation, error) {
	key := fmt.Sprintf("%v:%v %v:%v", eps[0].ApplicationName, eps[0].Name, eps[1].ApplicationName, eps[1].Name)
	if rel, ok := st.relations[key]; ok {
		return rel, nil
	}
	return nil, errors.NotFoundf("relation with key %q", key)
}

func (st *mockState) AddRemoteApplication(params commoncrossmodel.AddRemoteApplicationParams) (commoncrossmodel.RemoteApplication,
	error) {
	app := &mockRemoteApplication{
		sourceModelUUID: params.SourceModel.Id(),
		consumerproxy:   params.IsConsumerProxy,
		consumeversion:  params.ConsumeVersion,
	}
	st.remoteApplications[params.Name] = app
	return app, nil
}

func (st *mockState) OfferUUIDForRelation(key string) (string, error) {
	st.MethodCall(st, "OfferUUIDForRelation", key)
	if err := st.NextErr(); err != nil {
		return "", err
	}
	return st.offerUUIDs[key], nil
}

func (st *mockState) ImportRemoteEntity(entity names.Tag, token string) error {
	st.MethodCall(st, "ImportRemoteEntity", entity, token)
	if err := st.NextErr(); err != nil {
		return err
	}
	if existing, ok := st.remoteEntities[entity]; ok && existing != token {
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

func (st *mockState) GetToken(entity names.Tag) (string, error) {
	st.MethodCall(st, "GetToken", entity)
	if err := st.NextErr(); err != nil {
		return "", err
	}
	for e, t := range st.remoteEntities {
		if e.Id() == entity.Id() {
			return t, nil
		}
	}
	return "", errors.NotFoundf("entity %v", entity)
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

func (st *mockState) IsMigrationActive() (bool, error) {
	st.MethodCall(st, "IsMigrationActive")
	if err := st.NextErr(); err != nil {
		return false, err
	}
	return st.migrationActive, nil
}

func (st *mockState) RemoteApplication(id string) (commoncrossmodel.RemoteApplication, error) {
	st.MethodCall(st, "RemoteApplication", id)
	if err := st.NextErr(); err != nil {
		return nil, err
	}
	a, ok := st.remoteApplications[id]
	if !ok {
		return nil, errors.NotFoundf("saas application %q", id)
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

type mockSecretsWatcher struct {
	*mockWatcher
	changes chan []string
}

func (w *mockSecretsWatcher) Changes() <-chan []string {
	return w.changes
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
	offerName string
	changes   chan struct{}
}

func (w *mockOfferStatusWatcher) Changes() <-chan struct{} {
	return w.changes
}

func (w *mockOfferStatusWatcher) OfferUUID() string {
	return w.offerUUID
}

func (w *mockOfferStatusWatcher) OfferName() string {
	return w.offerName
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
	endpoints       []relation.Endpoint
	watchers        map[string]*mockUnitsWatcher
	appSettings     map[string]map[string]interface{}
}

func newMockRelation(id int) *mockRelation {
	return &mockRelation{
		id:          id,
		key:         "db2:db django:db",
		units:       make(map[string]commoncrossmodel.RelationUnit),
		watchers:    make(map[string]*mockUnitsWatcher),
		appSettings: make(map[string]map[string]interface{}),
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

func (r *mockRelation) Destroy(_ objectstore.ObjectStore) error {
	r.MethodCall(r, "Destroy")
	return r.NextErr()
}

func (r *mockRelation) DestroyWithForce(force bool, maxWait time.Duration) ([]error, error) {
	r.MethodCall(r, "DestroyWithForce", force)
	return nil, r.NextErr()
}

func (r *mockRelation) Life() state.Life {
	return state.Alive
}

func (r *mockRelation) UnitCount() int {
	return 666
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

func (r *mockRelation) ReplaceApplicationSettings(appName string, values map[string]interface{}) error {
	r.MethodCall(r, "ReplaceApplicationSettings", appName, values)
	return r.NextErr()
}

func (r *mockRelation) Endpoints() []relation.Endpoint {
	r.MethodCall(r, "Endpoints")
	return r.endpoints
}

func (r *mockRelation) WatchUnits(appName string) (relation.RelationUnitsWatcher, error) {
	r.MethodCall(r, "WatchUnits", appName)
	if err := r.NextErr(); err != nil {
		return nil, err
	}
	w, found := r.watchers[appName]
	if !found {
		return nil, errors.NotFoundf("fake watcher for %q units", appName)
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

type mockDestroyOperation struct {
	state.ModelOperation
}

func (m *mockDestroyOperation) Build(int) ([]txn.Op, error) {
	return nil, jujutxn.ErrNoOperations
}

func (m *mockDestroyOperation) Done(error) error {
	return nil
}

type mockRemoteApplication struct {
	commoncrossmodel.RemoteApplication
	testing.Stub
	consumerproxy   bool
	sourceModelUUID string
	consumeversion  int
}

func (r *mockRemoteApplication) IsConsumerProxy() bool {
	r.MethodCall(r, "IsConsumerProxy")
	return r.consumerproxy
}

func (a *mockRemoteApplication) ConsumeVersion() int {
	a.MethodCall(a, "ConsumeVersion")
	return a.consumeversion
}

func (r *mockRemoteApplication) Destroy() error {
	r.MethodCall(r, "Destroy")
	return r.NextErr()
}

func (r *mockRemoteApplication) DestroyOperation(force bool) state.ModelOperation {
	r.MethodCall(r, "DestroyOperation")
	return &mockDestroyOperation{}
}

type mockApplication struct {
	commoncrossmodel.Application
	name      string
	appStatus status.Status
	testing.Stub
	life state.Life
	eps  []relation.Endpoint
}

func (a *mockApplication) Name() string {
	a.MethodCall(a, "Name")
	return a.name
}

func (a *mockApplication) Endpoints() ([]relation.Endpoint, error) {
	a.MethodCall(a, "Endpoints")
	return a.eps, nil
}

func (a *mockApplication) Life() state.Life {
	a.MethodCall(a, "Life")
	return a.life
}

func (a *mockApplication) Status() (status.StatusInfo, error) {
	a.MethodCall(a, "Status")
	s := status.Terminated
	if a.appStatus != "" {
		s = a.appStatus
	}
	return status.StatusInfo{Status: s}, nil
}

type mockOfferConnection struct {
	sourcemodelUUID string
	relationId      int
	relationKey     string
	username        string
	offerUUID       string
}

func (m *mockOfferConnection) OfferUUID() string {
	return m.offerUUID
}

func (m *mockOfferConnection) UserName() string {
	return m.username
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

func (u *mockRelationUnit) Settings() (map[string]interface{}, error) {
	u.MethodCall(u, "Settings")
	return u.settings, u.NextErr()
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

type mockAuthorizer struct{}

func (mockAuthorizer) AuthorizeOps(ctx context.Context, authorizedOp bakery.Op, queryOps []bakery.Op) ([]bool, []checkers.Caveat, error) {
	allowed := make([]bool, len(queryOps))
	for i := range allowed {
		allowed[i] = queryOps[i].Action == "consume" || queryOps[i].Action == "relate"
	}
	return allowed, nil, nil
}

type mockVerifier struct {
	ops []bakery.Op
}

func (m mockVerifier) VerifyMacaroon(ctx context.Context, ms macaroon.Slice) ([]bakery.Op, []string, error) {
	declared := checkers.InferDeclared(internalmacaroon.MacaroonNamespace, ms)
	var conditions []string
	for k, v := range declared {
		conditions = append(conditions, fmt.Sprintf("declared %v %v", k, v))
	}
	return m.ops, conditions, nil
}

type mockBakeryService struct {
	testing.Stub
	authentication.ExpirableStorageBakery
	ops []bakery.Op
}

func (s *mockBakeryService) NewMacaroon(ctx context.Context, version bakery.Version, caveats []checkers.Caveat, ops ...bakery.Op) (*bakery.Macaroon, error) {
	s.MethodCall(s, "NewMacaroon", caveats)
	mac, err := macaroon.New(nil, []byte("id"), "", macaroon.LatestVersion)
	if err != nil {
		return nil, err
	}
	for _, cav := range caveats {
		if err := mac.AddFirstPartyCaveat([]byte(cav.Condition)); err != nil {
			return nil, err
		}
	}
	s.ops = ops
	return bakery.NewLegacyMacaroon(mac)
}

func (s *mockBakeryService) Auth(_ context.Context, mss ...macaroon.Slice) *bakery.AuthChecker {
	s.MethodCall(s, "Auth", mss)
	checker := bakery.NewChecker(bakery.CheckerParams{
		OpsAuthorizer:    mockAuthorizer{},
		MacaroonVerifier: mockVerifier{s.ops},
	})
	return checker.Auth(mss...)
}

func (s *mockBakeryService) ExpireStorageAfter(when time.Duration) (authentication.ExpirableStorageBakery, error) {
	s.MethodCall(s, "ExpireStorageAfter", when)
	return s, nil
}

type mockUnitsWatcher struct {
	*mockWatcher
	changes chan watcher.RelationUnitsChange
}

func (w *mockUnitsWatcher) Changes() watcher.RelationUnitsChannel {
	return w.changes
}

type mockSecretService struct {
	secrets map[string]coresecrets.SecretMetadata
}

func newMockSecretService() *mockSecretService {
	return &mockSecretService{
		secrets: make(map[string]coresecrets.SecretMetadata),
	}
}

func (m *mockSecretService) GetSecret(ctx context.Context, uri *coresecrets.URI) (*coresecrets.SecretMetadata, error) {
	md, ok := m.secrets[uri.ID]
	if ok {
		return &md, nil
	}
	return nil, errors.NotFoundf("secret id %q", uri.ID)
}

func (m *mockSecretService) WatchRemoteConsumedSecretsChanges(ctx context.Context, appName string) (watcher.StringsWatcher, error) {
	return &mockSecretsWatcher{}, nil
}
