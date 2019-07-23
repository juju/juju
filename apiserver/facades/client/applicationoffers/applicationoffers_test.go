// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package applicationoffers_test

import (
	"fmt"
	"strings"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/names.v2"
	"gopkg.in/macaroon-bakery.v2-unstable/bakery/checkers"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/crossmodel"
	"github.com/juju/juju/apiserver/facades/client/applicationoffers"
	"github.com/juju/juju/apiserver/params"
	jujucrossmodel "github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/permission"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
)

type applicationOffersSuite struct {
	baseSuite
	api *applicationoffers.OffersAPIV2
}

var _ = gc.Suite(&applicationOffersSuite{})

func (s *applicationOffersSuite) SetUpTest(c *gc.C) {
	s.baseSuite.SetUpTest(c)
	s.applicationOffers = &stubApplicationOffers{}
	getApplicationOffers := func(interface{}) jujucrossmodel.ApplicationOffers {
		return s.applicationOffers
	}

	resources := common.NewResources()
	resources.RegisterNamed("dataDir", common.StringResource(c.MkDir()))

	getEnviron := func(modelUUID string) (environs.Environ, error) {
		return s.env, nil
	}
	var err error
	s.bakery = &mockBakeryService{caveats: make(map[string][]checkers.Caveat)}
	s.authContext, err = crossmodel.NewAuthContext(&mockCommonStatePool{s.mockStatePool}, s.bakery, s.bakery)
	c.Assert(err, jc.ErrorIsNil)
	apiV1, err := applicationoffers.CreateOffersAPI(
		getApplicationOffers, getEnviron, getFakeControllerInfo,
		s.mockState, s.mockStatePool, s.authorizer, resources, s.authContext,
		context.NewCloudCallContext(),
	)
	c.Assert(err, jc.ErrorIsNil)
	s.api = &applicationoffers.OffersAPIV2{OffersAPI: apiV1}
}

func (s *applicationOffersSuite) assertOffer(c *gc.C, expectedErr error) {
	applicationName := "test"
	s.addApplication(c, applicationName)
	one := params.AddApplicationOffer{
		ModelTag:        testing.ModelTag.String(),
		OfferName:       "offer-test",
		ApplicationName: applicationName,
		Endpoints:       map[string]string{"db": "db"},
	}
	all := params.AddApplicationOffers{Offers: []params.AddApplicationOffer{one}}
	s.applicationOffers.addOffer = func(offer jujucrossmodel.AddApplicationOfferArgs) (*jujucrossmodel.ApplicationOffer, error) {
		c.Assert(offer.OfferName, gc.Equals, one.OfferName)
		c.Assert(offer.ApplicationName, gc.Equals, one.ApplicationName)
		c.Assert(offer.ApplicationDescription, gc.Equals, "A pretty popular blog engine")
		c.Assert(offer.Owner, gc.Equals, "admin")
		c.Assert(offer.HasRead, gc.DeepEquals, []string{"everyone@external"})
		return &jujucrossmodel.ApplicationOffer{}, nil
	}
	ch := &mockCharm{meta: &charm.Meta{Description: "A pretty popular blog engine"}}
	s.mockState.applications = map[string]crossmodel.Application{
		applicationName: &mockApplication{charm: ch, bindings: map[string]string{"db": "myspace"}},
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

	errs, err := s.api.Offer(all)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errs.Results, gc.HasLen, len(all.Offers))
	if expectedErr != nil {
		c.Assert(errs.Results[0].Error, gc.ErrorMatches, expectedErr.Error())
		return
	}
	c.Assert(errs.Results[0].Error, gc.IsNil)
	s.applicationOffers.CheckCallNames(c, addOffersBackendCall)
}

func (s *applicationOffersSuite) TestOffer(c *gc.C) {
	s.authorizer.Tag = names.NewUserTag("admin")
	s.assertOffer(c, nil)
}

func (s *applicationOffersSuite) TestOfferPermission(c *gc.C) {
	s.authorizer.Tag = names.NewUserTag("mary")
	s.assertOffer(c, common.ErrPerm)
}

func (s *applicationOffersSuite) TestOfferSomeFail(c *gc.C) {
	s.authorizer.Tag = names.NewUserTag("admin")
	s.addApplication(c, "one")
	s.addApplication(c, "two")
	s.addApplication(c, "paramsfail")
	one := params.AddApplicationOffer{
		ModelTag:        testing.ModelTag.String(),
		OfferName:       "offer-one",
		ApplicationName: "one",
		Endpoints:       map[string]string{"db": "db"},
	}
	bad := params.AddApplicationOffer{
		ModelTag:        testing.ModelTag.String(),
		OfferName:       "offer-bad",
		ApplicationName: "notthere",
		Endpoints:       map[string]string{"db": "db"},
	}
	bad2 := params.AddApplicationOffer{
		ModelTag:        testing.ModelTag.String(),
		OfferName:       "offer-bad",
		ApplicationName: "paramsfail",
		Endpoints:       map[string]string{"db": "db"},
	}
	two := params.AddApplicationOffer{
		ModelTag:        testing.ModelTag.String(),
		OfferName:       "offer-two",
		ApplicationName: "two",
		Endpoints:       map[string]string{"db": "db"},
	}
	all := params.AddApplicationOffers{Offers: []params.AddApplicationOffer{one, bad, bad2, two}}
	s.applicationOffers.addOffer = func(offer jujucrossmodel.AddApplicationOfferArgs) (*jujucrossmodel.ApplicationOffer, error) {
		if offer.ApplicationName == "paramsfail" {
			return nil, errors.New("params fail")
		}
		return &jujucrossmodel.ApplicationOffer{}, nil
	}
	ch := &mockCharm{meta: &charm.Meta{Description: "A pretty popular blog engine"}}
	s.mockState.applications = map[string]crossmodel.Application{
		"one":        &mockApplication{charm: ch, bindings: map[string]string{"db": "myspace"}},
		"two":        &mockApplication{charm: ch, bindings: map[string]string{"db": "myspace"}},
		"paramsfail": &mockApplication{charm: ch, bindings: map[string]string{"db": "myspace"}},
	}

	errs, err := s.api.Offer(all)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errs.Results, gc.HasLen, len(all.Offers))
	c.Assert(errs.Results[0].Error, gc.IsNil)
	c.Assert(errs.Results[3].Error, gc.IsNil)
	c.Assert(errs.Results[1].Error, gc.ErrorMatches, `getting offered application notthere: application "notthere" not found`)
	c.Assert(errs.Results[2].Error, gc.ErrorMatches, `params fail`)
	s.applicationOffers.CheckCallNames(c, addOffersBackendCall, addOffersBackendCall, addOffersBackendCall)
}

func (s *applicationOffersSuite) TestOfferError(c *gc.C) {
	s.authorizer.Tag = names.NewUserTag("admin")
	applicationName := "test"
	s.addApplication(c, applicationName)
	one := params.AddApplicationOffer{
		ModelTag:        testing.ModelTag.String(),
		OfferName:       "offer-test",
		ApplicationName: applicationName,
		Endpoints:       map[string]string{"db": "db"},
	}
	all := params.AddApplicationOffers{Offers: []params.AddApplicationOffer{one}}

	msg := "fail"

	s.applicationOffers.addOffer = func(offer jujucrossmodel.AddApplicationOfferArgs) (*jujucrossmodel.ApplicationOffer, error) {
		return nil, errors.New(msg)
	}
	ch := &mockCharm{meta: &charm.Meta{Description: "A pretty popular blog engine"}}
	s.mockState.applications = map[string]crossmodel.Application{
		applicationName: &mockApplication{charm: ch, bindings: map[string]string{"db": "myspace"}},
	}

	errs, err := s.api.Offer(all)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errs.Results, gc.HasLen, len(all.Offers))
	c.Assert(errs.Results[0].Error, gc.ErrorMatches, fmt.Sprintf(".*%v.*", msg))
	s.applicationOffers.CheckCallNames(c, addOffersBackendCall)
}

func (s *applicationOffersSuite) assertList(c *gc.C, expectedErr error, expectedCIDRS []string) {
	s.mockState.users["mary"] = &mockUser{"mary"}
	s.mockState.CreateOfferAccess(
		names.NewApplicationOfferTag("hosted-db2"),
		names.NewUserTag("mary"), permission.ConsumeAccess)
	filter := params.OfferFilters{
		Filters: []params.OfferFilter{
			{
				OwnerName:       "fred",
				ModelName:       "prod",
				OfferName:       "hosted-db2",
				ApplicationName: "test",
			},
		},
	}
	found, err := s.api.ListApplicationOffers(filter)
	if expectedErr != nil {
		c.Assert(errors.Cause(err), gc.ErrorMatches, expectedErr.Error())
		return
	}
	c.Assert(err, jc.ErrorIsNil)

	expectedOfferDetails := []params.ApplicationOfferAdminDetails{
		{
			ApplicationOfferDetails: params.ApplicationOfferDetails{
				SourceModelTag:         testing.ModelTag.String(),
				ApplicationDescription: "description",
				OfferName:              "hosted-db2",
				OfferUUID:              "hosted-db2-uuid",
				OfferURL:               "fred/prod.hosted-db2",
				Endpoints:              []params.RemoteEndpoint{{Name: "db"}},
				Bindings:               map[string]string{"db2": "myspace"},
				Spaces: []params.RemoteSpace{
					{
						Name:       "myspace",
						ProviderId: "juju-space-myspace",
						Subnets:    []params.Subnet{{CIDR: "4.3.2.0/24", ProviderId: "juju-subnet-1", Zones: []string{"az1"}}},
					},
				},
				Users: []params.OfferUserDetails{
					{UserName: "admin", DisplayName: "", Access: "admin"},
					{UserName: "mary", DisplayName: "mary", Access: "consume"},
				},
			},
			ApplicationName: "test",
			CharmURL:        "cs:db2-2",
			Connections: []params.OfferConnection{{
				SourceModelTag: testing.ModelTag.String(),
				RelationId:     1,
				Endpoint:       "db",
				Username:       "fred",
				Status:         params.EntityStatus{Status: "joined"},
				IngressSubnets: expectedCIDRS,
			}},
		},
	}
	if s.mockState.model.modelType == state.ModelTypeCAAS {
		expectedOfferDetails[0].Spaces = nil
		expectedOfferDetails[0].Bindings = nil
	}
	c.Assert(found, jc.DeepEquals, params.QueryApplicationOffersResults{
		expectedOfferDetails,
	})
	s.applicationOffers.CheckCallNames(c, listOffersBackendCall)
	if s.mockState.model.modelType == state.ModelTypeCAAS {
		s.env.stub.CheckNoCalls(c)
		return
	}
	s.env.stub.CheckCallNames(c, "ProviderSpaceInfo")
	s.env.stub.CheckCall(c, 0, "ProviderSpaceInfo", &network.SpaceInfo{
		Name:       "myspace",
		ProviderId: "juju-space-myspace",
		Subnets: []network.SubnetInfo{{
			CIDR:              "4.3.2.0/24",
			ProviderId:        "juju-subnet-1",
			AvailabilityZones: []string{"az1"},
		}},
	})
}

func (s *applicationOffersSuite) TestList(c *gc.C) {
	s.authorizer.Tag = names.NewUserTag("admin")
	s.setupOffers(c, "test", false)
	s.assertList(c, nil, []string{"192.168.1.0/32", "10.0.0.0/8"})
}

func (s *applicationOffersSuite) TestListCAAS(c *gc.C) {
	s.authorizer.Tag = names.NewUserTag("admin")
	s.setupOffers(c, "test", false)
	s.mockState.model.modelType = state.ModelTypeCAAS
	s.assertList(c, nil, []string{"192.168.1.0/32", "10.0.0.0/8"})
}

func (s *applicationOffersSuite) TestListNoRelationNetworks(c *gc.C) {
	s.authorizer.Tag = names.NewUserTag("admin")
	s.mockState.relationNetworks = nil
	s.setupOffers(c, "test", false)
	s.assertList(c, nil, nil)
}

func (s *applicationOffersSuite) TestListPermission(c *gc.C) {
	s.setupOffers(c, "test", false)
	s.assertList(c, common.ErrPerm, nil)
}

func (s *applicationOffersSuite) TestListError(c *gc.C) {
	s.setupOffers(c, "test", false)
	s.authorizer.Tag = names.NewUserTag("admin")
	filter := params.OfferFilters{
		Filters: []params.OfferFilter{
			{
				OwnerName:       "fred",
				ModelName:       "prod",
				OfferName:       "hosted-db2",
				ApplicationName: "test",
			},
		},
	}
	msg := "fail"

	s.applicationOffers.listOffers = func(filters ...jujucrossmodel.ApplicationOfferFilter) ([]jujucrossmodel.ApplicationOffer, error) {
		return nil, errors.New(msg)
	}

	_, err := s.api.ListApplicationOffers(filter)
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf(".*%v.*", msg))
	s.applicationOffers.CheckCallNames(c, listOffersBackendCall)
}

func (s *applicationOffersSuite) TestListFilterRequiresModel(c *gc.C) {
	s.setupOffers(c, "test", false)
	filter := params.OfferFilters{
		Filters: []params.OfferFilter{
			{
				OfferName:       "hosted-db2",
				ApplicationName: "test",
			},
		},
	}
	_, err := s.api.ListApplicationOffers(filter)
	c.Assert(err, gc.ErrorMatches, "application offer filter must specify a model name")
}

func (s *applicationOffersSuite) TestListRequiresFilter(c *gc.C) {
	s.setupOffers(c, "test", false)
	_, err := s.api.ListApplicationOffers(params.OfferFilters{})
	c.Assert(err, gc.ErrorMatches, "at least one offer filter is required")
}

func (s *applicationOffersSuite) assertShow(c *gc.C, url string, expected []params.ApplicationOfferResult) {
	s.setupOffers(c, "", false)
	s.mockState.users["mary"] = &mockUser{"mary"}
	s.mockState.CreateOfferAccess(
		names.NewApplicationOfferTag("hosted-db2"),
		names.NewUserTag("mary"), permission.ConsumeAccess)
	filter := params.OfferURLs{[]string{url}}

	found, err := s.api.ApplicationOffers(filter)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.Results, jc.DeepEquals, expected)
	s.applicationOffers.CheckCallNames(c, listOffersBackendCall)
	if len(expected) > 0 {
		return
	}
	s.env.stub.CheckCallNames(c, "ProviderSpaceInfo")
	s.env.stub.CheckCall(c, 0, "ProviderSpaceInfo", &network.SpaceInfo{
		Name:       "myspace",
		ProviderId: "juju-space-myspace",
		Subnets: []network.SubnetInfo{{
			CIDR:              "4.3.2.0/24",
			ProviderId:        "juju-subnet-1",
			AvailabilityZones: []string{"az1"},
		}},
	})
}

func (s *applicationOffersSuite) TestShow(c *gc.C) {
	expected := []params.ApplicationOfferResult{{
		Result: &params.ApplicationOfferAdminDetails{
			ApplicationOfferDetails: params.ApplicationOfferDetails{
				SourceModelTag:         testing.ModelTag.String(),
				ApplicationDescription: "description",
				OfferURL:               "fred/prod.hosted-db2",
				OfferName:              "hosted-db2",
				OfferUUID:              "hosted-db2-uuid",
				Endpoints:              []params.RemoteEndpoint{{Name: "db"}},
				Bindings:               map[string]string{"db2": "myspace"},
				Spaces: []params.RemoteSpace{
					{
						Name:       "myspace",
						ProviderId: "juju-space-myspace",
						Subnets:    []params.Subnet{{CIDR: "4.3.2.0/24", ProviderId: "juju-subnet-1", Zones: []string{"az1"}}},
					},
				},
				Users: []params.OfferUserDetails{
					{UserName: "fred", DisplayName: "", Access: "admin"},
					{UserName: "mary", DisplayName: "mary", Access: "consume"},
				},
			},
			ApplicationName: "test",
			CharmURL:        "cs:db2-2",
			Connections: []params.OfferConnection{{
				SourceModelTag: "model-deadbeef-0bad-400d-8000-4b1d0d06f00d",
				RelationId:     1, Username: "fred", Endpoint: "db",
				Status:         params.EntityStatus{Status: "joined"},
				IngressSubnets: []string{"192.168.1.0/32", "10.0.0.0/8"},
			}},
		},
	}}
	s.authorizer.Tag = names.NewUserTag("admin")
	expected[0].Result.Users[0].UserName = "admin"
	s.assertShow(c, "fred/prod.hosted-db2", expected)
	// Again with an unqualified model path.
	s.authorizer.AdminTag = names.NewUserTag("fred")
	s.authorizer.Tag = s.authorizer.AdminTag
	expected[0].Result.Users[0].UserName = "fred"
	s.applicationOffers.ResetCalls()
	s.assertShow(c, "prod.hosted-db2", expected)
}

func (s *applicationOffersSuite) TestShowNoPermission(c *gc.C) {
	s.mockState.users["someone"] = &mockUser{"someone"}
	user := names.NewUserTag("someone")
	offer := names.NewApplicationOfferTag("hosted-db2")
	err := s.mockState.CreateOfferAccess(offer, user, permission.NoAccess)
	c.Assert(err, jc.ErrorIsNil)

	s.authorizer.Tag = user
	expected := []params.ApplicationOfferResult{{
		Error: common.ServerError(errors.NotFoundf("application offer %q", "fred/prod.hosted-db2")),
	}}
	s.assertShow(c, "fred/prod.hosted-db2", expected)
}

func (s *applicationOffersSuite) TestShowPermission(c *gc.C) {
	user := names.NewUserTag("someone")
	s.authorizer.Tag = user
	expected := []params.ApplicationOfferResult{{
		Result: &params.ApplicationOfferAdminDetails{
			ApplicationOfferDetails: params.ApplicationOfferDetails{
				SourceModelTag:         testing.ModelTag.String(),
				ApplicationDescription: "description",
				OfferURL:               "fred/prod.hosted-db2",
				OfferName:              "hosted-db2",
				OfferUUID:              "hosted-db2-uuid",
				Endpoints:              []params.RemoteEndpoint{{Name: "db"}},
				Bindings:               map[string]string{"db2": "myspace"},
				Spaces: []params.RemoteSpace{
					{
						Name:       "myspace",
						ProviderId: "juju-space-myspace",
						Subnets:    []params.Subnet{{CIDR: "4.3.2.0/24", ProviderId: "juju-subnet-1", Zones: []string{"az1"}}},
					},
				},
				Users: []params.OfferUserDetails{
					{UserName: "someone", DisplayName: "someone", Access: "read"},
				},
			},
		}}}
	s.mockState.users[user.Name()] = &mockUser{user.Name()}
	s.mockState.CreateOfferAccess(names.NewApplicationOfferTag("hosted-db2"), user, permission.ReadAccess)
	s.assertShow(c, "fred/prod.hosted-db2", expected)
}

func (s *applicationOffersSuite) TestShowError(c *gc.C) {
	url := "fred/prod.hosted-db2"
	filter := params.OfferURLs{[]string{url}}
	msg := "fail"

	s.applicationOffers.listOffers = func(filters ...jujucrossmodel.ApplicationOfferFilter) ([]jujucrossmodel.ApplicationOffer, error) {
		return nil, errors.New(msg)
	}
	s.mockState.model = &mockModel{uuid: testing.ModelTag.Id(), name: "prod", owner: "fred", modelType: state.ModelTypeIAAS}

	_, err := s.api.ApplicationOffers(filter)
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf(".*%v.*", msg))
	s.applicationOffers.CheckCallNames(c, listOffersBackendCall)
}

func (s *applicationOffersSuite) TestShowNotFound(c *gc.C) {
	urls := []string{"fred/prod.hosted-db2"}
	filter := params.OfferURLs{urls}

	s.applicationOffers.listOffers = func(filters ...jujucrossmodel.ApplicationOfferFilter) ([]jujucrossmodel.ApplicationOffer, error) {
		return nil, nil
	}
	s.mockState.model = &mockModel{uuid: testing.ModelTag.Id(), name: "prod", owner: "fred", modelType: state.ModelTypeIAAS}

	found, err := s.api.ApplicationOffers(filter)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.Results, gc.HasLen, 1)
	c.Assert(found.Results[0].Error.Error(), gc.Matches, `application offer "fred/prod.hosted-db2" not found`)
	s.applicationOffers.CheckCallNames(c, listOffersBackendCall)
}

func (s *applicationOffersSuite) TestShowRejectsEndpoints(c *gc.C) {
	urls := []string{"fred/prod.hosted-db2:db"}
	filter := params.OfferURLs{urls}
	s.mockState.model = &mockModel{uuid: testing.ModelTag.Id(), name: "prod", owner: "fred", modelType: state.ModelTypeIAAS}

	found, err := s.api.ApplicationOffers(filter)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.Results, gc.HasLen, 1)
	c.Assert(found.Results[0].Error.Message, gc.Equals, `remote application "fred/prod.hosted-db2:db" shouldn't include endpoint`)
}

func (s *applicationOffersSuite) TestShowErrorMsgMultipleURLs(c *gc.C) {
	urls := []string{"fred/prod.hosted-mysql", "fred/test.hosted-db2"}
	filter := params.OfferURLs{urls}

	s.applicationOffers.listOffers = func(filters ...jujucrossmodel.ApplicationOfferFilter) ([]jujucrossmodel.ApplicationOffer, error) {
		return nil, nil
	}
	s.mockState.model = &mockModel{uuid: testing.ModelTag.Id(), name: "prod", owner: "fred", modelType: state.ModelTypeIAAS}
	anotherModel := &mockModel{uuid: "uuid2", name: "test", owner: "fred", modelType: state.ModelTypeIAAS}
	s.mockStatePool.st["uuid2"] = &mockState{
		modelUUID: "uuid2",
		model:     anotherModel,
	}
	s.mockState.allmodels = []applicationoffers.Model{s.mockState.model, anotherModel}

	found, err := s.api.ApplicationOffers(filter)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.Results, gc.HasLen, 2)
	c.Assert(found.Results[0].Error.Error(), gc.Matches, `application offer "fred/prod.hosted-mysql" not found`)
	c.Assert(found.Results[1].Error.Error(), gc.Matches, `application offer "fred/test.hosted-db2" not found`)
	s.applicationOffers.CheckCallNames(c, listOffersBackendCall, listOffersBackendCall)
}

func (s *applicationOffersSuite) TestShowFoundMultiple(c *gc.C) {
	name := "test"
	url := "fred/prod.hosted-" + name
	anOffer := jujucrossmodel.ApplicationOffer{
		ApplicationName:        name,
		ApplicationDescription: "description",
		OfferName:              "hosted-" + name,
		OfferUUID:              "hosted-" + name + "-uuid",
		Endpoints:              map[string]charm.Relation{"db": {Name: "db"}},
	}

	name2 := "testagain"
	url2 := "mary/test.hosted-" + name2
	anOffer2 := jujucrossmodel.ApplicationOffer{
		ApplicationName:        name2,
		ApplicationDescription: "description2",
		OfferName:              "hosted-" + name2,
		OfferUUID:              "hosted-" + name2 + "-uuid",
		Endpoints:              map[string]charm.Relation{"db2": {Name: "db2"}},
	}

	filter := params.OfferURLs{[]string{url, url2}}

	s.applicationOffers.listOffers = func(filters ...jujucrossmodel.ApplicationOfferFilter) ([]jujucrossmodel.ApplicationOffer, error) {
		c.Assert(filters, gc.HasLen, 1)
		if filters[0].OfferName == "hosted-test" {
			return []jujucrossmodel.ApplicationOffer{anOffer}, nil
		}
		return []jujucrossmodel.ApplicationOffer{anOffer2}, nil
	}
	ch := &mockCharm{meta: &charm.Meta{Description: "A pretty popular blog engine"}}
	s.mockState.applications = map[string]crossmodel.Application{
		"test": &mockApplication{
			charm: ch, curl: charm.MustParseURL("db2-2"), bindings: map[string]string{"db": "myspace"}},
	}

	model := &mockModel{uuid: testing.ModelTag.Id(), name: "prod", owner: "fred", modelType: state.ModelTypeIAAS}
	anotherModel := &mockModel{uuid: "uuid2", name: "test", owner: "mary", modelType: state.ModelTypeIAAS}

	s.mockState.model = model
	s.mockState.allmodels = []applicationoffers.Model{model, anotherModel}
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

	user := names.NewUserTag("someone")
	s.authorizer.Tag = user
	s.mockState.users[user.Name()] = &mockUser{user.Name()}
	s.mockState.CreateOfferAccess(names.NewApplicationOfferTag("hosted-test"), user, permission.ReadAccess)

	anotherState := &mockState{
		modelUUID:   "uuid2",
		users:       make(map[string]applicationoffers.User),
		accessPerms: make(map[offerAccess]permission.Access),
		spaces:      make(map[string]applicationoffers.Space),
		model:       anotherModel,
	}
	anotherState.applications = map[string]crossmodel.Application{
		"testagain": &mockApplication{
			charm: ch, curl: charm.MustParseURL("mysql-2"), bindings: map[string]string{"db2": "anotherspace"}},
	}
	anotherState.spaces["anotherspace"] = &mockSpace{
		name:       "anotherspace",
		providerId: "juju-space-myspace",
		subnets: []applicationoffers.Subnet{
			&mockSubnet{cidr: "4.3.2.0/24", providerId: "juju-subnet-1", zones: []string{"az1"}},
		},
	}
	anotherState.users[user.Name()] = &mockUser{user.Name()}
	anotherState.CreateOfferAccess(names.NewApplicationOfferTag("hosted-testagain"), user, permission.ConsumeAccess)
	s.mockStatePool.st["uuid2"] = anotherState

	found, err := s.api.ApplicationOffers(filter)
	c.Assert(err, jc.ErrorIsNil)
	var results []params.ApplicationOfferAdminDetails
	for _, r := range found.Results {
		c.Assert(r.Error, gc.IsNil)
		results = append(results, *r.Result)
	}
	c.Assert(results, jc.DeepEquals, []params.ApplicationOfferAdminDetails{
		{
			ApplicationOfferDetails: params.ApplicationOfferDetails{
				SourceModelTag:         testing.ModelTag.String(),
				ApplicationDescription: "description",
				OfferName:              "hosted-" + name,
				OfferUUID:              "hosted-" + name + "-uuid",
				OfferURL:               url,
				Endpoints:              []params.RemoteEndpoint{{Name: "db"}},
				Bindings:               map[string]string{"db": "myspace"},
				Spaces: []params.RemoteSpace{
					{
						Name:       "myspace",
						ProviderId: "juju-space-myspace",
						Subnets:    []params.Subnet{{CIDR: "4.3.2.0/24", ProviderId: "juju-subnet-1", Zones: []string{"az1"}}},
					},
				},
				Users: []params.OfferUserDetails{
					{UserName: "someone", DisplayName: "someone", Access: "read"},
				},
			},
		}, {
			ApplicationOfferDetails: params.ApplicationOfferDetails{
				SourceModelTag:         "model-uuid2",
				ApplicationDescription: "description2",
				OfferName:              "hosted-" + name2,
				OfferUUID:              "hosted-" + name2 + "-uuid",
				OfferURL:               url2,
				Endpoints:              []params.RemoteEndpoint{{Name: "db2"}},
				Users: []params.OfferUserDetails{
					{UserName: "someone", DisplayName: "someone", Access: "consume"},
				}},
		},
	})
	s.applicationOffers.CheckCallNames(c, listOffersBackendCall, listOffersBackendCall)
}

func (s *applicationOffersSuite) assertFind(c *gc.C, expected []params.ApplicationOfferAdminDetails) {
	filter := params.OfferFilters{
		Filters: []params.OfferFilter{
			{
				OfferName: "hosted-db2",
				Endpoints: []params.EndpointFilterAttributes{{
					Interface: "db2",
				}},
			},
		},
	}
	found, err := s.api.FindApplicationOffers(filter)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found, jc.DeepEquals, params.QueryApplicationOffersResults{
		Results: expected,
	})
	s.applicationOffers.CheckCallNames(c, listOffersBackendCall)
	if len(expected) == 0 {
		return
	}
	s.env.stub.CheckCallNames(c, "ProviderSpaceInfo")
	s.env.stub.CheckCall(c, 0, "ProviderSpaceInfo", &network.SpaceInfo{
		Name:       "myspace",
		ProviderId: "juju-space-myspace",
		Subnets: []network.SubnetInfo{{
			CIDR:              "4.3.2.0/24",
			ProviderId:        "juju-subnet-1",
			AvailabilityZones: []string{"az1"},
		}},
	})
}

func (s *applicationOffersSuite) TestFid(c *gc.C) {
	s.setupOffers(c, "", true)
	s.authorizer.Tag = names.NewUserTag("admin")
	expected := []params.ApplicationOfferAdminDetails{
		{
			ApplicationOfferDetails: params.ApplicationOfferDetails{
				SourceModelTag:         testing.ModelTag.String(),
				ApplicationDescription: "description",
				OfferName:              "hosted-db2",
				OfferUUID:              "hosted-db2-uuid",
				OfferURL:               "fred/prod.hosted-db2",
				Endpoints:              []params.RemoteEndpoint{{Name: "db"}},
				Bindings:               map[string]string{"db2": "myspace"},
				Spaces: []params.RemoteSpace{
					{
						Name:       "myspace",
						ProviderId: "juju-space-myspace",
						Subnets:    []params.Subnet{{CIDR: "4.3.2.0/24", ProviderId: "juju-subnet-1", Zones: []string{"az1"}}},
					},
				},
				Users: []params.OfferUserDetails{
					{UserName: "admin", DisplayName: "", Access: "admin"},
				}},
			ApplicationName: "test",
			CharmURL:        "cs:db2-2",
			Connections: []params.OfferConnection{{
				SourceModelTag: "model-deadbeef-0bad-400d-8000-4b1d0d06f00d",
				RelationId:     1, Username: "fred", Endpoint: "db",
				Status:         params.EntityStatus{Status: "joined"},
				IngressSubnets: []string{"192.168.1.0/32", "10.0.0.0/8"},
			}},
		},
	}
	s.assertFind(c, expected)
}

func (s *applicationOffersSuite) TestFindNoPermission(c *gc.C) {
	s.mockState.users["someone"] = &mockUser{"someone"}
	user := names.NewUserTag("someone")
	offer := names.NewApplicationOfferTag("hosted-db2")
	err := s.mockState.CreateOfferAccess(offer, user, permission.NoAccess)
	c.Assert(err, jc.ErrorIsNil)

	s.setupOffers(c, "", true)
	s.authorizer.Tag = names.NewUserTag("someone")
	s.assertFind(c, []params.ApplicationOfferAdminDetails{})
}

func (s *applicationOffersSuite) TestFindPermission(c *gc.C) {
	s.setupOffers(c, "", true)
	user := names.NewUserTag("someone")
	s.authorizer.Tag = user
	expected := []params.ApplicationOfferAdminDetails{
		{
			ApplicationOfferDetails: params.ApplicationOfferDetails{
				SourceModelTag:         testing.ModelTag.String(),
				ApplicationDescription: "description",
				OfferName:              "hosted-db2",
				OfferUUID:              "hosted-db2-uuid",
				OfferURL:               "fred/prod.hosted-db2",
				Endpoints:              []params.RemoteEndpoint{{Name: "db"}},
				Bindings:               map[string]string{"db2": "myspace"},
				Spaces: []params.RemoteSpace{
					{
						Name:       "myspace",
						ProviderId: "juju-space-myspace",
						Subnets:    []params.Subnet{{CIDR: "4.3.2.0/24", ProviderId: "juju-subnet-1", Zones: []string{"az1"}}},
					},
				},
				Users: []params.OfferUserDetails{
					{UserName: "someone", DisplayName: "someone", Access: "read"},
				}},
		},
	}
	s.mockState.users[user.Name()] = &mockUser{user.Name()}
	s.mockState.CreateOfferAccess(names.NewApplicationOfferTag("hosted-db2"), user, permission.ReadAccess)
	s.assertFind(c, expected)
}

func (s *applicationOffersSuite) TestFindFiltersRequireModel(c *gc.C) {
	s.setupOffers(c, "", true)
	filter := params.OfferFilters{
		Filters: []params.OfferFilter{
			{
				OfferName:       "hosted-db2",
				ApplicationName: "test",
			}, {
				OfferName:       "hosted-mysql",
				ApplicationName: "test",
			},
		},
	}
	_, err := s.api.FindApplicationOffers(filter)
	c.Assert(err, gc.ErrorMatches, "application offer filter must specify a model name")
}

func (s *applicationOffersSuite) TestFindRequiresFilter(c *gc.C) {
	s.setupOffers(c, "", true)
	_, err := s.api.FindApplicationOffers(params.OfferFilters{})
	c.Assert(err, gc.ErrorMatches, "at least one offer filter is required")
}

func (s *applicationOffersSuite) TestFindMulti(c *gc.C) {
	db2Offer := jujucrossmodel.ApplicationOffer{
		OfferName:              "hosted-db2",
		OfferUUID:              "hosted-db2-uuid",
		ApplicationName:        "db2",
		ApplicationDescription: "db2 description",
		Endpoints:              map[string]charm.Relation{"db": {Name: "db2"}},
	}
	mysqlOffer := jujucrossmodel.ApplicationOffer{
		OfferName:              "hosted-mysql",
		OfferUUID:              "hosted-mysql-uuid",
		ApplicationName:        "mysql",
		ApplicationDescription: "mysql description",
		Endpoints:              map[string]charm.Relation{"db": {Name: "mysql"}},
	}
	postgresqlOffer := jujucrossmodel.ApplicationOffer{
		OfferName:              "hosted-postgresql",
		OfferUUID:              "hosted-postgresql-uuid",
		ApplicationName:        "postgresql",
		ApplicationDescription: "postgresql description",
		Endpoints:              map[string]charm.Relation{"db": {Name: "postgresql"}},
	}
	// Include an offer with bad data to ensure it is ignored.
	offerAppNotFound := jujucrossmodel.ApplicationOffer{
		OfferName:       "badoffer",
		ApplicationName: "missing",
	}

	s.applicationOffers.listOffers = func(filters ...jujucrossmodel.ApplicationOfferFilter) ([]jujucrossmodel.ApplicationOffer, error) {
		var result []jujucrossmodel.ApplicationOffer
		for _, f := range filters {
			switch f.OfferName {
			case "hosted-db2":
				result = append(result, db2Offer)
			case "hosted-mysql":
				result = append(result, mysqlOffer)
			case "hosted-postgresql":
				result = append(result, postgresqlOffer)
			default:
				result = append(result, offerAppNotFound)
			}
		}
		return result, nil
	}
	ch := &mockCharm{meta: &charm.Meta{Description: "A pretty popular blog engine"}}
	s.mockState.applications = map[string]crossmodel.Application{
		"db2": &mockApplication{
			name:  "db2",
			charm: ch, curl: charm.MustParseURL("db2-2"), bindings: map[string]string{"db2": "myspace"}},
	}
	s.mockState.model = &mockModel{uuid: testing.ModelTag.Id(), name: "prod", owner: "fred", modelType: state.ModelTypeIAAS}
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

	user := names.NewUserTag("someone")
	s.authorizer.Tag = user
	s.mockState.users[user.Name()] = &mockUser{user.Name()}
	s.mockState.CreateOfferAccess(names.NewApplicationOfferTag("hosted-db2"), user, permission.ConsumeAccess)

	anotherState := &mockState{
		modelUUID:   "uuid2",
		users:       make(map[string]applicationoffers.User),
		accessPerms: make(map[offerAccess]permission.Access),
		spaces:      make(map[string]applicationoffers.Space),
	}
	s.mockStatePool.st["uuid2"] = anotherState
	anotherState.applications = map[string]crossmodel.Application{
		"mysql": &mockApplication{
			name:  "mysql",
			charm: ch, curl: charm.MustParseURL("mysql-2"), bindings: map[string]string{"mysql": "anotherspace"}},
		"postgresql": &mockApplication{
			charm: ch, curl: charm.MustParseURL("postgresql-2"), bindings: map[string]string{"postgresql": "anotherspace"}},
	}
	anotherState.spaces["anotherspace"] = &mockSpace{
		name:       "anotherspace",
		providerId: "juju-space-anotherspace",
		subnets: []applicationoffers.Subnet{
			&mockSubnet{cidr: "4.3.2.0/24", providerId: "juju-subnet-1", zones: []string{"az1"}},
		},
	}
	anotherState.model = &mockModel{uuid: "uuid2", name: "another", owner: "mary", modelType: state.ModelTypeIAAS}
	s.mockState.relations["hosted-mysql:server wordpress:db"] = &mockRelation{
		id: 1,
		endpoint: state.Endpoint{
			ApplicationName: "mysql",
			Relation: charm.Relation{
				Name:      "server",
				Interface: "mysql",
				Role:      "provider",
			},
		},
	}
	s.mockState.connections = []applicationoffers.OfferConnection{
		&mockOfferConnection{
			username:    "fred",
			modelUUID:   testing.ModelTag.Id(),
			relationKey: "hosted-db2:db wordpress:db",
			relationId:  1,
		},
	}
	anotherState.users[user.Name()] = &mockUser{user.Name()}
	anotherState.CreateOfferAccess(names.NewApplicationOfferTag("hosted-mysql"), user, permission.ReadAccess)
	anotherState.CreateOfferAccess(names.NewApplicationOfferTag("hosted-postgresql"), user, permission.AdminAccess)

	s.mockState.allmodels = []applicationoffers.Model{
		s.mockState.model,
		anotherState.model,
	}

	filter := params.OfferFilters{
		Filters: []params.OfferFilter{
			{
				OfferName: "hosted-db2",
				OwnerName: "fred",
				ModelName: "prod",
			},
			{
				OfferName: "hosted-mysql",
				OwnerName: "mary",
				ModelName: "another",
			},
			{
				OfferName: "hosted-postgresql",
				OwnerName: "mary",
				ModelName: "another",
			},
			{
				OfferName: "badoffer",
				OwnerName: "mary",
				ModelName: "another",
			},
		},
	}
	found, err := s.api.FindApplicationOffers(filter)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found, jc.DeepEquals, params.QueryApplicationOffersResults{
		[]params.ApplicationOfferAdminDetails{
			{
				ApplicationOfferDetails: params.ApplicationOfferDetails{
					SourceModelTag:         testing.ModelTag.String(),
					ApplicationDescription: "db2 description",
					OfferName:              "hosted-db2",
					OfferUUID:              "hosted-db2-uuid",
					OfferURL:               "fred/prod.hosted-db2",
					Endpoints:              []params.RemoteEndpoint{{Name: "db"}},
					Bindings:               map[string]string{"db2": "myspace"},
					Spaces: []params.RemoteSpace{
						{
							Name:       "myspace",
							ProviderId: "juju-space-myspace",
							Subnets:    []params.Subnet{{CIDR: "4.3.2.0/24", ProviderId: "juju-subnet-1", Zones: []string{"az1"}}},
						},
					},
					Users: []params.OfferUserDetails{
						{UserName: "someone", DisplayName: "someone", Access: "consume"},
					},
				},
			},
			{
				ApplicationOfferDetails: params.ApplicationOfferDetails{
					SourceModelTag:         "model-uuid2",
					ApplicationDescription: "mysql description",
					OfferName:              "hosted-mysql",
					OfferUUID:              "hosted-mysql-uuid",
					OfferURL:               "mary/another.hosted-mysql",
					Endpoints:              []params.RemoteEndpoint{{Name: "db"}},
					Users: []params.OfferUserDetails{
						{UserName: "someone", DisplayName: "someone", Access: "read"},
					},
				},
			},
			{
				ApplicationOfferDetails: params.ApplicationOfferDetails{
					SourceModelTag:         "model-uuid2",
					ApplicationDescription: "postgresql description",
					OfferName:              "hosted-postgresql",
					OfferUUID:              "hosted-postgresql-uuid",
					OfferURL:               "mary/another.hosted-postgresql",
					Endpoints:              []params.RemoteEndpoint{{Name: "db"}},
					Users: []params.OfferUserDetails{
						{UserName: "someone", DisplayName: "someone", Access: "admin"},
					},
				},
				CharmURL: "cs:postgresql-2",
			},
		},
	})
	s.applicationOffers.CheckCallNames(c, listOffersBackendCall, listOffersBackendCall)
}

func (s *applicationOffersSuite) TestFindError(c *gc.C) {
	filter := params.OfferFilters{
		Filters: []params.OfferFilter{
			{
				OfferName:       "hosted-db2",
				ApplicationName: "test",
			},
		},
	}
	msg := "fail"

	s.applicationOffers.listOffers = func(filters ...jujucrossmodel.ApplicationOfferFilter) ([]jujucrossmodel.ApplicationOffer, error) {
		return nil, errors.New(msg)
	}
	s.mockState.model = &mockModel{uuid: testing.ModelTag.Id(), name: "prod", owner: "fred", modelType: state.ModelTypeIAAS}

	_, err := s.api.FindApplicationOffers(filter)
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf(".*%v.*", msg))
	s.applicationOffers.CheckCallNames(c, listOffersBackendCall)
}

func (s *applicationOffersSuite) TestFindMissingModelInMultipleFilters(c *gc.C) {
	filter := params.OfferFilters{
		Filters: []params.OfferFilter{
			{
				OfferName:       "hosted-db2",
				ApplicationName: "test",
			},
			{
				OfferName:       "hosted-mysql",
				ApplicationName: "test",
			},
		},
	}

	s.applicationOffers.listOffers = func(filters ...jujucrossmodel.ApplicationOfferFilter) ([]jujucrossmodel.ApplicationOffer, error) {
		panic("should not be called")
	}

	_, err := s.api.FindApplicationOffers(filter)
	c.Assert(err, gc.ErrorMatches, "application offer filter must specify a model name")
	s.applicationOffers.CheckCallNames(c)
}

type consumeSuite struct {
	baseSuite
	api *applicationoffers.OffersAPIV2
}

var _ = gc.Suite(&consumeSuite{})

func (s *consumeSuite) SetUpTest(c *gc.C) {
	s.baseSuite.SetUpTest(c)
	s.bakery = &mockBakeryService{caveats: make(map[string][]checkers.Caveat)}
	getApplicationOffers := func(st interface{}) jujucrossmodel.ApplicationOffers {
		return &mockApplicationOffers{st: st.(*mockState)}
	}

	resources := common.NewResources()
	resources.RegisterNamed("dataDir", common.StringResource(c.MkDir()))

	getEnviron := func(modelUUID string) (environs.Environ, error) {
		return s.env, nil
	}
	var err error
	s.authContext, err = crossmodel.NewAuthContext(&mockCommonStatePool{s.mockStatePool}, s.bakery, s.bakery)
	c.Assert(err, jc.ErrorIsNil)
	apiV1, err := applicationoffers.CreateOffersAPI(
		getApplicationOffers, getEnviron, getFakeControllerInfo,
		s.mockState, s.mockStatePool, s.authorizer, resources, s.authContext,
		context.NewCloudCallContext(),
	)
	c.Assert(err, jc.ErrorIsNil)
	s.api = &applicationoffers.OffersAPIV2{OffersAPI: apiV1}
}

func (s *consumeSuite) TestConsumeDetailsRejectsEndpoints(c *gc.C) {
	results, err := s.api.GetConsumeDetails(params.OfferURLs{
		OfferURLs: []string{"fred/prod.application:db"},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error != nil, jc.IsTrue)
	c.Assert(results.Results[0].Error.Message, gc.Equals, `remote application "fred/prod.application:db" shouldn't include endpoint`)
}

func (s *consumeSuite) TestConsumeDetailsNoPermission(c *gc.C) {
	s.setupOffer()
	st := s.mockStatePool.st[testing.ModelTag.Id()]
	st.(*mockState).users["someone"] = &mockUser{"someone"}
	apiUser := names.NewUserTag("someone")
	offer := names.NewApplicationOfferTag("hosted-mysql")
	err := st.CreateOfferAccess(offer, apiUser, permission.NoAccess)
	c.Assert(err, jc.ErrorIsNil)

	s.authorizer.Tag = apiUser
	results, err := s.api.GetConsumeDetails(params.OfferURLs{
		OfferURLs: []string{"fred/prod.hosted-mysql"},
	})
	c.Assert(err, jc.ErrorIsNil)
	expected := []params.ConsumeOfferDetailsResult{{
		Error: common.ServerError(errors.NotFoundf("application offer %q", "fred/prod.hosted-mysql")),
	}}
	c.Assert(results.Results, jc.DeepEquals, expected)
}

func (s *consumeSuite) TestConsumeDetailsWithPermission(c *gc.C) {
	s.setupOffer()
	st := s.mockStatePool.st[testing.ModelTag.Id()]
	st.(*mockState).users["someone"] = &mockUser{"someone"}
	apiUser := names.NewUserTag("someone")
	offer := names.NewApplicationOfferTag("hosted-mysql")
	err := st.CreateOfferAccess(offer, apiUser, permission.ConsumeAccess)
	c.Assert(err, jc.ErrorIsNil)

	s.authorizer.Tag = apiUser
	results, err := s.api.GetConsumeDetails(params.OfferURLs{
		OfferURLs: []string{"fred/prod.hosted-mysql"},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)
	c.Assert(results.Results[0].Offer, jc.DeepEquals, &params.ApplicationOfferDetails{
		SourceModelTag:         "model-deadbeef-0bad-400d-8000-4b1d0d06f00d",
		OfferURL:               "fred/prod.hosted-mysql",
		OfferName:              "hosted-mysql",
		OfferUUID:              "hosted-mysql-uuid",
		ApplicationDescription: "a database",
		Endpoints:              []params.RemoteEndpoint{{Name: "server", Role: "provider", Interface: "mysql"}},
		Bindings:               map[string]string{"database": "myspace"},
		Spaces: []params.RemoteSpace{
			{
				Name:       "myspace",
				ProviderId: "juju-space-myspace",
				Subnets:    []params.Subnet{{CIDR: "4.3.2.0/24", ProviderId: "juju-subnet-1", Zones: []string{"az1"}}},
			},
		},
		Users: []params.OfferUserDetails{
			{UserName: "someone", DisplayName: "someone", Access: "consume"},
		},
	})
	c.Assert(results.Results[0].ControllerInfo, jc.DeepEquals, &params.ExternalControllerInfo{
		ControllerTag: testing.ControllerTag.String(),
		Addrs:         []string{"192.168.1.1:17070"},
		CACert:        testing.CACert,
	})
	c.Assert(results.Results[0].Macaroon.Id(), jc.DeepEquals, []byte("id"))
	cav := s.bakery.caveats[string(results.Results[0].Macaroon.Id())]
	c.Check(cav, gc.HasLen, 4)
	c.Check(strings.HasPrefix(cav[0].Condition, "time-before "), jc.IsTrue)
	c.Check(cav[1].Condition, gc.Equals, "declared source-model-uuid deadbeef-0bad-400d-8000-4b1d0d06f00d")
	c.Check(cav[2].Condition, gc.Equals, "declared offer-uuid hosted-mysql-uuid")
	c.Check(cav[3].Condition, gc.Equals, "declared username someone")
}

func (s *consumeSuite) TestConsumeDetailsDefaultEndpoint(c *gc.C) {
	s.setupOffer()

	st := s.mockStatePool.st[testing.ModelTag.Id()].(*mockState)
	st.users["someone"] = &mockUser{"someone"}
	delete(st.applications["mysql"].(*mockApplication).bindings, "database")
	st.spaces[""] = &mockSpace{
		name: "",
	}

	apiUser := names.NewUserTag("someone")
	offer := names.NewApplicationOfferTag("hosted-mysql")
	err := st.CreateOfferAccess(offer, apiUser, permission.ConsumeAccess)
	c.Assert(err, jc.ErrorIsNil)

	s.authorizer.Tag = apiUser
	results, err := s.api.GetConsumeDetails(params.OfferURLs{
		OfferURLs: []string{"fred/prod.hosted-mysql"},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)
	c.Assert(results.Results[0].Offer, jc.DeepEquals, &params.ApplicationOfferDetails{
		SourceModelTag:         "model-deadbeef-0bad-400d-8000-4b1d0d06f00d",
		OfferURL:               "fred/prod.hosted-mysql",
		OfferName:              "hosted-mysql",
		OfferUUID:              "hosted-mysql-uuid",
		ApplicationDescription: "a database",
		Endpoints:              []params.RemoteEndpoint{{Name: "server", Role: "provider", Interface: "mysql"}},
		Bindings:               map[string]string{"database": ""},
		Users: []params.OfferUserDetails{
			{UserName: "someone", DisplayName: "someone", Access: "consume"},
		},
	})
}

func (s *consumeSuite) setupOffer() {
	modelUUID := testing.ModelTag.Id()
	offerName := "hosted-mysql"

	model := &mockModel{uuid: modelUUID, name: "prod", owner: "fred", modelType: state.ModelTypeIAAS}
	s.mockState.allmodels = []applicationoffers.Model{model}
	st := &mockState{
		modelUUID:         modelUUID,
		model:             model,
		applications:      make(map[string]crossmodel.Application),
		applicationOffers: make(map[string]jujucrossmodel.ApplicationOffer),
		users:             make(map[string]applicationoffers.User),
		accessPerms:       make(map[offerAccess]permission.Access),
		spaces:            make(map[string]applicationoffers.Space),
		relations:         make(map[string]crossmodel.Relation),
	}
	s.mockStatePool.st[modelUUID] = st
	anOffer := jujucrossmodel.ApplicationOffer{
		ApplicationName:        "mysql",
		ApplicationDescription: "a database",
		OfferName:              offerName,
		OfferUUID:              offerName + "-uuid",
		Endpoints: map[string]charm.Relation{
			"server": {Name: "database", Interface: "mysql", Role: "provider", Scope: "global"}},
	}
	st.applicationOffers[offerName] = anOffer
	st.applications["mysql"] = &mockApplication{
		name:     "mysql",
		charm:    &mockCharm{meta: &charm.Meta{Description: "A pretty popular database"}},
		bindings: map[string]string{"database": "myspace"},
		endpoints: []state.Endpoint{
			{Relation: charm.Relation{Name: "juju-info", Role: "provider", Interface: "juju-info", Limit: 0, Scope: "global"}},
			{Relation: charm.Relation{Name: "server", Role: "provider", Interface: "mysql", Limit: 0, Scope: "global"}},
			{Relation: charm.Relation{Name: "server-admin", Role: "provider", Interface: "mysql-root", Limit: 0, Scope: "global"}}},
	}
	st.spaces["myspace"] = &mockSpace{
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

func (s *consumeSuite) TestRemoteApplicationInfo(c *gc.C) {
	s.setupOffer()
	st := s.mockStatePool.st[testing.ModelTag.Id()]
	st.(*mockState).users["foobar"] = &mockUser{"foobar"}

	// Give user permission to see the offer.
	user := names.NewUserTag("foobar")
	offer := names.NewApplicationOfferTag("hosted-mysql")
	err := st.CreateOfferAccess(offer, user, permission.ConsumeAccess)
	c.Assert(err, jc.ErrorIsNil)

	s.authorizer.Tag = user
	results, err := s.api.RemoteApplicationInfo(params.OfferURLs{
		OfferURLs: []string{"fred/prod.hosted-mysql", "fred/prod.unknown"},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 2)
	c.Assert(results.Results[0].Error, gc.IsNil)
	c.Assert(results.Results, jc.DeepEquals, []params.RemoteApplicationInfoResult{
		{Result: &params.RemoteApplicationInfo{
			ModelTag:         testing.ModelTag.String(),
			Name:             "hosted-mysql",
			Description:      "a database",
			OfferURL:         "fred/prod.hosted-mysql",
			SourceModelLabel: "prod",
			IconURLPath:      "rest/1.0/remote-application/hosted-mysql/icon",
			Endpoints: []params.RemoteEndpoint{
				{Name: "server", Role: "provider", Interface: "mysql"}},
		}},
		{
			Error: &params.Error{Message: `application offer "unknown" not found`, Code: "not found"},
		},
	})
	s.env.stub.CheckCallNames(c, "ProviderSpaceInfo")
	s.env.stub.CheckCall(c, 0, "ProviderSpaceInfo", &network.SpaceInfo{
		Name:       "myspace",
		ProviderId: "juju-space-myspace",
		Subnets: []network.SubnetInfo{{
			CIDR:              "4.3.2.0/24",
			ProviderId:        "juju-subnet-1",
			AvailabilityZones: []string{"az1"},
		}},
	})
}

func (s *consumeSuite) TestDestroyOffersNoForceV1(c *gc.C) {
	s.assertDestroyOffersNoForce(c, s.api.OffersAPI)
}

func (s *consumeSuite) TestDestroyOffersNoForceV2(c *gc.C) {
	s.assertDestroyOffersNoForce(c, s.api)
}

type destroyOffers interface {
	DestroyOffers(args params.DestroyApplicationOffers) (params.ErrorResults, error)
}

func (s *consumeSuite) assertDestroyOffersNoForce(c *gc.C, api destroyOffers) {
	s.setupOffer()
	st := s.mockStatePool.st[testing.ModelTag.Id()]
	st.(*mockState).users["foobar"] = &mockUser{"foobar"}
	st.(*mockState).connections = []applicationoffers.OfferConnection{
		&mockOfferConnection{
			username:    "fred",
			modelUUID:   testing.ModelTag.Id(),
			relationKey: "hosted-db2:db wordpress:db",
			relationId:  1,
		},
	}

	s.authorizer.Tag = names.NewUserTag("admin")
	results, err := s.api.DestroyOffers(params.DestroyApplicationOffers{
		OfferURLs: []string{
			"fred/prod.hosted-mysql"},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results, jc.DeepEquals, []params.ErrorResult{
		{
			Error: &params.Error{Message: `offer has 1 relations`},
		},
	})

	urls := []string{"fred/prod.hosted-db2"}
	filter := params.OfferURLs{urls}
	found, err := s.api.ApplicationOffers(filter)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.Results, gc.HasLen, 1)
	c.Assert(found.Results[0].Error.Error(), gc.Matches, `application offer "fred/prod.hosted-db2" not found`)
}

func (s *consumeSuite) TestDestroyOffersForce(c *gc.C) {
	s.setupOffer()
	st := s.mockStatePool.st[testing.ModelTag.Id()]
	st.(*mockState).users["foobar"] = &mockUser{"foobar"}
	st.(*mockState).connections = []applicationoffers.OfferConnection{
		&mockOfferConnection{
			username:    "fred",
			modelUUID:   testing.ModelTag.Id(),
			relationKey: "hosted-db2:db wordpress:db",
			relationId:  1,
		},
	}

	s.authorizer.Tag = names.NewUserTag("admin")
	results, err := s.api.DestroyOffers(params.DestroyApplicationOffers{
		Force: true,
		OfferURLs: []string{
			"fred/prod.hosted-mysql", "fred/prod.unknown", "garbage/badmodel.someoffer", "badmodel.someoffer"},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 4)
	c.Assert(results.Results[0].Error, gc.IsNil)
	c.Assert(results.Results, jc.DeepEquals, []params.ErrorResult{
		{},
		{
			Error: &params.Error{Message: `application offer "unknown" not found`, Code: "not found"},
		}, {
			Error: &params.Error{Message: `model "garbage/badmodel" not found`, Code: "not found"},
		}, {
			Error: &params.Error{Message: `model "admin/badmodel" not found`, Code: "not found"},
		},
	})

	urls := []string{"fred/prod.hosted-db2"}
	filter := params.OfferURLs{urls}
	found, err := s.api.ApplicationOffers(filter)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.Results, gc.HasLen, 1)
	c.Assert(found.Results[0].Error.Error(), gc.Matches, `application offer "fred/prod.hosted-db2" not found`)
}

func (s *consumeSuite) TestDestroyOffersPermission(c *gc.C) {
	s.setupOffer()
	s.authorizer.Tag = names.NewUserTag("mary")
	st := s.mockStatePool.st[testing.ModelTag.Id()]
	st.(*mockState).users["foobar"] = &mockUser{"foobar"}

	results, err := s.api.DestroyOffers(params.DestroyApplicationOffers{
		OfferURLs: []string{"fred/prod.hosted-mysql"},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.ErrorMatches, common.ErrPerm.Error())
}
