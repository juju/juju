// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel_test

import (
	"fmt"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/juju/apiserver/crossmodel"
	"github.com/juju/juju/apiserver/params"
	jujucrossmodel "github.com/juju/juju/core/crossmodel"
)

type crossmodelSuite struct {
	baseCrossmodelSuite
}

var _ = gc.Suite(&crossmodelSuite{})

func (s *crossmodelSuite) SetUpTest(c *gc.C) {
	s.baseCrossmodelSuite.SetUpTest(c)
}

func (s *crossmodelSuite) TestOffer(c *gc.C) {
	applicationName := "test"
	s.addApplication(c, applicationName)
	one := params.AddApplicationOffer{
		OfferName:       "offer-test",
		ApplicationName: applicationName,
		Endpoints:       map[string]string{"db": "db"},
	}
	all := params.AddApplicationOffers{Offers: []params.AddApplicationOffer{one}}
	s.applicationOffers.addOffer = func(offer jujucrossmodel.AddApplicationOfferArgs) (*jujucrossmodel.ApplicationOffer, error) {
		c.Assert(offer.OfferName, gc.Equals, one.OfferName)
		c.Assert(offer.ApplicationName, gc.Equals, one.ApplicationName)
		c.Assert(offer.ApplicationDescription, gc.Equals, "A pretty popular blog engine")
		return &jujucrossmodel.ApplicationOffer{}, nil
	}
	charm := &mockCharm{meta: &charm.Meta{Description: "A pretty popular blog engine"}}
	s.mockState.applications = map[string]crossmodel.Application{
		applicationName: &mockApplication{charm: charm},
	}

	errs, err := s.api.Offer(all)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errs.Results, gc.HasLen, len(all.Offers))
	c.Assert(errs.Results[0].Error, gc.IsNil)
	s.applicationOffers.CheckCallNames(c, addOffersBackendCall)
}

func (s *crossmodelSuite) TestOfferSomeFail(c *gc.C) {
	s.addApplication(c, "one")
	s.addApplication(c, "two")
	s.addApplication(c, "paramsfail")
	one := params.AddApplicationOffer{
		OfferName:       "offer-one",
		ApplicationName: "one",
		Endpoints:       map[string]string{"db": "db"},
	}
	bad := params.AddApplicationOffer{
		OfferName:       "offer-bad",
		ApplicationName: "notthere",
		Endpoints:       map[string]string{"db": "db"},
	}
	bad2 := params.AddApplicationOffer{
		OfferName:       "offer-bad",
		ApplicationName: "paramsfail",
		Endpoints:       map[string]string{"db": "db"},
	}
	two := params.AddApplicationOffer{
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
	charm := &mockCharm{meta: &charm.Meta{Description: "A pretty popular blog engine"}}
	s.mockState.applications = map[string]crossmodel.Application{
		"one":        &mockApplication{charm: charm},
		"two":        &mockApplication{charm: charm},
		"paramsfail": &mockApplication{charm: charm},
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

func (s *crossmodelSuite) TestOfferError(c *gc.C) {
	applicationName := "test"
	s.addApplication(c, applicationName)
	one := params.AddApplicationOffer{
		OfferName:       "offer-test",
		ApplicationName: applicationName,
		Endpoints:       map[string]string{"db": "db"},
	}
	all := params.AddApplicationOffers{Offers: []params.AddApplicationOffer{one}}

	msg := "fail"

	s.applicationOffers.addOffer = func(offer jujucrossmodel.AddApplicationOfferArgs) (*jujucrossmodel.ApplicationOffer, error) {
		return nil, errors.New(msg)
	}
	charm := &mockCharm{meta: &charm.Meta{Description: "A pretty popular blog engine"}}
	s.mockState.applications = map[string]crossmodel.Application{
		applicationName: &mockApplication{charm: charm},
	}

	errs, err := s.api.Offer(all)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errs.Results, gc.HasLen, len(all.Offers))
	c.Assert(errs.Results[0].Error, gc.ErrorMatches, fmt.Sprintf(".*%v.*", msg))
	s.applicationOffers.CheckCallNames(c, addOffersBackendCall)
}

func (s *crossmodelSuite) TestShow(c *gc.C) {
	applicationName := "test"
	offerName := "hosted-test"
	url := "fred/prod.hosted-db2"

	filter := params.ApplicationURLs{[]string{url}}
	anOffer := jujucrossmodel.ApplicationOffer{
		ApplicationName:        applicationName,
		ApplicationDescription: "description",
		OfferName:              offerName,
		Endpoints:              map[string]charm.Relation{"db": {Name: "db"}},
	}

	s.applicationOffers.listOffers = func(filters ...jujucrossmodel.ApplicationOfferFilter) ([]jujucrossmodel.ApplicationOffer, error) {
		return []jujucrossmodel.ApplicationOffer{anOffer}, nil
	}
	ch := &mockCharm{meta: &charm.Meta{Description: "A pretty popular blog engine"}}
	s.mockState.applications = map[string]crossmodel.Application{
		applicationName: &mockApplication{charm: ch, curl: charm.MustParseURL("db2-2")},
	}
	s.mockState.model = &mockModel{uuid: "uuid", name: "prod", owner: "fred"}
	s.mockState.usermodels = []crossmodel.UserModel{
		&mockUserModel{model: s.mockState.model},
	}
	s.mockState.connStatus = &mockConnectionStatus{count: 5}

	found, err := s.api.ApplicationOffers(filter)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.Results, gc.HasLen, 1)
	result := found.Results[0]
	c.Assert(result.Error, gc.IsNil)
	c.Assert(result.Result, jc.DeepEquals, params.ApplicationOffer{
		ApplicationDescription: "description",
		OfferURL:               url,
		OfferName:              offerName,
		Endpoints:              []params.RemoteEndpoint{{Name: "db"}}},
	)
	s.applicationOffers.CheckCallNames(c, listOffersBackendCall)
}

func (s *crossmodelSuite) TestShowError(c *gc.C) {
	url := "fred/prod.hosted-db2"
	filter := params.ApplicationURLs{[]string{url}}
	msg := "fail"

	s.applicationOffers.listOffers = func(filters ...jujucrossmodel.ApplicationOfferFilter) ([]jujucrossmodel.ApplicationOffer, error) {
		return nil, errors.New(msg)
	}
	s.mockState.model = &mockModel{uuid: "uuid", name: "prod", owner: "fred"}
	s.mockState.usermodels = []crossmodel.UserModel{
		&mockUserModel{model: s.mockState.model},
	}

	result, err := s.api.ApplicationOffers(filter)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.ErrorMatches, fmt.Sprintf(".*%v.*", msg))
	s.applicationOffers.CheckCallNames(c, listOffersBackendCall)
}

func (s *crossmodelSuite) TestShowNotFound(c *gc.C) {
	urls := []string{"fred/prod.hosted-db2"}
	filter := params.ApplicationURLs{urls}

	s.applicationOffers.listOffers = func(filters ...jujucrossmodel.ApplicationOfferFilter) ([]jujucrossmodel.ApplicationOffer, error) {
		return nil, nil
	}
	s.mockState.model = &mockModel{uuid: "uuid", name: "prod", owner: "fred"}
	s.mockState.usermodels = []crossmodel.UserModel{
		&mockUserModel{model: s.mockState.model},
	}

	found, err := s.api.ApplicationOffers(filter)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.Results, gc.HasLen, 1)
	c.Assert(found.Results[0].Error.Error(), gc.Matches, `application offer "hosted-db2" not found`)
	s.applicationOffers.CheckCallNames(c, listOffersBackendCall)
}

func (s *crossmodelSuite) TestShowErrorMsgMultipleURLs(c *gc.C) {
	urls := []string{"fred/prod.hosted-mysql", "fred/test.hosted-db2"}
	filter := params.ApplicationURLs{urls}

	s.applicationOffers.listOffers = func(filters ...jujucrossmodel.ApplicationOfferFilter) ([]jujucrossmodel.ApplicationOffer, error) {
		return nil, nil
	}
	s.mockState.model = &mockModel{uuid: "uuid", name: "prod", owner: "fred"}
	s.mockState.usermodels = []crossmodel.UserModel{
		&mockUserModel{model: s.mockState.model},
		&mockUserModel{model: &mockModel{uuid: "uuid2", name: "test", owner: "fred"}},
	}
	anotherState := &mockState{modelUUID: "uuid2"}
	s.mockStatePool.st["uuid2"] = anotherState

	found, err := s.api.ApplicationOffers(filter)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.Results, gc.HasLen, 2)
	c.Assert(found.Results[0].Error.Error(), gc.Matches, `application offer "hosted-mysql" not found`)
	c.Assert(found.Results[1].Error.Error(), gc.Matches, `application offer "hosted-db2" not found`)
	s.applicationOffers.CheckCallNames(c, listOffersBackendCall, listOffersBackendCall)
}

func (s *crossmodelSuite) TestShowFoundMultiple(c *gc.C) {
	name := "test"
	url := "fred/prod.hosted-sql"
	anOffer := jujucrossmodel.ApplicationOffer{
		ApplicationName:        name,
		ApplicationDescription: "description",
		OfferName:              "hosted-" + name,
		Endpoints:              map[string]charm.Relation{"db": {Name: "db"}},
	}

	name2 := "testAgain"
	url2 := "mary/test.hosted-db2"
	anOffer2 := jujucrossmodel.ApplicationOffer{
		ApplicationName:        name2,
		ApplicationDescription: "description2",
		OfferName:              "hosted-" + name2,
		Endpoints:              map[string]charm.Relation{"db2": {Name: "db2"}},
	}

	filter := params.ApplicationURLs{[]string{url, url2}}

	s.applicationOffers.listOffers = func(filters ...jujucrossmodel.ApplicationOfferFilter) ([]jujucrossmodel.ApplicationOffer, error) {
		c.Assert(filters, gc.HasLen, 1)
		if filters[0].OfferName == "hosted-sql" {
			return []jujucrossmodel.ApplicationOffer{anOffer}, nil
		}
		return []jujucrossmodel.ApplicationOffer{anOffer2}, nil
	}
	ch := &mockCharm{meta: &charm.Meta{Description: "A pretty popular blog engine"}}
	s.mockState.applications = map[string]crossmodel.Application{
		"test": &mockApplication{charm: ch, curl: charm.MustParseURL("db2-2")},
	}
	s.mockState.model = &mockModel{uuid: "uuid", name: "prod", owner: "fred"}
	s.mockState.usermodels = []crossmodel.UserModel{
		&mockUserModel{model: s.mockState.model},
		&mockUserModel{model: &mockModel{uuid: "uuid2", name: "test", owner: "mary"}},
	}
	s.mockState.connStatus = &mockConnectionStatus{count: 5}

	anotherState := &mockState{modelUUID: "uuid2"}
	anotherState.applications = map[string]crossmodel.Application{
		"testAgain": &mockApplication{charm: ch, curl: charm.MustParseURL("mysql-2")},
	}
	anotherState.connStatus = &mockConnectionStatus{count: 5}
	s.mockStatePool.st["uuid2"] = anotherState

	found, err := s.api.ApplicationOffers(filter)
	c.Assert(err, jc.ErrorIsNil)
	var results []params.ApplicationOffer
	for _, r := range found.Results {
		c.Assert(r.Error, gc.IsNil)
		results = append(results, r.Result)
	}
	c.Assert(results, jc.DeepEquals, []params.ApplicationOffer{
		{
			ApplicationDescription: "description",
			OfferName:              "hosted-" + name,
			OfferURL:               url,
			Endpoints:              []params.RemoteEndpoint{{Name: "db"}}},
		{
			ApplicationDescription: "description2",
			OfferName:              "hosted-" + name2,
			OfferURL:               url2,
			Endpoints:              []params.RemoteEndpoint{{Name: "db2"}}},
	})
	s.applicationOffers.CheckCallNames(c, listOffersBackendCall, listOffersBackendCall)
}

func (s *crossmodelSuite) setupOffers(c *gc.C, filterAppName string) {
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
	ch := &mockCharm{meta: &charm.Meta{Description: "A pretty popular blog engine"}}
	s.mockState.applications = map[string]crossmodel.Application{
		"test": &mockApplication{charm: ch, curl: charm.MustParseURL("db2-2")},
	}
	s.mockState.model = &mockModel{uuid: "uuid", name: "prod", owner: "fred"}
	s.mockState.usermodels = []crossmodel.UserModel{
		&mockUserModel{model: s.mockState.model},
	}
	s.mockState.connStatus = &mockConnectionStatus{count: 5}
}

func (s *crossmodelSuite) TestFind(c *gc.C) {
	s.setupOffers(c, "")
	filter := params.OfferFilters{
		Filters: []params.OfferFilter{
			{
				OfferName: "hosted-db2",
			},
		},
	}
	found, err := s.api.FindApplicationOffers(filter)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found, jc.DeepEquals, params.FindApplicationOffersResults{
		[]params.ApplicationOffer{
			{
				ApplicationDescription: "description",
				OfferName:              "hosted-db2",
				OfferURL:               "fred/prod.hosted-db2",
				Endpoints:              []params.RemoteEndpoint{{Name: "db"}}}},
	})
	s.applicationOffers.CheckCallNames(c, listOffersBackendCall)
}

func (s *crossmodelSuite) TestFindMultiModel(c *gc.C) {
	db2Offer := jujucrossmodel.ApplicationOffer{
		OfferName:              "hosted-db2",
		ApplicationName:        "db2",
		ApplicationDescription: "db2 description",
		Endpoints:              map[string]charm.Relation{"db": {Name: "db2"}},
	}
	mysqlOffer := jujucrossmodel.ApplicationOffer{
		OfferName:              "hosted-mysql",
		ApplicationName:        "mysql",
		ApplicationDescription: "mysql description",
		Endpoints:              map[string]charm.Relation{"db": {Name: "mysql"}},
	}
	postgresqlOffer := jujucrossmodel.ApplicationOffer{
		OfferName:              "hosted-postgresql",
		ApplicationName:        "postgresql",
		ApplicationDescription: "postgresql description",
		Endpoints:              map[string]charm.Relation{"db": {Name: "postgresql"}},
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
			}
		}
		return result, nil
	}
	ch := &mockCharm{meta: &charm.Meta{Description: "A pretty popular blog engine"}}
	s.mockState.applications = map[string]crossmodel.Application{
		"db2": &mockApplication{charm: ch, curl: charm.MustParseURL("db2-2")},
	}
	s.mockState.model = &mockModel{uuid: "uuid", name: "prod", owner: "fred"}
	s.mockState.connStatus = &mockConnectionStatus{count: 5}

	anotherState := &mockState{modelUUID: "uuid2"}
	s.mockStatePool.st["uuid2"] = anotherState
	anotherState.applications = map[string]crossmodel.Application{
		"mysql":      &mockApplication{charm: ch, curl: charm.MustParseURL("mysql-2")},
		"postgresql": &mockApplication{charm: ch, curl: charm.MustParseURL("postgresql-2")},
	}
	anotherState.model = &mockModel{uuid: "uuid2", name: "another", owner: "fred"}
	anotherState.usermodels = []crossmodel.UserModel{
		&mockUserModel{model: anotherState.model},
	}
	anotherState.connStatus = &mockConnectionStatus{count: 15}

	s.mockState.usermodels = []crossmodel.UserModel{
		&mockUserModel{model: s.mockState.model},
		&mockUserModel{model: anotherState.model},
	}

	filter := params.OfferFilters{
		Filters: []params.OfferFilter{
			{
				OfferName: "hosted-db2",
			},
			{
				OfferName: "hosted-mysql",
				ModelName: "another",
			},
			{
				OfferName: "hosted-postgresql",
				ModelName: "another",
			},
		},
	}
	found, err := s.api.FindApplicationOffers(filter)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found, jc.DeepEquals, params.FindApplicationOffersResults{
		[]params.ApplicationOffer{
			{
				ApplicationDescription: "db2 description",
				OfferName:              "hosted-db2",
				OfferURL:               "fred/prod.hosted-db2",
				Endpoints:              []params.RemoteEndpoint{{Name: "db"}},
			},
			{
				ApplicationDescription: "mysql description",
				OfferName:              "hosted-mysql",
				OfferURL:               "fred/another.hosted-mysql",
				Endpoints:              []params.RemoteEndpoint{{Name: "db"}},
			},
			{
				ApplicationDescription: "postgresql description",
				OfferName:              "hosted-postgresql",
				OfferURL:               "fred/another.hosted-postgresql",
				Endpoints:              []params.RemoteEndpoint{{Name: "db"}},
			},
		},
	})
	s.applicationOffers.CheckCallNames(c, listOffersBackendCall, listOffersBackendCall)
}

func (s *crossmodelSuite) TestFindError(c *gc.C) {
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

	_, err := s.api.FindApplicationOffers(filter)
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf(".*%v.*", msg))
	s.applicationOffers.CheckCallNames(c, listOffersBackendCall)
}

func (s *crossmodelSuite) TestList(c *gc.C) {
	s.setupOffers(c, "test")
	filter := params.OfferFilters{
		Filters: []params.OfferFilter{
			{
				OfferName:       "hosted-db2",
				ApplicationName: "test",
			},
		},
	}
	found, err := s.api.ListApplicationOffers(filter)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found, jc.DeepEquals, params.ListApplicationOffersResults{
		[]params.ApplicationOfferDetails{
			{
				ApplicationOffer: params.ApplicationOffer{
					ApplicationDescription: "description",
					OfferName:              "hosted-db2",
					OfferURL:               "fred/prod.hosted-db2",
					Endpoints:              []params.RemoteEndpoint{{Name: "db"}},
				},
				CharmName:      "db2",
				ConnectedCount: 5,
			},
		},
	})
	s.applicationOffers.CheckCallNames(c, listOffersBackendCall)
}

func (s *crossmodelSuite) TestListError(c *gc.C) {
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

	_, err := s.api.ListApplicationOffers(filter)
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf(".*%v.*", msg))
	s.applicationOffers.CheckCallNames(c, listOffersBackendCall)
}
