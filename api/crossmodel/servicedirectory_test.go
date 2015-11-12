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
	jujucrossmodel "github.com/juju/juju/model/crossmodel"
	"github.com/juju/juju/testing"
)

type serviceDirectoryMockSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&serviceDirectoryMockSuite{})

func serviceForURLCaller(c *gc.C, offers []params.ServiceOffer, err string) basetesting.APICallerFunc {
	return basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Check(objType, gc.Equals, "ServiceDirectory")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "ListOffers")

			args, ok := a.(params.OfferFilters)
			c.Assert(ok, jc.IsTrue)
			c.Assert(args.Filters, gc.HasLen, 1)

			filter := args.Filters[0]
			c.Check(filter.ServiceURL, gc.DeepEquals, "local:/u/user/name")
			c.Check(filter.AllowedUserTags, jc.SameContents, []string{"user-foo"})
			c.Check(filter.Endpoints, gc.HasLen, 0)
			c.Check(filter.ServiceName, gc.Equals, "")
			c.Check(filter.ServiceDescription, gc.Equals, "")
			c.Check(filter.ServiceUser, gc.Equals, "")
			c.Check(filter.SourceLabel, gc.Equals, "")

			if results, ok := result.(*params.ServiceOfferResults); ok {
				results.Offers = offers
				if err != "" {
					results.Error = common.ServerError(errors.New(err))
				}
			}
			return nil
		})
}

var fakeUUID = "df136476-12e9-11e4-8a70-b2227cce2b54"

func (s *serviceDirectoryMockSuite) TestServiceForURL(c *gc.C) {
	endpoints := []params.RemoteEndpoint{
		{
			Name:      "db",
			Role:      charm.RoleProvider,
			Interface: "mysql",
		},
	}
	offers := []params.ServiceOffer{
		{
			ServiceURL:       "local:/u/user/name",
			ServiceName:      "service",
			SourceEnvironTag: "environment-" + fakeUUID,
			Endpoints:        endpoints,
		},
	}
	apiCaller := serviceForURLCaller(c, offers, "")
	client := jujucrossmodel.ServiceDirectoryWrapper{crossmodel.NewServiceDirectory(apiCaller)}
	result, err := client.ServiceForURL("local:/u/user/name", "foo")
	c.Assert(err, jc.ErrorIsNil)
	expectedOffer, err := crossmodel.MakeOfferFromParams(offers[0])
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, expectedOffer)
}

func (s *serviceDirectoryMockSuite) TestServiceForURLError(c *gc.C) {
	apiCaller := serviceForURLCaller(c, nil, "error")
	client := jujucrossmodel.ServiceDirectoryWrapper{crossmodel.NewServiceDirectory(apiCaller)}
	_, err := client.ServiceForURL("local:/u/user/name", "foo")
	c.Assert(err, gc.ErrorMatches, "error")
}

func listOffersCaller(c *gc.C, offers []params.ServiceOffer, err string) basetesting.APICallerFunc {
	return basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Check(objType, gc.Equals, "ServiceDirectory")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "ListOffers")

			args, ok := a.(params.OfferFilters)
			c.Assert(ok, jc.IsTrue)
			c.Assert(args.Filters, gc.HasLen, 1)

			filter := args.Filters[0]
			c.Check(filter.ServiceName, gc.Equals, "service")
			c.Check(filter.ServiceDescription, gc.Equals, "description")
			c.Check(filter.SourceEnvUUIDTag, gc.Equals, "environment-"+fakeUUID)

			if results, ok := result.(*params.ServiceOfferResults); ok {
				results.Offers = offers
				if err != "" {
					results.Error = common.ServerError(errors.New(err))
				}
			}
			return nil
		})
}

func (s *serviceDirectoryMockSuite) TestListOffers(c *gc.C) {
	endpoints := []params.RemoteEndpoint{
		{
			Name:      "db",
			Role:      charm.RoleProvider,
			Interface: "mysql",
		},
	}
	offers := []params.ServiceOffer{
		{
			ServiceURL:       "local:/u/user/name",
			ServiceName:      "service",
			SourceEnvironTag: "environment-" + fakeUUID,
			Endpoints:        endpoints,
		},
	}
	apiCaller := listOffersCaller(c, offers, "")
	client := crossmodel.NewServiceDirectory(apiCaller)
	filter := jujucrossmodel.OfferFilter{
		ServiceOffer: jujucrossmodel.ServiceOffer{
			ServiceName:        "service",
			ServiceDescription: "description",
			SourceEnvUUID:      fakeUUID,
		},
	}
	result, err := client.ListOffers(filter)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.HasLen, 1)
	expectedOffer, err := crossmodel.MakeOfferFromParams(offers[0])
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result[0], jc.DeepEquals, expectedOffer)
}

func (s *serviceDirectoryMockSuite) TestListOffersError(c *gc.C) {
	apiCaller := listOffersCaller(c, nil, "error")
	client := crossmodel.NewServiceDirectory(apiCaller)
	filter := jujucrossmodel.OfferFilter{
		ServiceOffer: jujucrossmodel.ServiceOffer{
			ServiceName:        "service",
			ServiceDescription: "description",
			SourceEnvUUID:      fakeUUID,
		},
	}
	_, err := client.ListOffers(filter)
	c.Assert(err, gc.ErrorMatches, "error")
}

func addOffersCaller(c *gc.C, expectedOffers []params.AddServiceOffer, err string) basetesting.APICallerFunc {
	return basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Check(objType, gc.Equals, "ServiceDirectory")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "AddOffers")

			args, ok := a.(params.AddServiceOffers)
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

func (s *serviceDirectoryMockSuite) TestAddOffers(c *gc.C) {
	endpoints := []params.RemoteEndpoint{
		{
			Name:      "db",
			Role:      charm.RoleProvider,
			Interface: "mysql",
		},
	}
	offers := []params.AddServiceOffer{
		{
			ServiceOffer: params.ServiceOffer{
				ServiceURL:       "local:/u/user/name",
				ServiceName:      "service",
				SourceEnvironTag: "environment-" + fakeUUID,
				Endpoints:        endpoints,
			},
			UserTags: []string{"user-foo"},
		},
	}
	apiCaller := addOffersCaller(c, offers, "")
	client := crossmodel.NewServiceDirectory(apiCaller)
	offerToAdd, err := crossmodel.MakeOfferFromParams(offers[0].ServiceOffer)
	c.Assert(err, jc.ErrorIsNil)
	err = client.AddOffer(offerToAdd, []string{"foo"})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceDirectoryMockSuite) TestAddOffersError(c *gc.C) {
	endpoints := []params.RemoteEndpoint{
		{
			Name:      "db",
			Role:      charm.RoleProvider,
			Interface: "mysql",
		},
	}
	offers := []params.AddServiceOffer{
		{
			ServiceOffer: params.ServiceOffer{
				ServiceURL:       "local:/u/user/name",
				ServiceName:      "service",
				SourceEnvironTag: "environment-" + fakeUUID,
				Endpoints:        endpoints,
			},
			UserTags: []string{"user-foo"},
		},
	}
	apiCaller := addOffersCaller(c, offers, "error")
	client := crossmodel.NewServiceDirectory(apiCaller)
	offerToAdd, err := crossmodel.MakeOfferFromParams(offers[0].ServiceOffer)
	c.Assert(err, jc.ErrorIsNil)
	err = client.AddOffer(offerToAdd, []string{"foo"})
	c.Assert(err, gc.ErrorMatches, "error")
}

func apiCallerWithError(c *gc.C, apiName string) basetesting.APICallerFunc {
	return basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Check(objType, gc.Equals, "ServiceDirectory")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, apiName)

			return errors.New("facade failure")
		})
}

func (s *serviceDirectoryMockSuite) TestServiceForURLFacadeCallError(c *gc.C) {
	client := jujucrossmodel.ServiceDirectoryWrapper{crossmodel.NewServiceDirectory(apiCallerWithError(c, "ListOffers"))}
	_, err := client.ServiceForURL("local:/u/user/name", "user")
	c.Assert(errors.Cause(err), gc.ErrorMatches, "facade failure")
}

func (s *serviceDirectoryMockSuite) TestListOffersFacadeCallError(c *gc.C) {
	client := crossmodel.NewServiceDirectory(apiCallerWithError(c, "ListOffers"))
	_, err := client.ListOffers(jujucrossmodel.OfferFilter{})
	c.Assert(errors.Cause(err), gc.ErrorMatches, "facade failure")
}

func (s *serviceDirectoryMockSuite) TestAddOfferFacadeCallError(c *gc.C) {
	client := crossmodel.NewServiceDirectory(apiCallerWithError(c, "AddOffers"))
	err := client.AddOffer(jujucrossmodel.ServiceOffer{}, nil)
	c.Assert(errors.Cause(err), gc.ErrorMatches, "facade failure")
}
