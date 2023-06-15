// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package applicationoffers_test

import (
	"fmt"
	"time"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/golang/mock/gomock"
	"github.com/juju/charm/v11"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	basemocks "github.com/juju/juju/api/base/mocks"
	"github.com/juju/juju/api/client/applicationoffers"
	apitesting "github.com/juju/juju/api/testing"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	jujucrossmodel "github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/testing"
)

type crossmodelMockSuite struct {
}

var _ = gc.Suite(&crossmodelMockSuite{})

func (s *crossmodelMockSuite) TestOffer(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	application := "shared"
	endPointA := "endPointA"
	endPointB := "endPointB"
	offer := "offer"
	desc := "desc"
	owner := "fred"

	msg := "fail"

	args := params.AddApplicationOffers{
		Offers: []params.AddApplicationOffer{
			{
				ModelTag:               names.NewModelTag("uuid").String(),
				ApplicationName:        application,
				ApplicationDescription: desc,
				Endpoints:              map[string]string{endPointA: endPointA, endPointB: endPointB},
				OfferName:              offer,
				OwnerTag:               names.NewUserTag(owner).String(),
			},
		},
	}

	res := new(params.ErrorResults)
	ress := params.ErrorResults{Results: []params.ErrorResult{{}, {Error: apiservererrors.ServerError(errors.New(msg))}}}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("Offer", args, res).SetArg(2, ress).Return(nil)
	client := applicationoffers.NewClientFromCaller(mockFacadeCaller)

	results, err := client.Offer("uuid", application, []string{endPointA, endPointB}, owner, offer, desc)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, 2)
	c.Assert(results, jc.DeepEquals,
		[]params.ErrorResult{
			{},
			{Error: apiservererrors.ServerError(errors.New(msg))},
		})
}

func (s *crossmodelMockSuite) TestOfferFacadeCallError(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	msg := "facade failure"
	args := params.AddApplicationOffers{
		Offers: []params.AddApplicationOffer{
			{
				ModelTag:               names.NewModelTag("").String(),
				ApplicationName:        "",
				ApplicationDescription: "",
				Endpoints:              map[string]string{},
				OfferName:              "",
				OwnerTag:               names.NewUserTag("fred").String(),
			},
		},
	}

	res := new(params.ErrorResults)
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("Offer", args, res).Return(errors.New(msg))
	client := applicationoffers.NewClientFromCaller(mockFacadeCaller)

	results, err := client.Offer("", "", nil, "fred", "", "")
	c.Assert(errors.Cause(err), gc.ErrorMatches, msg)
	c.Assert(results, gc.IsNil)
}

func (s *crossmodelMockSuite) TestList(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

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
	since := time.Now()

	args := params.OfferFilters{
		Filters: []params.OfferFilter{{
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
		}},
	}

	res := new(params.QueryApplicationOffersResults)
	offer := params.ApplicationOfferDetails{
		OfferURL:  url,
		OfferName: offerName,
		OfferUUID: offerName + "-uuid",
		Endpoints: endpoints,
		Users: []params.OfferUserDetails{
			{UserName: "fred", DisplayName: "Fred", Access: "consume"},
		},
	}
	ress := params.QueryApplicationOffersResults{
		Results: []params.ApplicationOfferAdminDetails{{
			ApplicationOfferDetails: offer,
			ApplicationName:         "db2-app",
			CharmURL:                "ch:db2-5",
			Connections: []params.OfferConnection{
				{SourceModelTag: testing.ModelTag.String(), Username: "fred", RelationId: 3,
					Endpoint: "db", Status: params.EntityStatus{Status: "joined", Info: "message", Since: &since},
					IngressSubnets: []string{"10.0.0.0/8"},
				},
			},
		}},
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("ListApplicationOffers", args, res).SetArg(2, ress).Return(nil)
	client := applicationoffers.NewClientFromCaller(mockFacadeCaller)

	results, err := client.ListOffers(filter)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, []*jujucrossmodel.ApplicationOfferDetails{{
		OfferURL:        url,
		OfferName:       offerName,
		Endpoints:       []charm.Relation{{Name: "endPointA"}},
		ApplicationName: "db2-app",
		CharmURL:        "ch:db2-5",
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
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	msg := "find failure"
	filter := jujucrossmodel.ApplicationOfferFilter{
		OfferName: "foo",
	}

	args := params.OfferFilters{
		Filters: []params.OfferFilter{{
			OwnerName:           filter.OwnerName,
			ModelName:           filter.ModelName,
			OfferName:           filter.OfferName,
			ApplicationName:     filter.ApplicationName,
			Endpoints:           make([]params.EndpointFilterAttributes, len(filter.Endpoints)),
			AllowedConsumerTags: make([]string, len(filter.AllowedConsumers)),
			ConnectedUserTags:   make([]string, len(filter.ConnectedUsers)),
		}},
	}

	res := new(params.QueryApplicationOffersResults)

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("ListApplicationOffers", args, res).Return(errors.New(msg))
	client := applicationoffers.NewClientFromCaller(mockFacadeCaller)

	results, err := client.ListOffers(filter)
	c.Assert(errors.Cause(err), gc.ErrorMatches, msg)
	c.Assert(results, gc.IsNil)
}

func (s *crossmodelMockSuite) TestShow(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	url := "fred/model.db2"

	desc := "IBM DB2 Express Server Edition is an entry level database system"
	endpoints := []params.RemoteEndpoint{
		{Name: "db2", Interface: "db2", Role: charm.RoleProvider},
		{Name: "log", Interface: "http", Role: charm.RoleRequirer},
	}
	offerName := "hosted-db2"
	access := "consume"
	since := time.Now()

	args := params.OfferURLs{OfferURLs: []string{url}, BakeryVersion: bakery.LatestVersion}

	res := new(params.ApplicationOffersResults)
	ress := params.ApplicationOffersResults{
		Results: []params.ApplicationOfferResult{
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
				CharmURL:        "ch:db2-5",
				Connections: []params.OfferConnection{
					{SourceModelTag: testing.ModelTag.String(), Username: "fred", RelationId: 3,
						Endpoint: "db", Status: params.EntityStatus{Status: "joined", Info: "message", Since: &since},
						IngressSubnets: []string{"10.0.0.0/8"},
					},
				},
			}},
		},
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("ApplicationOffers", args, res).SetArg(2, ress).Return(nil)
	client := applicationoffers.NewClientFromCaller(mockFacadeCaller)

	results, err := client.ApplicationOffer(url)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(results, jc.DeepEquals, &jujucrossmodel.ApplicationOfferDetails{
		OfferURL:  url,
		OfferName: offerName,
		Endpoints: []charm.Relation{
			{Name: "db2", Role: "provider", Interface: "db2", Optional: false, Limit: 0, Scope: ""},
			{Name: "log", Role: "requirer", Interface: "http", Optional: false, Limit: 0, Scope: ""}},
		ApplicationName:        "db2-app",
		ApplicationDescription: "IBM DB2 Express Server Edition is an entry level database system",
		CharmURL:               "ch:db2-5",
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
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	url := "fred/model.db2"
	msg := "facade failure"

	args := params.OfferURLs{OfferURLs: []string{url}, BakeryVersion: bakery.LatestVersion}

	res := new(params.ApplicationOffersResults)
	ress := params.ApplicationOffersResults{
		Results: []params.ApplicationOfferResult{
			{Error: apiservererrors.ServerError(errors.New(msg))}},
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("ApplicationOffers", args, res).SetArg(2, ress).Return(nil)
	client := applicationoffers.NewClientFromCaller(mockFacadeCaller)

	found, err := client.ApplicationOffer(url)

	c.Assert(errors.Cause(err), gc.ErrorMatches, msg)
	c.Assert(found, gc.IsNil)
}

func (s *crossmodelMockSuite) TestShowMultiple(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	url := "fred/model.db2"

	desc := "IBM DB2 Express Server Edition is an entry level database system"
	endpoints := []params.RemoteEndpoint{
		{Name: "db2", Interface: "db2", Role: charm.RoleProvider},
		{Name: "log", Interface: "http", Role: charm.RoleRequirer},
	}
	offerName := "hosted-db2"
	args := params.OfferURLs{OfferURLs: []string{url}, BakeryVersion: bakery.LatestVersion}

	res := new(params.ApplicationOffersResults)
	ress := params.ApplicationOffersResults{
		Results: []params.ApplicationOfferResult{
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
			}}},
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("ApplicationOffers", args, res).SetArg(2, ress).Return(nil)
	client := applicationoffers.NewClientFromCaller(mockFacadeCaller)

	found, err := client.ApplicationOffer(url)
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf(`expected to find one result for url %q but found 2`, url))
	c.Assert(found, gc.IsNil)
}

func (s *crossmodelMockSuite) TestShowFacadeCallError(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	msg := "facade failure"
	args := params.OfferURLs{OfferURLs: []string{"fred/model.db2"}, BakeryVersion: bakery.LatestVersion}

	res := new(params.ApplicationOffersResults)
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("ApplicationOffers", args, res).Return(errors.New(msg))
	client := applicationoffers.NewClientFromCaller(mockFacadeCaller)

	found, err := client.ApplicationOffer("fred/model.db2")
	c.Assert(errors.Cause(err), gc.ErrorMatches, msg)
	c.Assert(found, gc.IsNil)
}

func (s *crossmodelMockSuite) TestFind(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

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

	args := params.OfferFilters{
		Filters: []params.OfferFilter{{
			OwnerName:       filter.OwnerName,
			ModelName:       filter.ModelName,
			OfferName:       filter.OfferName,
			ApplicationName: filter.ApplicationName,
			Endpoints: []params.EndpointFilterAttributes{{
				Name:      "endPointA",
				Interface: "http",
			}},
		}},
	}

	res := new(params.QueryApplicationOffersResults)
	offer := params.ApplicationOfferDetails{
		OfferURL:  url,
		OfferName: offerName,
		Endpoints: endpoints,
		Users: []params.OfferUserDetails{
			{UserName: "fred", DisplayName: "Fred", Access: "consume"},
		},
	}
	ress := params.QueryApplicationOffersResults{
		Results: []params.ApplicationOfferAdminDetails{{
			ApplicationOfferDetails: offer,
		}},
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("FindApplicationOffers", args, res).SetArg(2, ress).Return(nil)
	client := applicationoffers.NewClientFromCaller(mockFacadeCaller)

	results, err := client.FindApplicationOffers(filter)
	c.Assert(err, jc.ErrorIsNil)
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
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	client := applicationoffers.NewClientFromCaller(mockFacadeCaller)
	_, err := client.FindApplicationOffers()
	c.Assert(err, gc.ErrorMatches, "at least one filter must be specified")
}

func (s *crossmodelMockSuite) TestFindError(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	msg := "find failure"
	filter := jujucrossmodel.ApplicationOfferFilter{
		OfferName: "foo",
	}
	args := params.OfferFilters{
		Filters: []params.OfferFilter{{
			OwnerName:       filter.OwnerName,
			ModelName:       filter.ModelName,
			OfferName:       filter.OfferName,
			ApplicationName: filter.ApplicationName,
			Endpoints:       []params.EndpointFilterAttributes{},
		}},
	}

	res := new(params.QueryApplicationOffersResults)
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("FindApplicationOffers", args, res).Return(errors.New(msg))
	client := applicationoffers.NewClientFromCaller(mockFacadeCaller)

	_, err := client.FindApplicationOffers(filter)
	c.Assert(errors.Cause(err), gc.ErrorMatches, msg)
}

func (s *crossmodelMockSuite) TestGetConsumeDetails(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

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

	args := params.ConsumeOfferDetailsArg{
		OfferURLs: params.OfferURLs{
			OfferURLs:     []string{"me/prod.app"},
			BakeryVersion: 3,
		},
		UserTag: "",
	}

	res := new(params.ConsumeOfferDetailsResults)
	ress := params.ConsumeOfferDetailsResults{
		Results: []params.ConsumeOfferDetailsResult{
			{
				ConsumeOfferDetails: params.ConsumeOfferDetails{
					Offer:          &offer,
					Macaroon:       mac,
					AuthToken:      "auth-token",
					ControllerInfo: controllerInfo,
				},
			},
		},
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("GetConsumeDetails", args, res).SetArg(2, ress).Return(nil)
	client := applicationoffers.NewClientFromCaller(mockFacadeCaller)

	details, err := client.GetConsumeDetails("me/prod.app")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(details, jc.DeepEquals, params.ConsumeOfferDetails{
		Offer:          &offer,
		Macaroon:       mac,
		AuthToken:      "auth-token",
		ControllerInfo: controllerInfo,
	})
}

func (s *crossmodelMockSuite) TestGetConsumeDetailsBadURL(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	client := applicationoffers.NewClientFromCaller(mockFacadeCaller)
	_, err := client.GetConsumeDetails("badurl")
	c.Assert(err, gc.ErrorMatches, "application offer URL is missing application")
}

func (s *crossmodelMockSuite) TestDestroyOffers(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.DestroyApplicationOffers{
		Force:     true,
		OfferURLs: []string{"me/prod.app"},
	}

	res := new(params.ErrorResults)
	ress := params.ErrorResults{
		Results: []params.ErrorResult{{
			Error: &params.Error{Message: "fail"},
		}},
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("DestroyOffers", args, res).SetArg(2, ress).Return(nil)
	client := applicationoffers.NewClientFromCaller(mockFacadeCaller)

	err := client.DestroyOffers(true, "me/prod.app")
	c.Assert(err, gc.ErrorMatches, "fail")
}
