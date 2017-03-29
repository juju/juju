// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoteendpoints_test

import (
	"fmt"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/crossmodelcommon"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/apiserver/remoteendpoints"
	jujucrossmodel "github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/permission"
)

type remoteEndpointsSuite struct {
	baseSuite
	api *remoteendpoints.EndpointsAPI
}

var _ = gc.Suite(&remoteEndpointsSuite{})

func (s *remoteEndpointsSuite) SetUpTest(c *gc.C) {
	s.baseSuite.SetUpTest(c)
	s.applicationOffers = &mockApplicationOffers{}
	getApplicationOffers := func(interface{}) jujucrossmodel.ApplicationOffers {
		return s.applicationOffers
	}

	var err error
	s.api, err = remoteendpoints.CreateEndpointsAPI(
		getApplicationOffers, s.mockState, s.mockStatePool, s.authorizer,
	)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *remoteEndpointsSuite) assertShow(c *gc.C, expected []params.ApplicationOfferResult) {
	applicationName := "test"
	filter := params.ApplicationURLs{[]string{"fred/prod.hosted-db2"}}
	anOffer := jujucrossmodel.ApplicationOffer{
		ApplicationName:        applicationName,
		ApplicationDescription: "description",
		OfferName:              "hosted-db2",
		Endpoints:              map[string]charm.Relation{"db": {Name: "db"}},
	}

	s.applicationOffers.listOffers = func(filters ...jujucrossmodel.ApplicationOfferFilter) ([]jujucrossmodel.ApplicationOffer, error) {
		return []jujucrossmodel.ApplicationOffer{anOffer}, nil
	}
	ch := &mockCharm{meta: &charm.Meta{Description: "A pretty popular blog engine"}}
	s.mockState.applications = map[string]crossmodelcommon.Application{
		applicationName: &mockApplication{charm: ch, curl: charm.MustParseURL("db2-2")},
	}
	s.mockState.model = &mockModel{uuid: "uuid", name: "prod", owner: "fred"}
	s.mockState.connStatus = &mockConnectionStatus{count: 5}

	found, err := s.api.ApplicationOffers(filter)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.Results, jc.DeepEquals, expected)
	s.applicationOffers.CheckCallNames(c, listOffersBackendCall)
}

func (s *remoteEndpointsSuite) TestShow(c *gc.C) {
	expected := []params.ApplicationOfferResult{{
		Result: params.ApplicationOffer{
			ApplicationDescription: "description",
			OfferURL:               "fred/prod.hosted-db2",
			OfferName:              "hosted-db2",
			Endpoints:              []params.RemoteEndpoint{{Name: "db"}},
			Access:                 "admin"},
	}}
	s.authorizer.Tag = names.NewUserTag("admin")
	s.assertShow(c, expected)
}

func (s *remoteEndpointsSuite) TestShowNoPermission(c *gc.C) {
	s.authorizer.Tag = names.NewUserTag("someone")
	expected := []params.ApplicationOfferResult{{
		Error: common.ServerError(errors.NotFoundf("application offer %q", "hosted-db2")),
	}}
	s.assertShow(c, expected)
}

func (s *remoteEndpointsSuite) TestShowPermission(c *gc.C) {
	s.authorizer.Tag = names.NewUserTag("someone")
	expected := []params.ApplicationOfferResult{{
		Result: params.ApplicationOffer{
			ApplicationDescription: "description",
			OfferURL:               "fred/prod.hosted-db2",
			OfferName:              "hosted-db2",
			Endpoints:              []params.RemoteEndpoint{{Name: "db"}},
			Access:                 "read"},
	}}
	s.mockState.offerAccess[names.NewApplicationOfferTag("hosted-db2")] = permission.ReadAccess
	s.assertShow(c, expected)
}

func (s *remoteEndpointsSuite) TestShowError(c *gc.C) {
	url := "fred/prod.hosted-db2"
	filter := params.ApplicationURLs{[]string{url}}
	msg := "fail"

	s.applicationOffers.listOffers = func(filters ...jujucrossmodel.ApplicationOfferFilter) ([]jujucrossmodel.ApplicationOffer, error) {
		return nil, errors.New(msg)
	}
	s.mockState.model = &mockModel{uuid: "uuid", name: "prod", owner: "fred"}

	result, err := s.api.ApplicationOffers(filter)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.ErrorMatches, fmt.Sprintf(".*%v.*", msg))
	s.applicationOffers.CheckCallNames(c, listOffersBackendCall)
}

func (s *remoteEndpointsSuite) TestShowNotFound(c *gc.C) {
	urls := []string{"fred/prod.hosted-db2"}
	filter := params.ApplicationURLs{urls}

	s.applicationOffers.listOffers = func(filters ...jujucrossmodel.ApplicationOfferFilter) ([]jujucrossmodel.ApplicationOffer, error) {
		return nil, nil
	}
	s.mockState.model = &mockModel{uuid: "uuid", name: "prod", owner: "fred"}

	found, err := s.api.ApplicationOffers(filter)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.Results, gc.HasLen, 1)
	c.Assert(found.Results[0].Error.Error(), gc.Matches, `application offer "hosted-db2" not found`)
	s.applicationOffers.CheckCallNames(c, listOffersBackendCall)
}

func (s *remoteEndpointsSuite) TestShowErrorMsgMultipleURLs(c *gc.C) {
	urls := []string{"fred/prod.hosted-mysql", "fred/test.hosted-db2"}
	filter := params.ApplicationURLs{urls}

	s.applicationOffers.listOffers = func(filters ...jujucrossmodel.ApplicationOfferFilter) ([]jujucrossmodel.ApplicationOffer, error) {
		return nil, nil
	}
	s.mockState.model = &mockModel{uuid: "uuid", name: "prod", owner: "fred"}
	s.mockState.allmodels = []crossmodelcommon.Model{
		s.mockState.model,
		&mockModel{uuid: "uuid2", name: "test", owner: "fred"},
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

func (s *remoteEndpointsSuite) TestShowFoundMultiple(c *gc.C) {
	name := "test"
	url := "fred/prod.hosted-" + name
	anOffer := jujucrossmodel.ApplicationOffer{
		ApplicationName:        name,
		ApplicationDescription: "description",
		OfferName:              "hosted-" + name,
		Endpoints:              map[string]charm.Relation{"db": {Name: "db"}},
	}

	name2 := "testagain"
	url2 := "mary/test.hosted-" + name2
	anOffer2 := jujucrossmodel.ApplicationOffer{
		ApplicationName:        name2,
		ApplicationDescription: "description2",
		OfferName:              "hosted-" + name2,
		Endpoints:              map[string]charm.Relation{"db2": {Name: "db2"}},
	}

	filter := params.ApplicationURLs{[]string{url, url2}}

	s.applicationOffers.listOffers = func(filters ...jujucrossmodel.ApplicationOfferFilter) ([]jujucrossmodel.ApplicationOffer, error) {
		c.Assert(filters, gc.HasLen, 1)
		if filters[0].OfferName == "hosted-test" {
			return []jujucrossmodel.ApplicationOffer{anOffer}, nil
		}
		return []jujucrossmodel.ApplicationOffer{anOffer2}, nil
	}
	ch := &mockCharm{meta: &charm.Meta{Description: "A pretty popular blog engine"}}
	s.mockState.applications = map[string]crossmodelcommon.Application{
		"test": &mockApplication{charm: ch, curl: charm.MustParseURL("db2-2")},
	}
	s.mockState.model = &mockModel{uuid: "uuid", name: "prod", owner: "fred"}
	s.mockState.allmodels = []crossmodelcommon.Model{
		s.mockState.model,
		&mockModel{uuid: "uuid2", name: "test", owner: "mary"},
	}
	s.mockState.connStatus = &mockConnectionStatus{count: 5}
	s.mockState.offerAccess[names.NewApplicationOfferTag("hosted-test")] = permission.ReadAccess

	anotherState := &mockState{
		modelUUID:   "uuid2",
		offerAccess: make(map[names.ApplicationOfferTag]permission.Access),
	}
	anotherState.applications = map[string]crossmodelcommon.Application{
		"testagain": &mockApplication{charm: ch, curl: charm.MustParseURL("mysql-2")},
	}
	anotherState.connStatus = &mockConnectionStatus{count: 5}
	anotherState.offerAccess[names.NewApplicationOfferTag("hosted-testagain")] = permission.ConsumeAccess
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
			Access:                 "read",
			Endpoints:              []params.RemoteEndpoint{{Name: "db"}}},
		{
			ApplicationDescription: "description2",
			OfferName:              "hosted-" + name2,
			OfferURL:               url2,
			Access:                 "consume",
			Endpoints:              []params.RemoteEndpoint{{Name: "db2"}}},
	})
	s.applicationOffers.CheckCallNames(c, listOffersBackendCall, listOffersBackendCall)
}

func (s *remoteEndpointsSuite) assertFind(c *gc.C, expected []params.ApplicationOffer) {
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
		Results: expected,
	})
	s.applicationOffers.CheckCallNames(c, listOffersBackendCall)
}

func (s *remoteEndpointsSuite) TestFind(c *gc.C) {
	s.setupOffers(c, "")
	s.authorizer.Tag = names.NewUserTag("admin")
	expected := []params.ApplicationOffer{
		{
			ApplicationDescription: "description",
			OfferName:              "hosted-db2",
			OfferURL:               "fred/prod.hosted-db2",
			Endpoints:              []params.RemoteEndpoint{{Name: "db"}},
			Access:                 "admin"}}
	s.assertFind(c, expected)
}

func (s *remoteEndpointsSuite) TestFindNoPermission(c *gc.C) {
	s.setupOffers(c, "")
	s.authorizer.Tag = names.NewUserTag("someone")
	s.assertFind(c, []params.ApplicationOffer{})
}

func (s *remoteEndpointsSuite) TestFindPermission(c *gc.C) {
	s.setupOffers(c, "")
	s.authorizer.Tag = names.NewUserTag("someone")
	expected := []params.ApplicationOffer{
		{
			ApplicationDescription: "description",
			OfferName:              "hosted-db2",
			OfferURL:               "fred/prod.hosted-db2",
			Endpoints:              []params.RemoteEndpoint{{Name: "db"}},
			Access:                 "read"}}
	s.mockState.offerAccess[names.NewApplicationOfferTag("hosted-db2")] = permission.ReadAccess
	s.assertFind(c, expected)
}

func (s *remoteEndpointsSuite) TestFindMulti(c *gc.C) {
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
	s.mockState.applications = map[string]crossmodelcommon.Application{
		"db2": &mockApplication{charm: ch, curl: charm.MustParseURL("db2-2")},
	}
	s.mockState.model = &mockModel{uuid: "uuid", name: "prod", owner: "fred"}
	s.mockState.connStatus = &mockConnectionStatus{count: 5}
	s.mockState.offerAccess[names.NewApplicationOfferTag("hosted-db2")] = permission.ConsumeAccess

	anotherState := &mockState{
		modelUUID:   "uuid2",
		offerAccess: make(map[names.ApplicationOfferTag]permission.Access),
	}
	s.mockStatePool.st["uuid2"] = anotherState
	anotherState.applications = map[string]crossmodelcommon.Application{
		"mysql":      &mockApplication{charm: ch, curl: charm.MustParseURL("mysql-2")},
		"postgresql": &mockApplication{charm: ch, curl: charm.MustParseURL("postgresql-2")},
	}
	anotherState.model = &mockModel{uuid: "uuid2", name: "another", owner: "mary"}
	anotherState.connStatus = &mockConnectionStatus{count: 15}
	anotherState.offerAccess[names.NewApplicationOfferTag("hosted-mysql")] = permission.ReadAccess
	anotherState.offerAccess[names.NewApplicationOfferTag("hosted-postgresql")] = permission.AdminAccess

	s.mockState.allmodels = []crossmodelcommon.Model{
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
				Access:                 "consume",
				Endpoints:              []params.RemoteEndpoint{{Name: "db"}},
			},
			{
				ApplicationDescription: "mysql description",
				OfferName:              "hosted-mysql",
				OfferURL:               "mary/another.hosted-mysql",
				Access:                 "read",
				Endpoints:              []params.RemoteEndpoint{{Name: "db"}},
			},
			{
				ApplicationDescription: "postgresql description",
				OfferName:              "hosted-postgresql",
				OfferURL:               "mary/another.hosted-postgresql",
				Access:                 "admin",
				Endpoints:              []params.RemoteEndpoint{{Name: "db"}},
			},
		},
	})
	s.applicationOffers.CheckCallNames(c, listOffersBackendCall, listOffersBackendCall)
}

func (s *remoteEndpointsSuite) TestFindError(c *gc.C) {
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
	s.mockState.model = &mockModel{uuid: "uuid", name: "prod", owner: "fred"}

	_, err := s.api.FindApplicationOffers(filter)
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf(".*%v.*", msg))
	s.applicationOffers.CheckCallNames(c, listOffersBackendCall)
}

func (s *remoteEndpointsSuite) TestFindMissingModelInMultipleFilters(c *gc.C) {
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
