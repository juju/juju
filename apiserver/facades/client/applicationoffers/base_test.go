// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package applicationoffers_test

import (
	jtesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/crossmodel"
	"github.com/juju/juju/apiserver/facades/client/applicationoffers"
	"github.com/juju/juju/apiserver/testing"
	jujucrossmodel "github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/permission"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
)

const (
	addOffersBackendCall  = "addOffersCall"
	listOffersBackendCall = "listOffersCall"
)

type baseSuite struct {
	jtesting.IsolationSuite

	resources  *common.Resources
	authorizer *testing.FakeAuthorizer

	mockState         *mockState
	mockStatePool     *mockStatePool
	env               *mockEnviron
	bakery            *mockBakeryService
	authContext       *crossmodel.AuthContext
	applicationOffers *stubApplicationOffers
}

func (s *baseSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.resources = common.NewResources()
	s.authorizer = &testing.FakeAuthorizer{
		Tag:      names.NewUserTag("read"),
		AdminTag: names.NewUserTag("admin"),
	}

	s.env = &mockEnviron{}
	s.mockState = &mockState{
		modelUUID:         coretesting.ModelTag.Id(),
		users:             make(map[string]applicationoffers.User),
		applicationOffers: make(map[string]jujucrossmodel.ApplicationOffer),
		accessPerms:       make(map[offerAccess]permission.Access),
		spaces:            make(map[string]applicationoffers.Space),
		relations:         make(map[string]crossmodel.Relation),
		relationNetworks:  &mockRelationNetworks{},
	}
	s.mockStatePool = &mockStatePool{map[string]applicationoffers.Backend{s.mockState.modelUUID: s.mockState}}
}

func (s *baseSuite) addApplication(c *gc.C, name string) jujucrossmodel.ApplicationOffer {
	return jujucrossmodel.ApplicationOffer{
		OfferName:              "offer-" + name,
		OfferUUID:              "offer-" + name + "-uuid",
		ApplicationName:        name,
		Endpoints:              map[string]charm.Relation{"db": {Name: "db"}},
		ApplicationDescription: "applicaion description",
	}
}

func (s *baseSuite) setupOffers(c *gc.C, filterAppName string, filterWithEndpoints bool) {
	applicationName := "test"
	offerName := "hosted-db2"

	anOffer := jujucrossmodel.ApplicationOffer{
		OfferName:              offerName,
		OfferUUID:              offerName + "-uuid",
		ApplicationName:        applicationName,
		ApplicationDescription: "description",
		Endpoints:              map[string]charm.Relation{"db": {Name: "db2"}},
	}

	s.applicationOffers.listOffers = func(filters ...jujucrossmodel.ApplicationOfferFilter) ([]jujucrossmodel.ApplicationOffer, error) {
		c.Assert(filters, gc.HasLen, 1)
		expectedFilter := jujucrossmodel.ApplicationOfferFilter{
			OfferName:       offerName,
			ApplicationName: filterAppName,
		}
		if filterWithEndpoints {
			expectedFilter.Endpoints = []jujucrossmodel.EndpointFilterTerm{{
				Interface: "db2",
			}}
		}
		c.Assert(filters[0], jc.DeepEquals, expectedFilter)
		return []jujucrossmodel.ApplicationOffer{anOffer}, nil
	}
	ch := &mockCharm{meta: &charm.Meta{Description: "A pretty popular database"}}
	s.mockState.applications = map[string]crossmodel.Application{
		"test": &mockApplication{
			name:  "test",
			charm: ch, curl: charm.MustParseURL("db2-2"),
			bindings: map[string]string{"db2": "myspace"},
		},
	}
	s.mockState.model = &mockModel{
		uuid: coretesting.ModelTag.Id(), name: "prod", owner: "fred", modelType: state.ModelTypeIAAS}
	s.mockState.relations["hosted-db2:db wordpress:db"] = &mockRelation{
		id: 1,
		endpoint: state.Endpoint{
			ApplicationName: "test",
			Relation: charm.Relation{
				Name:      "db",
				Interface: "db2",
				Role:      "provider",
			},
		},
	}
	s.mockState.connections = []applicationoffers.OfferConnection{
		&mockOfferConnection{
			username:    "fred",
			modelUUID:   coretesting.ModelTag.Id(),
			relationKey: "hosted-db2:db wordpress:db",
			relationId:  1,
		},
	}
	s.mockState.spaces["myspace"] = &mockSpace{
		name:       "myspace",
		providerId: "juju-space-myspace",
		subnets: []applicationoffers.Subnet{
			&mockSubnet{cidr: "4.3.2.0/24", providerId: "juju-subnet-1", zones: []string{"az1"}},
		},
	}
	s.env.spaceInfo = &environs.ProviderSpaceInfo{
		SpaceInfo: network.SpaceInfo{
			Name:       "myspace",
			ProviderId: "juju-space-myspace",
			Subnets: []network.SubnetInfo{{
				CIDR:              "4.3.2.0/24",
				ProviderId:        "juju-subnet-1",
				AvailabilityZones: []string{"az1"},
			}},
		},
	}
}
