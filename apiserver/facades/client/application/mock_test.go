// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	"io"
	"strings"
	"sync"

	"github.com/juju/errors"
	jtesting "github.com/juju/testing"
	"github.com/juju/utils/set"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/names.v2"
	"gopkg.in/macaroon.v1"

	"github.com/juju/juju/apiserver/facades/client/application"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	statestorage "github.com/juju/juju/state/storage"
	"github.com/juju/juju/status"
	coretesting "github.com/juju/juju/testing"
)

type mockEnviron struct {
	environs.NetworkingEnviron

	stub      jtesting.Stub
	spaceInfo *environs.ProviderSpaceInfo
}

func (e *mockEnviron) ProviderSpaceInfo(space *network.SpaceInfo) (*environs.ProviderSpaceInfo, error) {
	e.stub.MethodCall(e, "ProviderSpaceInfo", space)
	return e.spaceInfo, e.stub.NextErr()
}

type mockNoNetworkEnviron struct {
	environs.Environ
}

type mockModel struct {
	uuid  string
	name  string
	owner string
}

func (m *mockModel) ModelTag() names.ModelTag {
	return names.NewModelTag(m.uuid)
}

func (m *mockModel) Name() string {
	return m.name
}

func (m *mockModel) Owner() names.UserTag {
	return names.NewUserTag(m.owner)
}

type mockCharm struct {
	jtesting.Stub

	charm.Charm
	config *charm.Config
	meta   *charm.Meta
}

func (m *mockCharm) Meta() *charm.Meta {
	return m.meta
}

func (c *mockCharm) Config() *charm.Config {
	c.MethodCall(c, "Config")
	c.PopNoErr()
	return c.config
}

type mockApplication struct {
	jtesting.Stub
	application.Application

	bindings    map[string]string
	charm       *mockCharm
	curl        *charm.URL
	endpoints   []state.Endpoint
	name        string
	subordinate bool
	series      string
	units       []mockUnit
}

func (m *mockApplication) Name() string {
	return m.name
}

func (m *mockApplication) Charm() (application.Charm, bool, error) {
	return m.charm, true, nil
}

func (m *mockApplication) CharmURL() (curl *charm.URL, force bool) {
	return m.curl, true
}

func (m *mockApplication) Endpoints() ([]state.Endpoint, error) {
	return m.endpoints, nil
}

func (m *mockApplication) EndpointBindings() (map[string]string, error) {
	return m.bindings, nil
}

func (a *mockApplication) AllUnits() ([]application.Unit, error) {
	a.MethodCall(a, "AllUnits")
	if err := a.NextErr(); err != nil {
		return nil, err
	}
	units := make([]application.Unit, len(a.units))
	for i := range a.units {
		units[i] = &a.units[i]
	}
	return units, nil
}

func (a *mockApplication) SetCharm(cfg state.SetCharmConfig) error {
	a.MethodCall(a, "SetCharm", cfg)
	return a.NextErr()
}

func (a *mockApplication) DestroyOperation() *state.DestroyApplicationOperation {
	a.MethodCall(a, "DestroyOperation")
	return &state.DestroyApplicationOperation{}
}

func (a *mockApplication) AddUnit(args state.AddUnitParams) (application.Unit, error) {
	a.MethodCall(a, "AddUnit", args)
	if err := a.NextErr(); err != nil {
		return nil, err
	}
	unitTag := names.NewUnitTag(a.name + "/99")
	return &mockUnit{tag: unitTag}, nil
}

func (a *mockApplication) IsPrincipal() bool {
	a.MethodCall(a, "IsPrincipal")
	a.PopNoErr()
	return !a.subordinate
}

func (a *mockApplication) UpdateApplicationSeries(series string, force bool) error {
	a.MethodCall(a, "UpdateApplicationSeries", series, force)
	return a.NextErr()
}

func (a *mockApplication) Series() string {
	a.MethodCall(a, "Series")
	a.PopNoErr()
	return a.series
}

type mockRemoteApplication struct {
	name           string
	sourceModelTag names.ModelTag
	endpoints      []state.Endpoint
	bindings       map[string]string
	spaces         []state.RemoteSpace
	offerUUID      string
	offerURL       string
	mac            *macaroon.Macaroon
}

func (m *mockRemoteApplication) Name() string {
	return m.name
}

func (m *mockRemoteApplication) SourceModel() names.ModelTag {
	return m.sourceModelTag
}

func (m *mockRemoteApplication) Endpoints() ([]state.Endpoint, error) {
	return m.endpoints, nil
}

func (m *mockRemoteApplication) Bindings() map[string]string {
	return m.bindings
}

func (m *mockRemoteApplication) Spaces() []state.RemoteSpace {
	return m.spaces
}

func (m *mockRemoteApplication) AddEndpoints(eps []charm.Relation) error {
	for _, ep := range eps {
		m.endpoints = append(m.endpoints, state.Endpoint{
			ApplicationName: m.name,
			Relation: charm.Relation{
				Name:      ep.Name,
				Interface: ep.Interface,
				Role:      ep.Role,
			},
		})
	}
	return nil
}

func (m *mockRemoteApplication) Destroy() error {
	return nil
}

type mockSpace struct {
	name       string
	providerId network.Id
	subnets    []application.Subnet
}

func (m *mockSpace) Name() string {
	return m.name
}

func (m *mockSpace) ProviderId() network.Id {
	return m.providerId
}

type mockSubnet struct {
	cidr              string
	vlantag           int
	providerId        network.Id
	providerNetworkId network.Id
	zones             []string
}

func (m *mockSubnet) CIDR() string {
	return m.cidr
}

func (m *mockSubnet) VLANTag() int {
	return m.vlantag
}

func (m *mockSubnet) ProviderId() network.Id {
	return m.providerId
}

func (m *mockSubnet) ProviderNetworkId() network.Id {
	return m.providerNetworkId
}

type mockBackend struct {
	jtesting.Stub
	application.Backend

	modelUUID                  string
	model                      application.Model
	charm                      *mockCharm
	allmodels                  []application.Model
	users                      set.Strings
	applications               map[string]application.Application
	remoteApplications         map[string]application.RemoteApplication
	spaces                     map[string]application.Space
	endpoints                  *[]state.Endpoint
	relations                  map[int]*mockRelation
	offerConnections           map[string]application.OfferConnection
	unitStorageAttachments     map[string][]state.StorageAttachment
	storageInstances           map[string]*mockStorage
	storageInstanceFilesystems map[string]*mockFilesystem
	controllers                map[string]crossmodel.ControllerInfo
}

func (m *mockBackend) ControllerTag() names.ControllerTag {
	return coretesting.ControllerTag
}

func (m *mockBackend) GetBlockForType(t state.BlockType) (state.Block, bool, error) {
	return nil, false, nil
}

func (m *mockBackend) Charm(curl *charm.URL) (application.Charm, error) {
	m.MethodCall(m, "Charm", curl)
	if err := m.NextErr(); err != nil {
		return nil, err
	}
	if m.charm != nil {
		return m.charm, nil
	}
	return nil, errors.NotFoundf("charm %q", curl)
}

func (m *mockBackend) Unit(name string) (application.Unit, error) {
	m.MethodCall(m, "Unit", name)
	if err := m.NextErr(); err != nil {
		return nil, err
	}
	var unitApp *mockApplication
	for appName, app := range m.applications {
		if strings.HasPrefix(name, appName+"/") {
			unitApp = app.(*mockApplication)
			break
		}
	}
	if unitApp != nil {
		for _, u := range unitApp.units {
			if u.tag.Id() == name {
				return &u, nil
			}
		}
	}
	return nil, errors.NotFoundf("unit %q", name)
}

func (m *mockBackend) InferEndpoints(endpoints ...string) ([]state.Endpoint, error) {
	m.MethodCall(m, "InferEndpoints", endpoints)
	if err := m.NextErr(); err != nil {
		return nil, err
	}
	if m.endpoints != nil {
		return *m.endpoints, nil
	}
	return nil, errors.Errorf("no relations found")
}

func (m *mockBackend) EndpointsRelation(endpoints ...state.Endpoint) (application.Relation, error) {
	m.MethodCall(m, "EndpointsRelation", endpoints)
	if err := m.NextErr(); err != nil {
		return nil, err
	}
	if rel, ok := m.relations[123]; ok {
		return rel, nil
	}
	return nil, errors.NotFoundf("relation")
}

func (m *mockBackend) Relation(id int) (application.Relation, error) {
	m.MethodCall(m, "Relation", id)
	if err := m.NextErr(); err != nil {
		return nil, err
	}
	if rel, ok := m.relations[id]; ok {
		return rel, nil
	}
	return nil, errors.NotFoundf("relation")
}

type mockOfferConnection struct {
	application.OfferConnection
}

func (m *mockBackend) OfferConnectionForRelation(key string) (application.OfferConnection, error) {
	m.MethodCall(m, "OfferConnectionForRelation", key)
	if err := m.NextErr(); err != nil {
		return nil, err
	}
	if oc, ok := m.offerConnections[key]; ok {
		return oc, nil
	}
	return nil, errors.NotFoundf("offer connection for relation")
}

func (m *mockBackend) UnitStorageAttachments(tag names.UnitTag) ([]state.StorageAttachment, error) {
	m.MethodCall(m, "UnitStorageAttachments", tag)
	if err := m.NextErr(); err != nil {
		return nil, err
	}
	return m.unitStorageAttachments[tag.Id()], nil
}

func (m *mockBackend) StorageInstance(tag names.StorageTag) (state.StorageInstance, error) {
	m.MethodCall(m, "StorageInstance", tag)
	if err := m.NextErr(); err != nil {
		return nil, err
	}
	s, ok := m.storageInstances[tag.Id()]
	if !ok {
		return nil, errors.NotFoundf("storage %s", tag.Id())
	}
	return s, nil
}

func (m *mockBackend) StorageInstanceFilesystem(tag names.StorageTag) (state.Filesystem, error) {
	m.MethodCall(m, "StorageInstanceFilesystem", tag)
	if err := m.NextErr(); err != nil {
		return nil, err
	}
	f, ok := m.storageInstanceFilesystems[tag.Id()]
	if !ok {
		return nil, errors.NotFoundf("filesystem for storage %s", tag.Id())
	}
	return f, nil
}

func (m *mockBackend) AddRemoteApplication(args state.AddRemoteApplicationParams) (application.RemoteApplication, error) {
	m.MethodCall(m, "AddRemoteApplication", args)
	if err := m.NextErr(); err != nil {
		return nil, err
	}
	app := &mockRemoteApplication{
		name:           args.Name,
		sourceModelTag: args.SourceModel,
		offerUUID:      args.OfferUUID,
		offerURL:       args.URL,
		bindings:       args.Bindings,
		mac:            args.Macaroon,
	}
	for _, ep := range args.Endpoints {
		app.endpoints = append(app.endpoints, state.Endpoint{
			ApplicationName: app.name,
			Relation: charm.Relation{
				Name:      ep.Name,
				Interface: ep.Interface,
				Role:      ep.Role,
			},
		})
	}
	for _, sp := range args.Spaces {
		remoteSpaceInfo := state.RemoteSpace{
			CloudType:          sp.CloudType,
			Name:               sp.Name,
			ProviderId:         string(sp.ProviderId),
			ProviderAttributes: sp.ProviderAttributes,
		}
		for _, sn := range sp.Subnets {
			remoteSpaceInfo.Subnets = append(remoteSpaceInfo.Subnets, state.RemoteSubnet{
				CIDR:              sn.CIDR,
				VLANTag:           sn.VLANTag,
				ProviderId:        string(sn.ProviderId),
				ProviderNetworkId: string(sn.ProviderNetworkId),
				AvailabilityZones: sn.AvailabilityZones,
			})
		}
		app.spaces = append(app.spaces, remoteSpaceInfo)
	}
	m.remoteApplications[app.name] = app
	return app, nil
}

func (m *mockBackend) RemoteApplication(name string) (application.RemoteApplication, error) {
	m.MethodCall(m, "RemoteApplication", name)
	if err := m.NextErr(); err != nil {
		return nil, err
	}
	app, ok := m.remoteApplications[name]
	if !ok {
		return nil, errors.NotFoundf("remote application %q", name)
	}
	return app, nil
}

func (m *mockBackend) Application(name string) (application.Application, error) {
	m.MethodCall(m, "Application", name)
	if err := m.NextErr(); err != nil {
		return nil, err
	}
	app, ok := m.applications[name]
	if !ok {
		return nil, errors.NotFoundf("application %q", name)
	}
	return app, nil
}

func (m *mockBackend) ApplyOperation(op state.ModelOperation) error {
	m.MethodCall(m, "ApplyOperation", op)
	return m.NextErr()
}

type mockExternalController struct {
	uuid string
	info crossmodel.ControllerInfo
}

func (m *mockExternalController) Id() string {
	return m.uuid
}

func (m *mockExternalController) ControllerInfo() crossmodel.ControllerInfo {
	return m.info
}

func (m *mockBackend) SaveController(controllerInfo crossmodel.ControllerInfo, modelUUID string) (application.ExternalController, error) {
	m.controllers[modelUUID] = controllerInfo
	return &mockExternalController{controllerInfo.ControllerTag.Id(), controllerInfo}, nil
}

func (m *mockBackend) Space(name string) (application.Space, error) {
	space, ok := m.spaces[name]
	if !ok {
		return nil, errors.NotFoundf("space %q", name)
	}
	return space, nil
}

func (m *mockBackend) Model() (application.Model, error) {
	return m.model, nil
}

func (m *mockBackend) ModelUUID() string {
	return m.modelUUID
}

func (m *mockBackend) ModelTag() names.ModelTag {
	m.MethodCall(m, "ModelTag")
	m.PopNoErr()
	return names.NewModelTag(m.modelUUID)
}

type mockBlockChecker struct {
	jtesting.Stub
}

func (c *mockBlockChecker) ChangeAllowed() error {
	c.MethodCall(c, "ChangeAllowed")
	return c.NextErr()
}

func (c *mockBlockChecker) RemoveAllowed() error {
	c.MethodCall(c, "RemoveAllowed")
	return c.NextErr()
}

type mockRelation struct {
	application.Relation
	jtesting.Stub

	tag             names.Tag
	status          status.Status
	message         string
	suspended       bool
	suspendedReason string
}

func (r *mockRelation) Tag() names.Tag {
	return r.tag
}

func (r *mockRelation) SetStatus(status status.StatusInfo) error {
	r.MethodCall(r, "SetStatus")
	r.status = status.Status
	r.message = status.Message
	return r.NextErr()
}

func (r *mockRelation) SetSuspended(suspended bool, reason string) error {
	r.MethodCall(r, "SetSuspended")
	r.suspended = suspended
	r.suspendedReason = reason
	return r.NextErr()
}

func (r *mockRelation) Suspended() bool {
	r.MethodCall(r, "Suspended")
	return r.suspended
}

func (r *mockRelation) SuspendedReason() string {
	r.MethodCall(r, "SuspendedReason")
	return r.suspendedReason
}

func (r *mockRelation) Destroy() error {
	r.MethodCall(r, "Destroy")
	return r.NextErr()
}

type mockUnit struct {
	application.Unit
	jtesting.Stub
	tag names.UnitTag
}

func (u *mockUnit) UnitTag() names.UnitTag {
	return u.tag
}

func (u *mockUnit) IsPrincipal() bool {
	u.MethodCall(u, "IsPrincipal")
	u.PopNoErr()
	return true
}

func (u *mockUnit) DestroyOperation() *state.DestroyUnitOperation {
	u.MethodCall(u, "DestroyOperation")
	return &state.DestroyUnitOperation{}
}

func (u *mockUnit) AssignWithPolicy(policy state.AssignmentPolicy) error {
	u.MethodCall(u, "AssignWithPolicy", policy)
	return u.NextErr()
}

func (u *mockUnit) AssignWithPlacement(placement *instance.Placement) error {
	u.MethodCall(u, "AssignWithPlacement", placement)
	return u.NextErr()
}

type mockStorageAttachment struct {
	state.StorageAttachment
	jtesting.Stub
	unit    names.UnitTag
	storage names.StorageTag
}

func (a *mockStorageAttachment) Unit() names.UnitTag {
	return a.unit
}

func (a *mockStorageAttachment) StorageInstance() names.StorageTag {
	return a.storage
}

type mockStorage struct {
	state.StorageInstance
	jtesting.Stub
	tag   names.StorageTag
	owner names.Tag
}

func (a *mockStorage) Kind() state.StorageKind {
	return state.StorageKindFilesystem
}

func (a *mockStorage) StorageTag() names.StorageTag {
	return a.tag
}

func (a *mockStorage) Owner() (names.Tag, bool) {
	return a.owner, a.owner != nil
}

type mockFilesystem struct {
	state.Filesystem
	detachable bool
}

func (f *mockFilesystem) Detachable() bool {
	return f.detachable
}

type blobs struct {
	sync.Mutex
	m map[string]bool // maps path to added (true), or deleted (false)
}

// Add adds a path to the list of known paths.
func (b *blobs) Add(path string) {
	b.Lock()
	defer b.Unlock()
	b.check()
	b.m[path] = true
}

// Remove marks a path as deleted, even if it was not previously Added.
func (b *blobs) Remove(path string) {
	b.Lock()
	defer b.Unlock()
	b.check()
	b.m[path] = false
}

func (b *blobs) check() {
	if b.m == nil {
		b.m = make(map[string]bool)
	}
}

type recordingStorage struct {
	statestorage.Storage
	putBarrier *sync.WaitGroup
	blobs      *blobs
}

func (s *recordingStorage) Put(path string, r io.Reader, size int64) error {
	if s.putBarrier != nil {
		// This goroutine has gotten to Put() so mark it Done() and
		// wait for the other goroutines to get to this point.
		s.putBarrier.Done()
		s.putBarrier.Wait()
	}
	if err := s.Storage.Put(path, r, size); err != nil {
		return errors.Trace(err)
	}
	s.blobs.Add(path)
	return nil
}

func (s *recordingStorage) Remove(path string) error {
	if err := s.Storage.Remove(path); err != nil {
		return errors.Trace(err)
	}
	s.blobs.Remove(path)
	return nil
}
