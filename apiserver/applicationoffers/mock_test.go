// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package applicationoffers_test

import (
	"fmt"

	"github.com/juju/errors"
	jtesting "github.com/juju/testing"
	"github.com/juju/utils/set"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/applicationoffers"
	jujucrossmodel "github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/network"
	"github.com/juju/juju/permission"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
)

const (
	offerCall       = "offerCall"
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

func (m *stubApplicationOffers) Remove(url string) error {
	m.AddCall(removeOfferCall)
	panic("not implemented")
}

func (m *stubApplicationOffers) ApplicationOffer(name string) (*jujucrossmodel.ApplicationOffer, error) {
	m.AddCall(offerCall)
	panic("not implemented")
}

type mockEnviron struct {
	environs.NetworkingEnviron

	stub      jtesting.Stub
	spaceInfo *environs.ProviderSpaceInfo
}

func (e *mockEnviron) ProviderSpaceInfo(space *network.SpaceInfo) (*environs.ProviderSpaceInfo, error) {
	e.stub.MethodCall(e, "ProviderSpaceInfo", space)
	if e.spaceInfo == nil || space.Name != e.spaceInfo.Name {
		return nil, errors.NotFoundf("space %s", space.Name)
	}
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

func (m *mockModel) UUID() string {
	return m.uuid
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
	meta *charm.Meta
}

func (m *mockCharm) Meta() *charm.Meta {
	return m.meta
}

func (m *mockCharm) StoragePath() string {
	return "storage-path"
}

type mockApplication struct {
	name      string
	charm     *mockCharm
	curl      *charm.URL
	endpoints []state.Endpoint
	bindings  map[string]string
}

func (m *mockApplication) Name() string {
	return m.name
}

func (m *mockApplication) Charm() (applicationoffers.Charm, bool, error) {
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

type mockRemoteApplication struct {
	name           string
	sourceModelTag names.ModelTag
	endpoints      []state.Endpoint
	bindings       map[string]string
	spaces         []state.RemoteSpace
	offerName      string
	offerURL       string
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

type mockConnectionStatus struct {
	count int
}

func (m *mockConnectionStatus) ConnectionCount() int {
	return m.count
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

type offerAccess struct {
	user  names.UserTag
	offer names.ApplicationOfferTag
}

type mockState struct {
	modelUUID         string
	model             applicationoffers.Model
	allmodels         []applicationoffers.Model
	users             set.Strings
	applications      map[string]applicationoffers.Application
	applicationOffers map[string]jujucrossmodel.ApplicationOffer
	spaces            map[string]applicationoffers.Space
	connStatus        applicationoffers.RemoteConnectionStatus
	accessPerms       map[offerAccess]permission.Access
}

func (m *mockState) ControllerTag() names.ControllerTag {
	return testing.ControllerTag
}

func (m *mockState) Charm(*charm.URL) (applicationoffers.Charm, error) {
	return &mockCharm{}, nil
}

func (m *mockState) Application(name string) (applicationoffers.Application, error) {
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

func (m *mockState) AllModels() ([]applicationoffers.Model, error) {
	if len(m.allmodels) > 0 {
		return m.allmodels, nil
	}
	return []applicationoffers.Model{m.model}, nil
}

func (m *mockState) RemoteConnectionStatus(offerName string) (applicationoffers.RemoteConnectionStatus, error) {
	return m.connStatus, nil
}

func (m *mockState) GetOfferAccess(offer names.ApplicationOfferTag, user names.UserTag) (permission.Access, error) {
	access, ok := m.accessPerms[offerAccess{user: user, offer: offer}]
	if !ok {
		return "", errors.NotFoundf("offer access for %v", user)
	}
	return access, nil
}

func (m *mockState) CreateOfferAccess(offer names.ApplicationOfferTag, user names.UserTag, access permission.Access) error {
	if !m.users.Contains(user.Name()) {
		return errors.NotFoundf("user %q", user.Name())
	}
	if _, ok := m.accessPerms[offerAccess{user: user, offer: offer}]; ok {
		return errors.NewAlreadyExists(nil, fmt.Sprintf("offer user %s", user.Name()))
	}
	m.accessPerms[offerAccess{user: user, offer: offer}] = access
	return nil
}

func (m *mockState) UpdateOfferAccess(offer names.ApplicationOfferTag, user names.UserTag, access permission.Access) error {
	if !m.users.Contains(user.Name()) {
		return errors.NotFoundf("user %q", user.Name())
	}
	if _, ok := m.accessPerms[offerAccess{user: user, offer: offer}]; !ok {
		return errors.NewNotFound(nil, fmt.Sprintf("offer user %s", user.Name()))
	}
	m.accessPerms[offerAccess{user: user, offer: offer}] = access
	return nil
}

func (m *mockState) RemoveOfferAccess(offer names.ApplicationOfferTag, user names.UserTag) error {
	if !m.users.Contains(user.Name()) {
		return errors.NewNotFound(nil, fmt.Sprintf("offer user %q does not exist", user.Name()))
	}
	delete(m.accessPerms, offerAccess{user: user, offer: offer})
	return nil
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
