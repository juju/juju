// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package applicationoffers_test

import (
	"context"
	"time"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/checkers"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	jtesting "github.com/juju/testing"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/crossmodel"
	"github.com/juju/juju/apiserver/facades/client/applicationoffers"
	jujucrossmodel "github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/domain/relation"
	"github.com/juju/juju/internal/testing"
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

	applicationOffer func(name string) (*jujucrossmodel.ApplicationOffer, error)
	addOffer         func(offer jujucrossmodel.AddApplicationOfferArgs) (*jujucrossmodel.ApplicationOffer, error)
	updateOffer      func(offer jujucrossmodel.AddApplicationOfferArgs) (*jujucrossmodel.ApplicationOffer, error)
	listOffers       func(filters ...jujucrossmodel.ApplicationOfferFilter) ([]jujucrossmodel.ApplicationOffer, error)
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
	return m.updateOffer(offer)
}

func (m *stubApplicationOffers) Remove(url string, force bool) error {
	m.AddCall(removeOfferCall)
	panic("not implemented")
}

func (m *stubApplicationOffers) ApplicationOffer(name string) (*jujucrossmodel.ApplicationOffer, error) {
	m.AddCall(offerCall)
	return m.applicationOffer(name)
}

func (m *stubApplicationOffers) ApplicationOfferForUUID(uuid string) (*jujucrossmodel.ApplicationOffer, error) {
	m.AddCall(offerCallUUID)
	panic("not implemented")
}

type mockApplication struct {
	crossmodel.Application
	name      string
	curl      string
	endpoints []relation.Endpoint
	bindings  map[string]string
}

func (m *mockApplication) Name() string {
	return m.name
}

func (m *mockApplication) CharmURL() (curl *string, force bool) {
	return &m.curl, true
}

func (m *mockApplication) Endpoints() ([]relation.Endpoint, error) {
	return m.endpoints, nil
}

func (m *mockApplication) EndpointBindings() (crossmodel.Bindings, error) {
	return &mockBindings{bMap: m.bindings}, nil
}

type mockBindings struct {
	bMap map[string]string
}

// TODO (stickupkid): This implementation is wrong, we should move to a newer
// gomock style setup.
func (b *mockBindings) MapWithSpaceNames(network.SpaceInfos) (map[string]string, error) {
	return b.bMap, nil
}

type mockRelation struct {
	crossmodel.Relation
	id       int
	endpoint relation.Endpoint
}

func (m *mockRelation) Status() (status.StatusInfo, error) {
	return status.StatusInfo{Status: status.Joined}, nil
}

func (m *mockRelation) Endpoint(appName string) (relation.Endpoint, error) {
	if m.endpoint.ApplicationName != appName {
		return relation.Endpoint{}, errors.NotFoundf("endpoint for %q", appName)
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

type mockState struct {
	crossmodel.Backend
	common.APIAddressAccessor
	AdminTag          names.UserTag
	applications      map[string]crossmodel.Application
	applicationOffers map[string]jujucrossmodel.ApplicationOffer
	relations         map[string]crossmodel.Relation
	connections       []applicationoffers.OfferConnection
	relationNetworks  crossmodel.RelationNetworks
}

func (m *mockState) GetAddressAndCertGetter() common.APIAddressAccessor {
	return m
}

func (m *mockState) ControllerTag() names.ControllerTag {
	return testing.ControllerTag
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

type mockRelationNetworks struct {
	crossmodel.RelationNetworks
}

func (m *mockRelationNetworks) CIDRS() []string {
	return []string{"192.168.1.0/32", "10.0.0.0/8"}
}

func (m *mockState) IngressNetworks(relationKey string) (crossmodel.RelationNetworks, error) {
	if m.relationNetworks == nil {
		return nil, errors.NotFoundf("ingress networks")
	}
	return m.relationNetworks, nil
}

func (m *mockState) UpdateOfferAccessCleanup(_ names.ApplicationOfferTag, _ names.UserTag, _ permission.Access, _ bool) error {
	return nil
}

func (m *mockState) RemoveOfferAccessCleanup(_ names.ApplicationOfferTag, _ names.UserTag, _ bool) error {
	return nil
}

func (m *mockState) AllSpaceInfos() (network.SpaceInfos, error) {
	return nil, nil
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

type mockBakeryService struct {
	authentication.ExpirableStorageBakery
	jtesting.Stub
	caveats map[string][]checkers.Caveat
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
	s.caveats["id"] = caveats
	return bakery.NewLegacyMacaroon(mac)
}

func (s *mockBakeryService) ExpireStorageAfter(when time.Duration) (authentication.ExpirableStorageBakery, error) {
	s.MethodCall(s, "ExpireStorageAfter", when)
	return s, nil
}

func getFakeControllerInfo(ctx context.Context) ([]string, string, error) {
	return []string{"192.168.1.1:17070"}, testing.CACert, nil
}
