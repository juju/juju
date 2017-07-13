// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package applicationoffers_test

import (
	jtesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/set"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facades/client/applicationoffers"
	"github.com/juju/juju/apiserver/testing"
	jujucrossmodel "github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/network"
	"github.com/juju/juju/permission"
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
		users:             set.NewStrings(),
		applicationOffers: make(map[string]jujucrossmodel.ApplicationOffer),
		accessPerms:       make(map[offerAccess]permission.Access),
		spaces:            make(map[string]applicationoffers.Space),
	}
	s.mockStatePool = &mockStatePool{map[string]applicationoffers.Backend{s.mockState.modelUUID: s.mockState}}
}

func (s *baseSuite) addApplication(c *gc.C, name string) jujucrossmodel.ApplicationOffer {
	return jujucrossmodel.ApplicationOffer{
		OfferName:              "offer-" + name,
		ApplicationName:        name,
		Endpoints:              map[string]charm.Relation{"db": {Name: "db"}},
		ApplicationDescription: "applicaion description",
	}
}

func (s *baseSuite) setupOffers(c *gc.C, filterAppName string) {
	applicationName := "test"
	offerName := "hosted-db2"

	anOffer := jujucrossmodel.ApplicationOffer{
		OfferName:              offerName,
		ApplicationName:        applicationName,
		ApplicationDescription: "description",
		Endpoints:              map[string]charm.Relation{"db": {Name: "db2"}},
	}

	s.applicationOffers.listOffers = func(filters ...jujucrossmodel.ApplicationOfferFilter) ([]jujucrossmodel.ApplicationOffer, error) {
		c.Assert(filters, gc.HasLen, 1)
		c.Assert(filters[0], jc.DeepEquals, jujucrossmodel.ApplicationOfferFilter{
			OfferName:       offerName,
			ApplicationName: filterAppName,
		})
		return []jujucrossmodel.ApplicationOffer{anOffer}, nil
	}
	ch := &mockCharm{meta: &charm.Meta{Description: "A pretty popular database"}}
	s.mockState.applications = map[string]applicationoffers.Application{
		"test": &mockApplication{
			charm: ch, curl: charm.MustParseURL("db2-2"),
			bindings: map[string]string{"db2": "myspace"},
		},
	}
	s.mockState.model = &mockModel{uuid: coretesting.ModelTag.Id(), name: "prod", owner: "fred"}
	s.mockState.connStatus = &mockConnectionStatus{count: 5}
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
