// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package applicationoffers_test

import (
	"fmt"
	"time"

	"github.com/juju/errors"
	jtesting "github.com/juju/testing"
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/names.v2"
	"gopkg.in/macaroon-bakery.v2-unstable/bakery/checkers"
	"gopkg.in/macaroon.v2-unstable"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/crossmodel"
	"github.com/juju/juju/apiserver/facades/client/applicationoffers"
	jujucrossmodel "github.com/juju/juju/core/crossmodel"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/network"
	"github.com/juju/juju/permission"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
)

const (
	offerCall       = "offerCall"
	offerCallUUID   = "offerCallUUID"
	addOfferCall    = "addOffersCall"
	listOffersCall  = "listOffersCall"
	updateOfferCall = "updateOfferCall"
	removeOfferCall = "removeOfferCall"
)

type stubApplicationOffers struct {
	jtesting.Stub
	jujucrossmodel.ApplicationOffers

	addOffer   func(offer jujucrossmodel.AddApplicationOfferArgs) (*jujucrossmodel.ApplicationOffer, error)
	listOffers func(filters ...jujucrossmodel.ApplicationOfferFilter) ([]jujucrossmodel.ApplicationOffer, error)
}

func (m *stubApplicationOffers) AddOffer(offer jujucrossmodel.AddApplicationOfferArgs) (*jujucrossmodel.ApplicationOffer, error) {
	m.AddCall(addOfferCall)
	return m.addOffer(offer)
}

func (m *stubApplicationOffers) ListOffers(filters ...jujucrossmodel.ApplicationOfferFilter) ([]jujucrossmodel.ApplicationOffer, error) {
	m.AddCall(listOffersCall)
	return m.listOffers(filters...)
}

func (m *stubApplicationOffers) UpdateOffer(offer jujucrossmodel.AddApplicationOfferArgs) (*jujucrossmodel.ApplicationOffer, error) {
	m.AddCall(updateOfferCall)
	panic("not implemented")
}

func (m *stubApplicationOffers) Remove(url string, force bool) error {
	m.AddCall(removeOfferCall)
	panic("not implemented")
}

func (m *stubApplicationOffers) ApplicationOffer(name string) (*jujucrossmodel.ApplicationOffer, error) {
	m.AddCall(offerCall)
	panic("not implemented")
}

func (m *stubApplicationOffers) ApplicationOfferForUUID(uuid string) (*jujucrossmodel.ApplicationOffer, error) {
	m.AddCall(offerCallUUID)
	panic("not implemented")
}

type mockEnviron struct {
	environs.NetworkingEnviron

	stub      jtesting.Stub
	spaceInfo *environs.ProviderSpaceInfo
}

func (e *mockEnviron) ProviderSpaceInfo(ctx context.ProviderCallContext, space *network.SpaceInfo) (*environs.ProviderSpaceInfo, error) {
	e.stub.MethodCall(e, "ProviderSpaceInfo", space)
	spaceName := corenetwork.DefaultSpaceName
	if space != nil {
		spaceName = space.Name
	}
	if e.spaceInfo == nil || spaceName != e.spaceInfo.Name {
		return nil, errors.NotFoundf("space %q", spaceName)
	}
	return e.spaceInfo, e.stub.NextErr()
}

type mockModel struct {
	uuid      string
	name      string
	owner     string
	modelType state.ModelType
}

func (m *mockModel) UUID() string {
	return m.uuid
}

func (m *mockModel) ModelTag() names.ModelTag {
	return names.NewModelTag(m.uuid)
}

func (m *mockModel) Type() state.ModelType {
	return m.modelType
}

func (m *mockModel) Name() string {
	return m.name
}

func (m *mockModel) Owner() names.UserTag {
	return names.NewUserTag(m.owner)
}

type mockCharm struct {
	meta *charm.Meta
}

func (m *mockCharm) Meta() *charm.Meta {
	return m.meta
}

func (m *mockCharm) StoragePath() string {
	return "storage-path"
}

type mockApplication struct {
	crossmodel.Application
	name      string
	charm     *mockCharm
	curl      *charm.URL
	endpoints []state.Endpoint
	bindings  map[string]string
}

func (m *mockApplication) Name() string {
	return m.name
}

func (m *mockApplication) Charm() (crossmodel.Charm, bool, error) {
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

type mockSpace struct {
	name       string
	providerId network.Id
	subnets    []applicationoffers.Subnet
}

func (m *mockSpace) Name() string {
	return m.name
}

func (m *mockSpace) Subnets() ([]applicationoffers.Subnet, error) {
	return m.subnets, nil
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

func (m *mockSubnet) AvailabilityZones() []string {
	return m.zones
}

type mockRelation struct {
	crossmodel.Relation
	id       int
	endpoint state.Endpoint
}

func (m *mockRelation) Status() (status.StatusInfo, error) {
	return status.StatusInfo{Status: status.Joined}, nil
}

func (m *mockRelation) Endpoint(appName string) (state.Endpoint, error) {
	if m.endpoint.ApplicationName != appName {
		return state.Endpoint{}, errors.NotFoundf("endpoint for %q", appName)
	}
	return m.endpoint, nil
}

type mockOfferConnection struct {
	modelUUID   string
	username    string
	relationKey string
	relationId  int
}

func (m *mockOfferConnection) SourceModelUUID() string {
	return m.modelUUID
}

func (m *mockOfferConnection) UserName() string {
	return m.username
}

func (m *mockOfferConnection) RelationKey() string {
	return m.relationKey
}

func (m *mockOfferConnection) RelationId() int {
	return m.relationId
}

type mockApplicationOffers struct {
	jujucrossmodel.ApplicationOffers
	st *mockState
}

func (m *mockApplicationOffers) ListOffers(filters ...jujucrossmodel.ApplicationOfferFilter) ([]jujucrossmodel.ApplicationOffer, error) {
	var result []jujucrossmodel.ApplicationOffer
	for _, f := range filters {
		if offer, ok := m.st.applicationOffers[f.OfferName]; ok {
			result = append(result, offer)
		}
	}
	return result, nil
}

func (m *mockApplicationOffers) Remove(name string, force bool) error {
	if len(m.st.connections) > 0 && !force {
		return errors.Errorf("offer has %d relations", len(m.st.connections))
	}
	_, ok := m.st.applicationOffers[name]
	if !ok {
		return errors.NotFoundf("application offer %q", name)
	}
	delete(m.st.applicationOffers, name)
	return nil
}

type offerAccess struct {
	user      names.UserTag
	offerUUID string
}

type mockState struct {
	crossmodel.Backend
	common.AddressAndCertGetter
	modelUUID         string
	model             *mockModel
	allmodels         []applicationoffers.Model
	users             map[string]applicationoffers.User
	applications      map[string]crossmodel.Application
	applicationOffers map[string]jujucrossmodel.ApplicationOffer
	spaces            map[string]applicationoffers.Space
	relations         map[string]crossmodel.Relation
	connections       []applicationoffers.OfferConnection
	accessPerms       map[offerAccess]permission.Access
	relationNetworks  state.RelationNetworks
}

func (m *mockState) GetAddressAndCertGetter() common.AddressAndCertGetter {
	return m
}

func (m *mockState) ControllerTag() names.ControllerTag {
	return testing.ControllerTag
}

func (m *mockState) Charm(*charm.URL) (crossmodel.Charm, error) {
	return &mockCharm{}, nil
}

func (m *mockState) Application(name string) (crossmodel.Application, error) {
	app, ok := m.applications[name]
	if !ok {
		return nil, errors.NotFoundf("application %q", name)
	}
	return app, nil
}

func (m *mockState) ApplicationOffer(name string) (*jujucrossmodel.ApplicationOffer, error) {
	offer, ok := m.applicationOffers[name]
	if !ok {
		return nil, errors.NotFoundf("application offer %q", name)
	}
	return &offer, nil
}

func (m *mockState) Space(name string) (applicationoffers.Space, error) {
	space, ok := m.spaces[name]
	if !ok {
		return nil, errors.NotFoundf("space %q", name)
	}
	return space, nil
}

func (m *mockState) Model() (applicationoffers.Model, error) {
	return m.model, nil
}

func (m *mockState) ModelUUID() string {
	return m.modelUUID
}

func (m *mockState) ModelTag() names.ModelTag {
	return names.NewModelTag(m.modelUUID)
}

func (m *mockState) AllModelUUIDs() ([]string, error) {
	if len(m.allmodels) == 0 {
		return []string{m.model.UUID()}, nil
	}

	var out []string
	for _, model := range m.allmodels {
		out = append(out, model.UUID())
	}
	return out, nil
}

func (m *mockState) KeyRelation(key string) (crossmodel.Relation, error) {
	rel, ok := m.relations[key]
	if !ok {
		return nil, errors.NotFoundf("relation key %v", key)
	}
	return rel, nil
}

func (m *mockState) OfferConnections(offerUUID string) ([]applicationoffers.OfferConnection, error) {
	return m.connections, nil
}

func (m *mockState) User(tag names.UserTag) (applicationoffers.User, error) {
	user, ok := m.users[tag.Id()]
	if !ok {
		return nil, errors.NotFoundf("user %v", tag.Id())
	}
	return user, nil
}

type mockUser struct {
	name string
}

func (m *mockUser) DisplayName() string {
	return m.name
}

type mockRelationNetworks struct {
	state.RelationNetworks
}

func (m *mockRelationNetworks) CIDRS() []string {
	return []string{"192.168.1.0/32", "10.0.0.0/8"}
}

func (m *mockState) IngressNetworks(relationKey string) (state.RelationNetworks, error) {
	if m.relationNetworks == nil {
		return nil, errors.NotFoundf("ingress networks")
	}
	return m.relationNetworks, nil
}

func (m *mockState) GetOfferAccess(offerUUID string, user names.UserTag) (permission.Access, error) {
	access, ok := m.accessPerms[offerAccess{user: user, offerUUID: offerUUID}]
	if !ok {
		return "", errors.NotFoundf("offer access for %v", user)
	}
	return access, nil
}

func (m *mockState) CreateOfferAccess(offer names.ApplicationOfferTag, user names.UserTag, access permission.Access) error {
	if _, ok := m.users[user.Name()]; !ok {
		return errors.NotFoundf("user %q", user.Name())
	}
	if _, ok := m.accessPerms[offerAccess{user: user, offerUUID: offer.Id() + "-uuid"}]; ok {
		return errors.NewAlreadyExists(nil, fmt.Sprintf("offer user %s", user.Name()))
	}
	m.accessPerms[offerAccess{user: user, offerUUID: offer.Id() + "-uuid"}] = access
	return nil
}

func (m *mockState) UpdateOfferAccess(offer names.ApplicationOfferTag, user names.UserTag, access permission.Access) error {
	if _, ok := m.users[user.Name()]; !ok {
		return errors.NotFoundf("user %q", user.Name())
	}
	if _, ok := m.accessPerms[offerAccess{user: user, offerUUID: offer.Id() + "-uuid"}]; !ok {
		return errors.NewNotFound(nil, fmt.Sprintf("offer user %s", user.Name()))
	}
	m.accessPerms[offerAccess{user: user, offerUUID: offer.Id() + "-uuid"}] = access
	return nil
}

func (m *mockState) RemoveOfferAccess(offer names.ApplicationOfferTag, user names.UserTag) error {
	if _, ok := m.users[user.Name()]; !ok {
		return errors.NewNotFound(nil, fmt.Sprintf("offer user %q does not exist", user.Name()))
	}
	delete(m.accessPerms, offerAccess{user: user, offerUUID: offer.Id() + "-uuid"})
	return nil
}

func (m *mockState) GetOfferUsers(offerUUID string) (map[string]permission.Access, error) {
	result := make(map[string]permission.Access)
	for offerAccess, access := range m.accessPerms {
		if offerAccess.offerUUID != offerUUID {
			continue
		}
		result[offerAccess.user.Id()] = access
	}
	return result, nil
}

type mockStatePool struct {
	st map[string]applicationoffers.Backend
}

func (st *mockStatePool) Get(modelUUID string) (applicationoffers.Backend, func(), error) {
	backend, ok := st.st[modelUUID]
	if !ok {
		return nil, nil, errors.NotFoundf("model for uuid %s", modelUUID)
	}
	return backend, func() {}, nil
}

func (st *mockStatePool) GetModel(modelUUID string) (applicationoffers.Model, func(), error) {
	backend, ok := st.st[modelUUID]
	if !ok {
		return nil, nil, errors.NotFoundf("model for uuid %s", modelUUID)
	}
	model, err := backend.Model()
	if err != nil {
		return nil, nil, err
	}
	return model, func() {}, nil
}

type mockCommonStatePool struct {
	*mockStatePool
}

func (st *mockCommonStatePool) Get(modelUUID string) (crossmodel.Backend, func(), error) {
	return st.mockStatePool.Get(modelUUID)
}

type mockBakeryService struct {
	authentication.ExpirableStorageBakeryService
	jtesting.Stub
	caveats map[string][]checkers.Caveat
}

func (s *mockBakeryService) NewMacaroon(caveats []checkers.Caveat) (*macaroon.Macaroon, error) {
	s.MethodCall(s, "NewMacaroon", caveats)
	s.caveats["id"] = caveats
	return macaroon.New(nil, []byte("id"), "")
}

func (s *mockBakeryService) ExpireStorageAfter(when time.Duration) (authentication.ExpirableStorageBakeryService, error) {
	s.MethodCall(s, "ExpireStorageAfter", when)
	return s, nil
}

func getFakeControllerInfo() ([]string, string, error) {
	return []string{"192.168.1.1:17070"}, testing.CACert, nil
}
