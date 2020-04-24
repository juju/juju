// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package applicationoffers_test

import (
	"fmt"
	"time"

	"github.com/juju/charm/v7"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/applicationoffers"
	basetesting "github.com/juju/juju/api/base/testing"
	apitesting "github.com/juju/juju/api/testing"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	jujucrossmodel "github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/testing"
)

type crossmodelMockSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&crossmodelMockSuite{})

func (s *crossmodelMockSuite) TestOffer(c *gc.C) {
	application := "shared"
	endPointA := "endPointA"
	endPointB := "endPointB"
	offer := "offer"
	desc := "desc"

	msg := "fail"
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Check(objType, gc.Equals, "ApplicationOffers")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "Offer")

			args, ok := a.(params.AddApplicationOffers)
			c.Assert(ok, jc.IsTrue)
			c.Assert(args.Offers, gc.HasLen, 1)

			offer := args.Offers[0]
			c.Assert(offer.ModelTag, gc.Equals, "model-uuid")
			c.Assert(offer.ApplicationName, gc.Equals, application)
			c.Assert(offer.Endpoints, jc.DeepEquals, map[string]string{endPointA: endPointA, endPointB: endPointB})
			c.Assert(offer.OfferName, gc.Equals, offer.OfferName)
			c.Assert(offer.ApplicationDescription, gc.Equals, desc)

			if results, ok := result.(*params.ErrorResults); ok {
				all := make([]params.ErrorResult, len(args.Offers))
				// add one error to make sure it's catered for.
				all = append(all, params.ErrorResult{
					Error: common.ServerError(errors.New(msg))})
				results.Results = all
			}

			return nil
		})

	client := applicationoffers.NewClient(apiCaller)
	results, err := client.Offer("uuid", application, []string{endPointA, endPointB}, offer, desc)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, 2)
	c.Assert(results, jc.DeepEquals,
		[]params.ErrorResult{
			{},
			{Error: common.ServerError(errors.New(msg))},
		})
}

func (s *crossmodelMockSuite) TestOfferFacadeCallError(c *gc.C) {
	msg := "facade failure"
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Check(objType, gc.Equals, "ApplicationOffers")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "Offer")

			return errors.New(msg)
		})
	client := applicationoffers.NewClient(apiCaller)
	results, err := client.Offer("", "", nil, "", "")
	c.Assert(errors.Cause(err), gc.ErrorMatches, msg)
	c.Assert(results, gc.IsNil)
}

func (s *crossmodelMockSuite) TestList(c *gc.C) {
	offerName := "hosted-db2"
	url := fmt.Sprintf("fred/model.%s", offerName)
	endpoints := []params.RemoteEndpoint{{Name: "endPointA"}}
	relations := []jujucrossmodel.EndpointFilterTerm{{Name: "endPointA", Interface: "http"}}

	filter := jujucrossmodel.ApplicationOfferFilter{
		OwnerName:        "fred",
		ModelName:        "prod",
		OfferName:        offerName,
		Endpoints:        relations,
		ApplicationName:  "mysql",
		AllowedConsumers: []string{"allowed"},
		ConnectedUsers:   []string{"connected"},
	}
	called := false
	since := time.Now()
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Check(objType, gc.Equals, "ApplicationOffers")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "ListApplicationOffers")

			called = true
			args, ok := a.(params.OfferFilters)
			c.Assert(ok, jc.IsTrue)
			c.Assert(args.Filters, gc.HasLen, 1)
			c.Assert(args.Filters[0], jc.DeepEquals, params.OfferFilter{
				OwnerName:       "fred",
				ModelName:       "prod",
				OfferName:       filter.OfferName,
				ApplicationName: filter.ApplicationName,
				Endpoints: []params.EndpointFilterAttributes{{
					Name:      "endPointA",
					Interface: "http",
				}},
				AllowedConsumerTags: []string{"user-allowed"},
				ConnectedUserTags:   []string{"user-connected"},
			})

			if results, ok := result.(*params.QueryApplicationOffersResults); ok {
				offer := params.ApplicationOfferDetails{
					OfferURL:  url,
					OfferName: offerName,
					OfferUUID: offerName + "-uuid",
					Endpoints: endpoints,
					Users: []params.OfferUserDetails{
						{UserName: "fred", DisplayName: "Fred", Access: "consume"},
					},
				}
				results.Results = []params.ApplicationOfferAdminDetails{{
					ApplicationOfferDetails: offer,
					ApplicationName:         "db2-app",
					CharmURL:                "cs:db2-5",
					Connections: []params.OfferConnection{
						{SourceModelTag: testing.ModelTag.String(), Username: "fred", RelationId: 3,
							Endpoint: "db", Status: params.EntityStatus{Status: "joined", Info: "message", Since: &since},
							IngressSubnets: []string{"10.0.0.0/8"},
						},
					},
				}}
			}
			return nil
		})

	client := applicationoffers.NewClient(apiCaller)
	results, err := client.ListOffers(filter)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
	c.Assert(results, jc.DeepEquals, []*jujucrossmodel.ApplicationOfferDetails{{
		OfferURL:        url,
		OfferName:       offerName,
		Endpoints:       []charm.Relation{{Name: "endPointA"}},
		ApplicationName: "db2-app",
		CharmURL:        "cs:db2-5",
		Connections: []jujucrossmodel.OfferConnection{
			{SourceModelUUID: testing.ModelTag.Id(), Username: "fred", RelationId: 3,
				Endpoint: "db", Status: "joined", Message: "message", Since: &since,
				IngressSubnets: []string{"10.0.0.0/8"},
			},
		},
		Users: []jujucrossmodel.OfferUserDetails{
			{UserName: "fred", DisplayName: "Fred", Access: "consume"},
		},
	}})
}

func (s *crossmodelMockSuite) TestListError(c *gc.C) {
	msg := "find failure"
	called := false
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Check(objType, gc.Equals, "ApplicationOffers")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "ListApplicationOffers")

			called = true
			return errors.New(msg)
		})

	client := applicationoffers.NewClient(apiCaller)
	filter := jujucrossmodel.ApplicationOfferFilter{
		OfferName: "foo",
	}
	_, err := client.ListOffers(filter)
	c.Assert(errors.Cause(err), gc.ErrorMatches, msg)
	c.Assert(called, jc.IsTrue)
}

func (s *crossmodelMockSuite) TestListFacadeCallError(c *gc.C) {
	msg := "facade failure"
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Check(objType, gc.Equals, "ApplicationOffers")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "ListApplicationOffers")

			return errors.New(msg)
		})
	client := applicationoffers.NewClient(apiCaller)
	filter := jujucrossmodel.ApplicationOfferFilter{
		OfferName: "foo",
	}
	results, err := client.ListOffers(filter)
	c.Assert(errors.Cause(err), gc.ErrorMatches, msg)
	c.Assert(results, gc.IsNil)
}

func (s *crossmodelMockSuite) TestShow(c *gc.C) {
	url := "fred/model.db2"

	desc := "IBM DB2 Express Server Edition is an entry level database system"
	endpoints := []params.RemoteEndpoint{
		{Name: "db2", Interface: "db2", Role: charm.RoleProvider},
		{Name: "log", Interface: "http", Role: charm.RoleRequirer},
	}
	offerName := "hosted-db2"
	access := "consume"
	since := time.Now()
	called := false

	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			called = true

			c.Check(objType, gc.Equals, "ApplicationOffers")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "ApplicationOffers")

			args, ok := a.(params.OfferURLs)
			c.Assert(ok, jc.IsTrue)
			c.Assert(args.OfferURLs, gc.DeepEquals, []string{url})

			if offers, ok := result.(*params.ApplicationOffersResults); ok {
				offers.Results = []params.ApplicationOfferResult{
					{Result: &params.ApplicationOfferAdminDetails{
						ApplicationOfferDetails: params.ApplicationOfferDetails{
							ApplicationDescription: desc,
							Endpoints:              endpoints,
							OfferURL:               url,
							OfferName:              offerName,
							Users: []params.OfferUserDetails{
								{UserName: "fred", DisplayName: "Fred", Access: access},
							},
						},
						ApplicationName: "db2-app",
						CharmURL:        "cs:db2-5",
						Connections: []params.OfferConnection{
							{SourceModelTag: testing.ModelTag.String(), Username: "fred", RelationId: 3,
								Endpoint: "db", Status: params.EntityStatus{Status: "joined", Info: "message", Since: &since},
								IngressSubnets: []string{"10.0.0.0/8"},
							},
						},
					}},
				}
			}
			return nil
		})
	client := applicationoffers.NewClient(apiCaller)
	results, err := client.ApplicationOffer(url)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(called, jc.IsTrue)
	c.Assert(results, jc.DeepEquals, &jujucrossmodel.ApplicationOfferDetails{
		OfferURL:  url,
		OfferName: offerName,
		Endpoints: []charm.Relation{
			{Name: "db2", Role: "provider", Interface: "db2", Optional: false, Limit: 0, Scope: ""},
			{Name: "log", Role: "requirer", Interface: "http", Optional: false, Limit: 0, Scope: ""}},
		ApplicationName:        "db2-app",
		ApplicationDescription: "IBM DB2 Express Server Edition is an entry level database system",
		CharmURL:               "cs:db2-5",
		Users: []jujucrossmodel.OfferUserDetails{
			{UserName: "fred", DisplayName: "Fred", Access: "consume"},
		},
		Connections: []jujucrossmodel.OfferConnection{
			{SourceModelUUID: testing.ModelTag.Id(), Username: "fred", RelationId: 3,
				Endpoint: "db", Status: "joined", Message: "message", Since: &since,
				IngressSubnets: []string{"10.0.0.0/8"},
			},
		},
	})
}

func (s *crossmodelMockSuite) TestShowURLError(c *gc.C) {
	url := "fred/model.db2"
	msg := "facade failure"

	called := false

	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			called = true

			c.Check(objType, gc.Equals, "ApplicationOffers")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "ApplicationOffers")

			args, ok := a.(params.OfferURLs)
			c.Assert(ok, jc.IsTrue)
			c.Assert(args.OfferURLs, gc.DeepEquals, []string{url})

			if points, ok := result.(*params.ApplicationOffersResults); ok {
				points.Results = []params.ApplicationOfferResult{
					{Error: common.ServerError(errors.New(msg))}}
			}
			return nil
		})
	client := applicationoffers.NewClient(apiCaller)
	found, err := client.ApplicationOffer(url)

	c.Assert(errors.Cause(err), gc.ErrorMatches, msg)
	c.Assert(found, gc.IsNil)
	c.Assert(called, jc.IsTrue)
}

func (s *crossmodelMockSuite) TestShowMultiple(c *gc.C) {
	url := "fred/model.db2"

	desc := "IBM DB2 Express Server Edition is an entry level database system"
	endpoints := []params.RemoteEndpoint{
		{Name: "db2", Interface: "db2", Role: charm.RoleProvider},
		{Name: "log", Interface: "http", Role: charm.RoleRequirer},
	}
	offerName := "hosted-db2"
	called := false

	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			called = true

			c.Check(objType, gc.Equals, "ApplicationOffers")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "ApplicationOffers")

			args, ok := a.(params.OfferURLs)
			c.Assert(ok, jc.IsTrue)
			c.Assert(args.OfferURLs, gc.DeepEquals, []string{url})

			if offers, ok := result.(*params.ApplicationOffersResults); ok {
				offers.Results = []params.ApplicationOfferResult{
					{Result: &params.ApplicationOfferAdminDetails{
						ApplicationOfferDetails: params.ApplicationOfferDetails{
							ApplicationDescription: desc,
							Endpoints:              endpoints,
							OfferURL:               url,
							OfferName:              offerName,
						},
					}},
					{Result: &params.ApplicationOfferAdminDetails{
						ApplicationOfferDetails: params.ApplicationOfferDetails{
							ApplicationDescription: desc,
							Endpoints:              endpoints,
							OfferURL:               url,
							OfferName:              offerName,
						},
					}}}
			}
			return nil
		})
	client := applicationoffers.NewClient(apiCaller)
	found, err := client.ApplicationOffer(url)
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf(`expected to find one result for url %q but found 2`, url))
	c.Assert(found, gc.IsNil)

	c.Assert(called, jc.IsTrue)
}

func (s *crossmodelMockSuite) TestShowNonLocal(c *gc.C) {
	url := "jaas:fred/model.db2"
	msg := "query for non-local application offers not supported"

	called := false

	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			called = true
			return nil
		})
	client := applicationoffers.NewClient(apiCaller)
	found, err := client.ApplicationOffer(url)

	c.Assert(errors.Cause(err), gc.ErrorMatches, msg)
	c.Assert(found, gc.IsNil)
	c.Assert(called, jc.IsFalse)
}

func (s *crossmodelMockSuite) TestShowFacadeCallError(c *gc.C) {
	msg := "facade failure"
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Check(objType, gc.Equals, "ApplicationOffers")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "ApplicationOffers")

			return errors.New(msg)
		})
	client := applicationoffers.NewClient(apiCaller)
	found, err := client.ApplicationOffer("fred/model.db2")
	c.Assert(errors.Cause(err), gc.ErrorMatches, msg)
	c.Assert(found, gc.IsNil)
}

func (s *crossmodelMockSuite) TestFind(c *gc.C) {
	offerName := "hosted-db2"
	ownerName := "owner"
	modelName := "model"
	url := fmt.Sprintf("fred/model.%s", offerName)
	endpoints := []params.RemoteEndpoint{{Name: "endPointA"}}
	relations := []jujucrossmodel.EndpointFilterTerm{{Name: "endPointA", Interface: "http"}}

	filter := jujucrossmodel.ApplicationOfferFilter{
		OwnerName: ownerName,
		ModelName: modelName,
		OfferName: offerName,
		Endpoints: relations,
	}

	called := false
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Check(objType, gc.Equals, "ApplicationOffers")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "FindApplicationOffers")

			called = true
			args, ok := a.(params.OfferFilters)
			c.Assert(ok, jc.IsTrue)
			c.Assert(args.Filters, gc.HasLen, 1)
			c.Assert(args.Filters[0], jc.DeepEquals, params.OfferFilter{
				OwnerName:       filter.OwnerName,
				ModelName:       filter.ModelName,
				OfferName:       filter.OfferName,
				ApplicationName: filter.ApplicationName,
				Endpoints: []params.EndpointFilterAttributes{{
					Name:      "endPointA",
					Interface: "http",
				}},
			})

			if results, ok := result.(*params.QueryApplicationOffersResults); ok {
				offer := params.ApplicationOfferDetails{
					OfferURL:  url,
					OfferName: offerName,
					Endpoints: endpoints,
					Users: []params.OfferUserDetails{
						{UserName: "fred", DisplayName: "Fred", Access: "consume"},
					},
				}
				results.Results = []params.ApplicationOfferAdminDetails{{
					ApplicationOfferDetails: offer,
				}}
			}
			return nil
		})

	client := applicationoffers.NewClient(apiCaller)
	results, err := client.FindApplicationOffers(filter)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
	c.Assert(results, jc.DeepEquals, []*jujucrossmodel.ApplicationOfferDetails{{
		OfferURL:  url,
		OfferName: offerName,
		Endpoints: []charm.Relation{{Name: "endPointA"}},
		Users: []jujucrossmodel.OfferUserDetails{
			{UserName: "fred", DisplayName: "Fred", Access: "consume"},
		},
	}})
}

func (s *crossmodelMockSuite) TestFindNoFilter(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Fail()
			return nil
		})

	client := applicationoffers.NewClient(apiCaller)
	_, err := client.FindApplicationOffers()
	c.Assert(err, gc.ErrorMatches, "at least one filter must be specified")
}

func (s *crossmodelMockSuite) TestFindError(c *gc.C) {
	msg := "find failure"
	called := false
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Check(objType, gc.Equals, "ApplicationOffers")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "FindApplicationOffers")

			called = true
			return errors.New(msg)
		})

	client := applicationoffers.NewClient(apiCaller)
	filter := jujucrossmodel.ApplicationOfferFilter{
		OfferName: "foo",
	}
	_, err := client.FindApplicationOffers(filter)
	c.Assert(errors.Cause(err), gc.ErrorMatches, msg)
	c.Assert(called, jc.IsTrue)
}

func (s *crossmodelMockSuite) TestFindFacadeCallError(c *gc.C) {
	msg := "facade failure"
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Check(objType, gc.Equals, "ApplicationOffers")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "FindApplicationOffers")

			return errors.New(msg)
		})
	client := applicationoffers.NewClient(apiCaller)
	filter := jujucrossmodel.ApplicationOfferFilter{
		OfferName: "foo",
	}
	results, err := client.FindApplicationOffers(filter)
	c.Assert(errors.Cause(err), gc.ErrorMatches, msg)
	c.Assert(results, gc.IsNil)
}

func (s *crossmodelMockSuite) TestGetConsumeDetails(c *gc.C) {
	offer := params.ApplicationOfferDetails{
		SourceModelTag:         "source model",
		OfferName:              "an offer",
		OfferURL:               "offer url",
		ApplicationDescription: "description",
		Endpoints:              []params.RemoteEndpoint{{Name: "endpoint"}},
	}
	controllerInfo := &params.ExternalControllerInfo{
		Addrs: []string{"1.2.3.4"},
	}
	mac, err := apitesting.NewMacaroon("id")
	c.Assert(err, jc.ErrorIsNil)
	var called bool
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			called = true
			c.Assert(request, gc.Equals, "GetConsumeDetails")
			args, ok := a.(params.OfferURLs)
			c.Assert(ok, jc.IsTrue)
			c.Assert(args.OfferURLs, jc.DeepEquals, []string{"me/prod.app"})
			if results, ok := result.(*params.ConsumeOfferDetailsResults); ok {
				result := params.ConsumeOfferDetailsResult{
					ConsumeOfferDetails: params.ConsumeOfferDetails{
						Offer:          &offer,
						Macaroon:       mac,
						ControllerInfo: controllerInfo,
					},
				}
				results.Results = []params.ConsumeOfferDetailsResult{result}
			}
			return nil
		})
	client := applicationoffers.NewClient(apiCaller)
	details, err := client.GetConsumeDetails("me/prod.app")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
	c.Assert(details, jc.DeepEquals, params.ConsumeOfferDetails{
		Offer:          &offer,
		Macaroon:       mac,
		ControllerInfo: controllerInfo,
	})
}

func (s *crossmodelMockSuite) TestGetConsumeDetailsBadURL(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			return errors.New("should not be called")
		})
	client := applicationoffers.NewClient(apiCaller)
	_, err := client.GetConsumeDetails("badurl")
	c.Assert(err, gc.ErrorMatches, "application offer URL is missing application")
}

func (s *crossmodelMockSuite) TestDestroyOffers(c *gc.C) {
	var called bool
	apiCaller := basetesting.BestVersionCaller{
		APICallerFunc: basetesting.APICallerFunc(
			func(objType string,
				version int,
				id, request string,
				a, result interface{},
			) error {
				called = true
				c.Assert(request, gc.Equals, "DestroyOffers")
				args, ok := a.(params.DestroyApplicationOffers)
				c.Assert(ok, jc.IsTrue)
				c.Assert(args.Force, jc.IsTrue)
				c.Assert(args.OfferURLs, jc.DeepEquals, []string{"me/prod.app"})
				if results, ok := result.(*params.ErrorResults); ok {
					results.Results = []params.ErrorResult{{
						Error: &params.Error{Message: "fail"},
					}}
				}
				return nil
			},
		),
		BestVersion: 2,
	}
	client := applicationoffers.NewClient(apiCaller)
	err := client.DestroyOffers(true, "me/prod.app")
	c.Assert(err, gc.ErrorMatches, "fail")
	c.Assert(called, jc.IsTrue)
}

func (s *crossmodelMockSuite) TestDestroyOffersForce(c *gc.C) {
	apiCaller := basetesting.BestVersionCaller{
		APICallerFunc: basetesting.APICallerFunc(
			func(objType string,
				version int,
				id, request string,
				a, result interface{},
			) error {
				c.Fail()
				return nil
			},
		),
		BestVersion: 1,
	}
	client := applicationoffers.NewClient(apiCaller)
	err := client.DestroyOffers(true, "offer-url")

	c.Assert(err, gc.ErrorMatches, "DestroyOffers\\(\\).* not implemented")
}
