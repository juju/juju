// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"

	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/crossmodel"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	jujucrossmodel "github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/testing"
)

type serviceDirectorySuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&serviceDirectorySuite{})

func serviceForURLCaller(c *gc.C, offers []params.ApplicationOffer, err string) basetesting.APICallerFunc {
	return basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Check(objType, gc.Equals, "ApplicationOffers")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "ListOffers")

			args, ok := a.(params.OfferFilters)
			c.Assert(ok, jc.IsTrue)
			c.Assert(args.Filters, gc.HasLen, 1)

			filter := args.Filters[0]
			c.Check(filter.ApplicationURL, gc.DeepEquals, "local:/u/user/servicename")
			c.Check(filter.AllowedUserTags, jc.SameContents, []string{"user-foo"})
			c.Check(filter.Endpoints, gc.HasLen, 0)
			c.Check(filter.ApplicationName, gc.Equals, "")
			c.Check(filter.ApplicationDescription, gc.Equals, "")
			c.Check(filter.ApplicationUser, gc.Equals, "")
			c.Check(filter.SourceLabel, gc.Equals, "")

			if results, ok := result.(*params.ApplicationOfferResults); ok {
				results.Offers = offers
				if err != "" {
					results.Error = common.ServerError(errors.New(err))
				}
			}
			return nil
		})
}

var fakeUUID = "df136476-12e9-11e4-8a70-b2227cce2b54"

func (s *serviceDirectorySuite) TestServiceForURL(c *gc.C) {
	endpoints := []params.RemoteEndpoint{
		{
			Name:      "db",
			Role:      charm.RoleProvider,
			Interface: "mysql",
		},
	}
	offers := []params.ApplicationOffer{
		{
			ApplicationURL:  "local:/u/user/servicename",
			ApplicationName: "service",
			SourceModelTag:  "model-" + fakeUUID,
			Endpoints:       endpoints,
		},
	}
	apiCaller := serviceForURLCaller(c, offers, "")
	client := crossmodel.NewApplicationOffers(apiCaller)
	result, err := jujucrossmodel.ApplicationOfferForURL(client, "local:/u/user/servicename", "foo")
	c.Assert(err, jc.ErrorIsNil)
	expectedOffer, err := crossmodel.MakeOfferFromParams(offers[0])
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, expectedOffer)
}

func (s *serviceDirectorySuite) TestServiceForURLNoneOrNoAccess(c *gc.C) {
	apiCaller := serviceForURLCaller(c, []params.ApplicationOffer{}, "")
	client := crossmodel.NewApplicationOffers(apiCaller)
	_, err := jujucrossmodel.ApplicationOfferForURL(client, "local:/u/user/servicename", "foo")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *serviceDirectorySuite) TestServiceForURLError(c *gc.C) {
	apiCaller := serviceForURLCaller(c, nil, "error")
	client := crossmodel.NewApplicationOffers(apiCaller)
	_, err := jujucrossmodel.ApplicationOfferForURL(client, "local:/u/user/servicename", "foo")
	c.Assert(err, gc.ErrorMatches, "error")
}

func listOffersCaller(c *gc.C, offers []params.ApplicationOffer, err string) basetesting.APICallerFunc {
	return basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Check(objType, gc.Equals, "ApplicationOffers")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "ListOffers")

			args, ok := a.(params.OfferFilters)
			c.Assert(ok, jc.IsTrue)
			c.Assert(args.Filters, gc.HasLen, 1)

			filter := args.Filters[0]
			c.Check(filter.ApplicationName, gc.Equals, "service")
			c.Check(filter.ApplicationDescription, gc.Equals, "description")
			c.Check(filter.SourceModelUUIDTag, gc.Equals, "model-"+fakeUUID)

			if results, ok := result.(*params.ApplicationOfferResults); ok {
				results.Offers = offers
				if err != "" {
					results.Error = common.ServerError(errors.New(err))
				}
			}
			return nil
		})
}

func (s *serviceDirectorySuite) TestListOffers(c *gc.C) {
	endpoints := []params.RemoteEndpoint{
		{
			Name:      "db",
			Role:      charm.RoleProvider,
			Interface: "mysql",
		},
	}
	offers := []params.ApplicationOffer{
		{
			ApplicationURL:  "local:/u/user/servicename",
			ApplicationName: "service",
			SourceModelTag:  "model-" + fakeUUID,
			Endpoints:       endpoints,
		},
	}
	apiCaller := listOffersCaller(c, offers, "")
	client := crossmodel.NewApplicationOffers(apiCaller)
	filter := jujucrossmodel.ApplicationOfferFilter{
		ApplicationOffer: jujucrossmodel.ApplicationOffer{
			ApplicationName:        "service",
			ApplicationDescription: "description",
			SourceModelUUID:        fakeUUID,
		},
	}
	result, err := client.ListOffers("local", filter)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.HasLen, 1)
	expectedOffer, err := crossmodel.MakeOfferFromParams(offers[0])
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result[0], jc.DeepEquals, expectedOffer)
}

func (s *serviceDirectorySuite) TestListOffersError(c *gc.C) {
	apiCaller := listOffersCaller(c, nil, "error")
	client := crossmodel.NewApplicationOffers(apiCaller)
	filter := jujucrossmodel.ApplicationOfferFilter{
		ApplicationOffer: jujucrossmodel.ApplicationOffer{
			ApplicationName:        "service",
			ApplicationDescription: "description",
			SourceModelUUID:        fakeUUID,
		},
	}
	_, err := client.ListOffers("local", filter)
	c.Assert(err, gc.ErrorMatches, "error")
}

func (s *serviceDirectorySuite) TestListOffersNoDirectory(c *gc.C) {
	apiCaller := listOffersCaller(c, nil, "error")
	client := crossmodel.NewApplicationOffers(apiCaller)
	_, err := client.ListOffers("", jujucrossmodel.ApplicationOfferFilter{})
	c.Assert(err, gc.ErrorMatches, "application directory must be specified")
}

func addOffersCaller(c *gc.C, expectedOffers []params.AddApplicationOffer, err string) basetesting.APICallerFunc {
	return basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Check(objType, gc.Equals, "ApplicationOffers")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "AddOffers")

			args, ok := a.(params.AddApplicationOffers)
			c.Assert(ok, jc.IsTrue)
			c.Assert(args.Offers, jc.DeepEquals, expectedOffers)

			if results, ok := result.(*params.ErrorResults); ok {
				results.Results = make([]params.ErrorResult, len(expectedOffers))
				if err != "" {
					results.Results[0].Error = common.ServerError(errors.New(err))
				}
			}
			return nil
		})
}

func (s *serviceDirectorySuite) TestAddOffers(c *gc.C) {
	endpoints := []params.RemoteEndpoint{
		{
			Name:      "db",
			Role:      charm.RoleProvider,
			Interface: "mysql",
		},
	}
	offers := []params.AddApplicationOffer{
		{
			ApplicationOffer: params.ApplicationOffer{
				ApplicationURL:  "local:/u/user/servicename",
				ApplicationName: "service",
				SourceModelTag:  "model-" + fakeUUID,
				Endpoints:       endpoints,
			},
			UserTags: []string{"user-foo"},
		},
	}
	apiCaller := addOffersCaller(c, offers, "")
	client := crossmodel.NewApplicationOffers(apiCaller)
	offerToAdd, err := crossmodel.MakeOfferFromParams(offers[0].ApplicationOffer)
	c.Assert(err, jc.ErrorIsNil)
	err = client.AddOffer(offerToAdd, []string{"foo"})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceDirectorySuite) TestAddOffersError(c *gc.C) {
	endpoints := []params.RemoteEndpoint{
		{
			Name:      "db",
			Role:      charm.RoleProvider,
			Interface: "mysql",
		},
	}
	offers := []params.AddApplicationOffer{
		{
			ApplicationOffer: params.ApplicationOffer{
				ApplicationURL:  "local:/u/user/servicename",
				ApplicationName: "service",
				SourceModelTag:  "model-" + fakeUUID,
				Endpoints:       endpoints,
			},
			UserTags: []string{"user-foo"},
		},
	}
	apiCaller := addOffersCaller(c, offers, "error")
	client := crossmodel.NewApplicationOffers(apiCaller)
	offerToAdd, err := crossmodel.MakeOfferFromParams(offers[0].ApplicationOffer)
	c.Assert(err, jc.ErrorIsNil)
	err = client.AddOffer(offerToAdd, []string{"foo"})
	c.Assert(err, gc.ErrorMatches, "error")
}

func (s *serviceDirectorySuite) TestAddOffersInvalidUser(c *gc.C) {
	apiCaller := addOffersCaller(c, nil, "")
	client := crossmodel.NewApplicationOffers(apiCaller)
	err := client.AddOffer(jujucrossmodel.ApplicationOffer{}, []string{"foo/23"})
	c.Assert(err, gc.ErrorMatches, `user name "foo/23" not valid`)
}

func apiCallerWithError(c *gc.C, facadeName, apiName string) basetesting.APICallerFunc {
	return basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Check(objType, gc.Equals, facadeName)
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, apiName)

			return errors.New("facade failure")
		})
}

func (s *serviceDirectorySuite) TestServiceForURLFacadeCallError(c *gc.C) {
	client := crossmodel.NewApplicationOffers(apiCallerWithError(c, "ApplicationOffers", "ListOffers"))
	_, err := jujucrossmodel.ApplicationOfferForURL(client, "local:/u/user/servicename", "user")
	c.Assert(errors.Cause(err), gc.ErrorMatches, "facade failure")
}

func (s *serviceDirectorySuite) TestListOffersFacadeCallError(c *gc.C) {
	client := crossmodel.NewApplicationOffers(apiCallerWithError(c, "ApplicationOffers", "ListOffers"))
	_, err := client.ListOffers("local", jujucrossmodel.ApplicationOfferFilter{})
	c.Assert(errors.Cause(err), gc.ErrorMatches, "facade failure")
}

func (s *serviceDirectorySuite) TestAddOfferFacadeCallError(c *gc.C) {
	client := crossmodel.NewApplicationOffers(apiCallerWithError(c, "ApplicationOffers", "AddOffers"))
	err := client.AddOffer(jujucrossmodel.ApplicationOffer{}, nil)
	c.Assert(errors.Cause(err), gc.ErrorMatches, "facade failure")
}
