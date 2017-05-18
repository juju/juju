// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package applicationoffers_test

import (
	"fmt"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/utils/set"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/applicationoffers"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	jujucrossmodel "github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/network"
	"github.com/juju/juju/permission"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
)

type applicationOffersSuite struct {
	baseSuite
	env environs.Environ
	api *applicationoffers.OffersAPI
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

	s.env = &mockEnviron{}
	getEnviron := func(modelUUID string) (environs.Environ, error) {
		return s.env, nil
	}
	var err error
	s.api, err = applicationoffers.CreateOffersAPI(
		getApplicationOffers, getEnviron, s.mockState, s.mockStatePool, s.authorizer, resources,
	)
	c.Assert(err, jc.ErrorIsNil)
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
	s.mockState.applications = map[string]applicationoffers.Application{
		applicationName: &mockApplication{charm: ch},
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
	s.mockState.applications = map[string]applicationoffers.Application{
		"one":        &mockApplication{charm: ch},
		"two":        &mockApplication{charm: ch},
		"paramsfail": &mockApplication{charm: ch},
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
	s.mockState.applications = map[string]applicationoffers.Application{
		applicationName: &mockApplication{charm: ch},
	}

	errs, err := s.api.Offer(all)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errs.Results, gc.HasLen, len(all.Offers))
	c.Assert(errs.Results[0].Error, gc.ErrorMatches, fmt.Sprintf(".*%v.*", msg))
	s.applicationOffers.CheckCallNames(c, addOffersBackendCall)
}

func (s *applicationOffersSuite) assertList(c *gc.C, expectedErr error) {
	s.setupOffers(c, "test")
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
	c.Assert(found, jc.DeepEquals, params.ListApplicationOffersResults{
		[]params.ApplicationOfferDetails{
			{
				ApplicationOffer: params.ApplicationOffer{
					SourceModelTag:         testing.ModelTag.String(),
					ApplicationDescription: "description",
					OfferName:              "hosted-db2",
					OfferURL:               "fred/prod.hosted-db2",
					Endpoints:              []params.RemoteEndpoint{{Name: "db"}},
					Access:                 "admin",
				},
				CharmName:      "db2",
				ConnectedCount: 5,
			},
		},
	})
	s.applicationOffers.CheckCallNames(c, listOffersBackendCall)
}

func (s *applicationOffersSuite) TestList(c *gc.C) {
	s.authorizer.Tag = names.NewUserTag("admin")
	s.assertList(c, nil)
}

func (s *applicationOffersSuite) TestListPermission(c *gc.C) {
	s.assertList(c, common.ErrPerm)
}

func (s *applicationOffersSuite) TestListError(c *gc.C) {
	s.setupOffers(c, "test")
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
	s.setupOffers(c, "test")
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
	s.setupOffers(c, "test")
	_, err := s.api.ListApplicationOffers(params.OfferFilters{})
	c.Assert(err, gc.ErrorMatches, "at least one offer filter is required")
}

func (s *applicationOffersSuite) assertShow(c *gc.C, expected []params.ApplicationOfferResult) {
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
	ch := &mockCharm{meta: &charm.Meta{Description: "A pretty popular database"}}
	s.mockState.applications = map[string]applicationoffers.Application{
		applicationName: &mockApplication{charm: ch, curl: charm.MustParseURL("db2-2")},
	}
	s.mockState.model = &mockModel{uuid: testing.ModelTag.Id(), name: "prod", owner: "fred"}
	s.mockState.connStatus = &mockConnectionStatus{count: 5}

	found, err := s.api.ApplicationOffers(filter)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.Results, jc.DeepEquals, expected)
	s.applicationOffers.CheckCallNames(c, listOffersBackendCall)
}

func (s *applicationOffersSuite) TestShow(c *gc.C) {
	expected := []params.ApplicationOfferResult{{
		Result: params.ApplicationOffer{
			SourceModelTag:         testing.ModelTag.String(),
			ApplicationDescription: "description",
			OfferURL:               "fred/prod.hosted-db2",
			OfferName:              "hosted-db2",
			Endpoints:              []params.RemoteEndpoint{{Name: "db"}},
			Access:                 "admin"},
	}}
	s.authorizer.Tag = names.NewUserTag("admin")
	s.assertShow(c, expected)
}

func (s *applicationOffersSuite) TestShowNoPermission(c *gc.C) {
	s.mockState.users.Add("someone")
	user := names.NewUserTag("someone")
	offer := names.NewApplicationOfferTag("hosted-db2")
	err := s.mockState.CreateOfferAccess(offer, user, permission.NoAccess)
	c.Assert(err, jc.ErrorIsNil)

	s.authorizer.Tag = user
	expected := []params.ApplicationOfferResult{{
		Error: common.ServerError(errors.NotFoundf("application offer %q", "hosted-db2")),
	}}
	s.assertShow(c, expected)
}

func (s *applicationOffersSuite) TestShowPermission(c *gc.C) {
	user := names.NewUserTag("someone")
	s.authorizer.Tag = user
	expected := []params.ApplicationOfferResult{{
		Result: params.ApplicationOffer{
			SourceModelTag:         testing.ModelTag.String(),
			ApplicationDescription: "description",
			OfferURL:               "fred/prod.hosted-db2",
			OfferName:              "hosted-db2",
			Endpoints:              []params.RemoteEndpoint{{Name: "db"}},
			Access:                 "read"},
	}}
	s.mockState.users.Add(user.Name())
	s.mockState.CreateOfferAccess(names.NewApplicationOfferTag("hosted-db2"), user, permission.ReadAccess)
	s.assertShow(c, expected)
}

func (s *applicationOffersSuite) TestShowError(c *gc.C) {
	url := "fred/prod.hosted-db2"
	filter := params.ApplicationURLs{[]string{url}}
	msg := "fail"

	s.applicationOffers.listOffers = func(filters ...jujucrossmodel.ApplicationOfferFilter) ([]jujucrossmodel.ApplicationOffer, error) {
		return nil, errors.New(msg)
	}
	s.mockState.model = &mockModel{uuid: testing.ModelTag.Id(), name: "prod", owner: "fred"}

	result, err := s.api.ApplicationOffers(filter)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.ErrorMatches, fmt.Sprintf(".*%v.*", msg))
	s.applicationOffers.CheckCallNames(c, listOffersBackendCall)
}

func (s *applicationOffersSuite) TestShowNotFound(c *gc.C) {
	urls := []string{"fred/prod.hosted-db2"}
	filter := params.ApplicationURLs{urls}

	s.applicationOffers.listOffers = func(filters ...jujucrossmodel.ApplicationOfferFilter) ([]jujucrossmodel.ApplicationOffer, error) {
		return nil, nil
	}
	s.mockState.model = &mockModel{uuid: testing.ModelTag.Id(), name: "prod", owner: "fred"}

	found, err := s.api.ApplicationOffers(filter)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.Results, gc.HasLen, 1)
	c.Assert(found.Results[0].Error.Error(), gc.Matches, `application offer "hosted-db2" not found`)
	s.applicationOffers.CheckCallNames(c, listOffersBackendCall)
}

func (s *applicationOffersSuite) TestShowErrorMsgMultipleURLs(c *gc.C) {
	urls := []string{"fred/prod.hosted-mysql", "fred/test.hosted-db2"}
	filter := params.ApplicationURLs{urls}

	s.applicationOffers.listOffers = func(filters ...jujucrossmodel.ApplicationOfferFilter) ([]jujucrossmodel.ApplicationOffer, error) {
		return nil, nil
	}
	s.mockState.model = &mockModel{uuid: testing.ModelTag.Id(), name: "prod", owner: "fred"}
	s.mockState.allmodels = []applicationoffers.Model{
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

func (s *applicationOffersSuite) TestShowFoundMultiple(c *gc.C) {
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
	s.mockState.applications = map[string]applicationoffers.Application{
		"test": &mockApplication{charm: ch, curl: charm.MustParseURL("db2-2")},
	}
	s.mockState.model = &mockModel{uuid: testing.ModelTag.Id(), name: "prod", owner: "fred"}
	s.mockState.allmodels = []applicationoffers.Model{
		s.mockState.model,
		&mockModel{uuid: "uuid2", name: "test", owner: "mary"},
	}
	s.mockState.connStatus = &mockConnectionStatus{count: 5}

	user := names.NewUserTag("someone")
	s.authorizer.Tag = user
	s.mockState.users.Add(user.Name())
	s.mockState.CreateOfferAccess(names.NewApplicationOfferTag("hosted-test"), user, permission.ReadAccess)

	anotherState := &mockState{
		modelUUID:   "uuid2",
		users:       set.NewStrings(),
		accessPerms: make(map[offerAccess]permission.Access),
	}
	anotherState.applications = map[string]applicationoffers.Application{
		"testagain": &mockApplication{charm: ch, curl: charm.MustParseURL("mysql-2")},
	}
	anotherState.connStatus = &mockConnectionStatus{count: 5}
	anotherState.users.Add(user.Name())
	anotherState.CreateOfferAccess(names.NewApplicationOfferTag("hosted-testagain"), user, permission.ConsumeAccess)
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
			SourceModelTag:         testing.ModelTag.String(),
			ApplicationDescription: "description",
			OfferName:              "hosted-" + name,
			OfferURL:               url,
			Access:                 "read",
			Endpoints:              []params.RemoteEndpoint{{Name: "db"}}},
		{
			SourceModelTag:         "model-uuid2",
			ApplicationDescription: "description2",
			OfferName:              "hosted-" + name2,
			OfferURL:               url2,
			Access:                 "consume",
			Endpoints:              []params.RemoteEndpoint{{Name: "db2"}}},
	})
	s.applicationOffers.CheckCallNames(c, listOffersBackendCall, listOffersBackendCall)
}

func (s *applicationOffersSuite) assertFind(c *gc.C, expected []params.ApplicationOffer) {
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

func (s *applicationOffersSuite) TestFind(c *gc.C) {
	s.setupOffers(c, "")
	s.authorizer.Tag = names.NewUserTag("admin")
	expected := []params.ApplicationOffer{
		{
			SourceModelTag:         testing.ModelTag.String(),
			ApplicationDescription: "description",
			OfferName:              "hosted-db2",
			OfferURL:               "fred/prod.hosted-db2",
			Endpoints:              []params.RemoteEndpoint{{Name: "db"}},
			Access:                 "admin"}}
	s.assertFind(c, expected)
}

func (s *applicationOffersSuite) TestFindNoPermission(c *gc.C) {
	s.mockState.users.Add("someone")
	user := names.NewUserTag("someone")
	offer := names.NewApplicationOfferTag("hosted-db2")
	err := s.mockState.CreateOfferAccess(offer, user, permission.NoAccess)
	c.Assert(err, jc.ErrorIsNil)

	s.setupOffers(c, "")
	s.authorizer.Tag = names.NewUserTag("someone")
	s.assertFind(c, []params.ApplicationOffer{})
}

func (s *applicationOffersSuite) TestFindPermission(c *gc.C) {
	s.setupOffers(c, "")
	user := names.NewUserTag("someone")
	s.authorizer.Tag = user
	expected := []params.ApplicationOffer{
		{
			SourceModelTag:         testing.ModelTag.String(),
			ApplicationDescription: "description",
			OfferName:              "hosted-db2",
			OfferURL:               "fred/prod.hosted-db2",
			Endpoints:              []params.RemoteEndpoint{{Name: "db"}},
			Access:                 "read"}}
	s.mockState.users.Add(user.Name())
	s.mockState.CreateOfferAccess(names.NewApplicationOfferTag("hosted-db2"), user, permission.ReadAccess)
	s.assertFind(c, expected)
}

func (s *applicationOffersSuite) TestFindFiltersRequireModel(c *gc.C) {
	s.setupOffers(c, "")
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
	s.setupOffers(c, "")
	_, err := s.api.FindApplicationOffers(params.OfferFilters{})
	c.Assert(err, gc.ErrorMatches, "at least one offer filter is required")
}

func (s *applicationOffersSuite) TestFindMulti(c *gc.C) {
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
	s.mockState.applications = map[string]applicationoffers.Application{
		"db2": &mockApplication{charm: ch, curl: charm.MustParseURL("db2-2")},
	}
	s.mockState.model = &mockModel{uuid: testing.ModelTag.Id(), name: "prod", owner: "fred"}
	s.mockState.connStatus = &mockConnectionStatus{count: 5}

	user := names.NewUserTag("someone")
	s.authorizer.Tag = user
	s.mockState.users.Add(user.Name())
	s.mockState.CreateOfferAccess(names.NewApplicationOfferTag("hosted-db2"), user, permission.ConsumeAccess)

	anotherState := &mockState{
		modelUUID:   "uuid2",
		users:       set.NewStrings(),
		accessPerms: make(map[offerAccess]permission.Access),
	}
	s.mockStatePool.st["uuid2"] = anotherState
	anotherState.applications = map[string]applicationoffers.Application{
		"mysql":      &mockApplication{charm: ch, curl: charm.MustParseURL("mysql-2")},
		"postgresql": &mockApplication{charm: ch, curl: charm.MustParseURL("postgresql-2")},
	}
	anotherState.model = &mockModel{uuid: "uuid2", name: "another", owner: "mary"}
	anotherState.connStatus = &mockConnectionStatus{count: 15}
	anotherState.users.Add(user.Name())
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
		},
	}
	found, err := s.api.FindApplicationOffers(filter)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found, jc.DeepEquals, params.FindApplicationOffersResults{
		[]params.ApplicationOffer{
			{
				SourceModelTag:         testing.ModelTag.String(),
				ApplicationDescription: "db2 description",
				OfferName:              "hosted-db2",
				OfferURL:               "fred/prod.hosted-db2",
				Access:                 "consume",
				Endpoints:              []params.RemoteEndpoint{{Name: "db"}},
			},
			{
				SourceModelTag:         "model-uuid2",
				ApplicationDescription: "mysql description",
				OfferName:              "hosted-mysql",
				OfferURL:               "mary/another.hosted-mysql",
				Access:                 "read",
				Endpoints:              []params.RemoteEndpoint{{Name: "db"}},
			},
			{
				SourceModelTag:         "model-uuid2",
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
	s.mockState.model = &mockModel{uuid: testing.ModelTag.Id(), name: "prod", owner: "fred"}

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
	env *mockEnviron
	api *applicationoffers.OffersAPI
}

var _ = gc.Suite(&consumeSuite{})

func (s *consumeSuite) SetUpTest(c *gc.C) {
	s.baseSuite.SetUpTest(c)
	getApplicationOffers := func(st interface{}) jujucrossmodel.ApplicationOffers {
		return &mockApplicationOffers{st: st.(*mockState)}
	}

	resources := common.NewResources()
	resources.RegisterNamed("dataDir", common.StringResource(c.MkDir()))

	s.env = &mockEnviron{}
	getEnviron := func(modelUUID string) (environs.Environ, error) {
		return s.env, nil
	}
	var err error
	s.api, err = applicationoffers.CreateOffersAPI(
		getApplicationOffers, getEnviron, s.mockState, s.mockStatePool, s.authorizer, resources,
	)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *consumeSuite) setupTargetModel() names.ModelTag {
	targetModelTag := names.NewModelTag(utils.MustNewUUID().String())
	targetSt := &mockState{
		modelUUID:          targetModelTag.Id(),
		applications:       make(map[string]applicationoffers.Application),
		remoteApplications: make(map[string]applicationoffers.RemoteApplication),
		applicationOffers:  make(map[string]jujucrossmodel.ApplicationOffer),
		users:              set.NewStrings(),
		accessPerms:        make(map[offerAccess]permission.Access),
	}
	s.mockStatePool.st[targetModelTag.Id()] = targetSt
	targetSt.model = &mockModel{uuid: targetModelTag.Id(), name: "target", owner: "fred"}
	targetSt.modelUUID = targetModelTag.Id()
	return targetModelTag
}

func (s *consumeSuite) TestConsumeIdempotent(c *gc.C) {
	targetModelTag := s.setupTargetModel()
	for i := 0; i < 2; i++ {
		results, err := s.api.Consume(params.ConsumeApplicationArgs{
			Args: []params.ConsumeApplicationArg{{
				TargetModelTag: targetModelTag.String(),
				ApplicationOffer: params.ApplicationOffer{
					SourceModelTag:         testing.ModelTag.String(),
					OfferName:              "hosted-mysql",
					ApplicationDescription: "a database",
					Endpoints:              []params.RemoteEndpoint{{Name: "database", Interface: "mysql", Role: "provider"}},
					OfferURL:               "othermodel.hosted-mysql",
				},
			}},
		})
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(results.Results, gc.HasLen, 1)
		c.Assert(results.Results[0].Error, gc.IsNil)
	}
	obtained, ok := s.mockStatePool.st[targetModelTag.Id()].(*mockState).remoteApplications["hosted-mysql"]
	c.Assert(ok, jc.IsTrue)
	c.Assert(obtained, jc.DeepEquals, &mockRemoteApplication{
		name:           "hosted-mysql",
		sourceModelTag: testing.ModelTag,
		offerName:      "hosted-mysql",
		offerURL:       "othermodel.hosted-mysql",
		endpoints: []state.Endpoint{
			{ApplicationName: "hosted-mysql", Relation: charm.Relation{Name: "database", Interface: "mysql", Role: "provider"}}},
	})
}

func (s *consumeSuite) TestConsumeIncludesSpaceInfo(c *gc.C) {
	targetModelTag := s.setupTargetModel()
	s.env.spaceInfo = &environs.ProviderSpaceInfo{
		CloudType: "grandaddy",
		ProviderAttributes: map[string]interface{}{
			"thunderjaws": 1,
		},
		SpaceInfo: network.SpaceInfo{
			Name:       "yourspace",
			ProviderId: "juju-space-myspace",
			Subnets: []network.SubnetInfo{{
				CIDR:              "5.6.7.0/24",
				ProviderId:        "juju-subnet-1",
				AvailabilityZones: []string{"az1"},
			}},
		},
	}

	results, err := s.api.Consume(params.ConsumeApplicationArgs{
		Args: []params.ConsumeApplicationArg{{
			TargetModelTag:   targetModelTag.String(),
			ApplicationAlias: "beirut",
			ApplicationOffer: params.ApplicationOffer{
				SourceModelTag:         testing.ModelTag.String(),
				OfferName:              "hosted-mysql",
				ApplicationDescription: "a database",
				Endpoints:              []params.RemoteEndpoint{{Name: "server", Interface: "mysql", Role: "provider"}},
				OfferURL:               "othermodel.hosted-mysql",
				Bindings:               map[string]string{"server": "myspace"},
				Spaces: []params.RemoteSpace{
					{
						CloudType:  "grandaddy",
						Name:       "myspace",
						ProviderId: "juju-space-myspace",
						ProviderAttributes: map[string]interface{}{
							"thunderjaws": 1,
						},
						Subnets: []params.Subnet{{
							CIDR:       "5.6.7.0/24",
							ProviderId: "juju-subnet-1",
							Zones:      []string{"az1"},
						}},
					},
				},
			},
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)
	c.Assert(results.Results[0].LocalName, gc.Equals, "beirut")

	obtained, ok := s.mockStatePool.st[targetModelTag.Id()].(*mockState).remoteApplications["beirut"]
	c.Assert(ok, jc.IsTrue)
	endpoints, err := obtained.Endpoints()
	c.Assert(err, jc.ErrorIsNil)
	epNames := make([]string, len(endpoints))
	for i, ep := range endpoints {
		epNames[i] = ep.Name
	}
	c.Assert(epNames, jc.SameContents, []string{"server"})
	c.Assert(obtained.Bindings(), jc.DeepEquals, map[string]string{"server": "myspace"})
	c.Assert(obtained.Spaces(), jc.DeepEquals, []state.RemoteSpace{{
		CloudType:  "grandaddy",
		Name:       "myspace",
		ProviderId: "juju-space-myspace",
		ProviderAttributes: map[string]interface{}{
			"thunderjaws": 1,
		},
		Subnets: []state.RemoteSubnet{{
			CIDR:              "5.6.7.0/24",
			ProviderId:        "juju-subnet-1",
			AvailabilityZones: []string{"az1"},
		}},
	}})
}

// TODO(wallyworld) - re-implement when OfferDetails is done
//func (s *consumeSuite) TestConsumeRejectsEndpoints(c *gc.C) {
//	results, err := s.api.Consume(params.ConsumeApplicationArgs{
//		Args: []params.ConsumeApplicationArg{{ApplicationURL: "othermodel.application:db"}},
//	})
//	c.Assert(err, jc.ErrorIsNil)
//	c.Assert(results.Results, gc.HasLen, 1)
//	c.Assert(results.Results[0].Error != nil, jc.IsTrue)
//	c.Assert(results.Results[0].Error.Message, gc.Equals, `remote application "othermodel.application:db" shouldn't include endpoint`)
//}
//
//func (s *consumeSuite) TestConsumeNoPermission(c *gc.C) {
//	s.setupOffer()
//	s.mockState.users.Add("someone")
//	user := names.NewUserTag("someone")
//	offer := names.NewApplicationOfferTag("hosted-mysql")
//	err := s.mockState.CreateOfferAccess(offer, user, permission.NoAccess)
//	c.Assert(err, jc.ErrorIsNil)
//
//	targetModelTag := s.setupTargetModel()
//
//	s.authorizer.Tag = names.NewUserTag("someone")
//	results, err := s.api.Consume(params.ConsumeApplicationArgs{
//		Args: []params.ConsumeApplicationArg{{
//			SourceModelTag:         testing.ModelTag.String(),
//			OfferName:              "hosted-mysql",
//			ApplicationDescription: "a database",
//			Endpoints:              []params.RemoteEndpoint{{Name: "database", Interface: "mysql", Role: "provider"}},
//			OfferURL:               "othermodel.hosted-mysql",
//			ApplicationAlias:       "mysql",
//			TargetModelTag:         targetModelTag.String(),
//		}},
//	})
//	c.Assert(err, jc.ErrorIsNil)
//	c.Assert(results.Results, gc.HasLen, 1)
//	c.Assert(results.Results[0].Error, gc.ErrorMatches, ".*permission denied.*")
//}
//
//func (s *consumeSuite) TestConsumeWithPermission(c *gc.C) {
//	st := s.mockStatePool.st[testing.ModelTag.Id()]
//	st.(*mockState).users.Add("foobar")

//_, err := s.otherModel.AddUser("someone", "spmeone", "secret", "admin")
//c.Assert(err, jc.ErrorIsNil)
//apiUser := names.NewUserTag("someone")
//err = s.otherModel.CreateOfferAccess(
//	names.NewApplicationOfferTag("hosted-mysql"), apiUser, permission.ConsumeAccess)
//s.authorizer.Tag = apiUser
//results, err := s.api.Consume(params.ConsumeApplicationArgs{
//	Args: []params.ConsumeApplicationArg{
//		{ApplicationURL: "admin/othermodel.hosted-mysql"},
//	},
//})
//c.Assert(err, jc.ErrorIsNil)
//c.Assert(results.Results, gc.HasLen, 1)
//c.Assert(results.Results[0].Error, gc.IsNil)
//}

func (s *consumeSuite) setupOffer() {
	modelUUID := testing.ModelTag.Id()
	offerName := "hosted-mysql"

	s.mockState.allmodels = []applicationoffers.Model{
		&mockModel{uuid: modelUUID, name: "prod", owner: "fred"}}
	st := &mockState{
		modelUUID:          modelUUID,
		applications:       make(map[string]applicationoffers.Application),
		remoteApplications: make(map[string]applicationoffers.RemoteApplication),
		applicationOffers:  make(map[string]jujucrossmodel.ApplicationOffer),
		users:              set.NewStrings(),
		accessPerms:        make(map[offerAccess]permission.Access),
		spaces:             make(map[string]applicationoffers.Space),
	}
	s.mockStatePool.st[modelUUID] = st
	anOffer := jujucrossmodel.ApplicationOffer{
		ApplicationName:        "mysql",
		ApplicationDescription: "a database",
		OfferName:              offerName,
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
	st.(*mockState).users.Add("foobar")

	// Give user permission to see the offer.
	user := names.NewUserTag("foobar")
	offer := names.NewApplicationOfferTag("hosted-mysql")
	err := st.CreateOfferAccess(offer, user, permission.ConsumeAccess)
	c.Assert(err, jc.ErrorIsNil)

	s.authorizer.Tag = user
	results, err := s.api.RemoteApplicationInfo(params.ApplicationURLs{
		ApplicationURLs: []string{"fred/prod.hosted-mysql", "fred/prod.unknown"},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 2)
	c.Assert(results.Results[0].Error, gc.IsNil)
	c.Assert(results.Results, jc.DeepEquals, []params.RemoteApplicationInfoResult{
		{Result: &params.RemoteApplicationInfo{
			ModelTag:         testing.ModelTag.String(),
			Name:             "hosted-mysql",
			Description:      "a database",
			ApplicationURL:   "fred/prod.hosted-mysql",
			SourceModelLabel: "prod",
			IconURLPath:      "rest/1.0/remote-application/hosted-mysql/icon",
			Endpoints: []params.RemoteEndpoint{
				{Name: "database", Role: "provider", Interface: "mysql", Limit: 0, Scope: "global"}},
		}},
		{
			Error: &params.Error{Message: `application offer "unknown" not found`, Code: "not found"},
		},
	})
	// TODO(wallyworld) - these checks are only relevant once OfferDetails is done
	//s.env.stub.CheckCallNames(c, "ProviderSpaceInfo")
	//s.env.stub.CheckCall(c, 0, "ProviderSpaceInfo", &network.SpaceInfo{
	//	Name:       "myspace",
	//	ProviderId: "juju-space-myspace",
	//	Subnets: []network.SubnetInfo{{
	//		CIDR:              "4.3.2.0/24",
	//		ProviderId:        "juju-subnet-1",
	//		AvailabilityZones: []string{"az1"},
	//	}},
	//})
}
