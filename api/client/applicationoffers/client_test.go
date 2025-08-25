// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package applicationoffers_test

import (
	"fmt"
	stdtesting "testing"
	"time"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	basemocks "github.com/juju/juju/api/base/mocks"
	"github.com/juju/juju/api/client/applicationoffers"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	jujucrossmodel "github.com/juju/juju/core/crossmodel"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/testing"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
)

type crossmodelMockSuite struct {
}

func TestCrossmodelMockSuite(t *stdtesting.T) {
	tc.Run(t, &crossmodelMockSuite{})
}

func (s *crossmodelMockSuite) TestOffer(c *tc.C) {
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
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "Offer", args, res).SetArg(3, ress).Return(nil)
	client := applicationoffers.NewClientFromCaller(mockFacadeCaller)

	results, err := client.Offer(c.Context(), "uuid", application, []string{endPointA, endPointB}, owner, offer, desc)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.HasLen, 2)
	c.Assert(results, tc.DeepEquals,
		[]params.ErrorResult{
			{},
			{Error: apiservererrors.ServerError(errors.New(msg))},
		})
}

func (s *crossmodelMockSuite) TestOfferFacadeCallError(c *tc.C) {
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
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "Offer", args, res).Return(errors.New(msg))
	client := applicationoffers.NewClientFromCaller(mockFacadeCaller)

	results, err := client.Offer(c.Context(), "", "", nil, "fred", "", "")
	c.Assert(errors.Cause(err), tc.ErrorMatches, msg)
	c.Assert(results, tc.IsNil)
}

func (s *crossmodelMockSuite) TestList(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	offerName := "hosted-db2"
	url := fmt.Sprintf("prod/model.%s", offerName)
	endpoints := []params.RemoteEndpoint{{Name: "endPointA"}}
	relations := []jujucrossmodel.EndpointFilterTerm{{Name: "endPointA", Interface: "http"}}

	filter := jujucrossmodel.ApplicationOfferFilter{
		ModelQualifier:   "prod",
		ModelName:        "model",
		OfferName:        offerName,
		Endpoints:        relations,
		ApplicationName:  "mysql",
		AllowedConsumers: []string{"allowed"},
		ConnectedUsers:   []string{"connected"},
	}
	since := time.Now()

	args := params.OfferFilters{
		Filters: []params.OfferFilter{{
			ModelQualifier:  "prod",
			ModelName:       "model",
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

	res := new(params.QueryApplicationOffersResultsV5)
	offer := params.ApplicationOfferDetailsV5{
		OfferURL:  url,
		OfferName: offerName,
		OfferUUID: offerName + "-uuid",
		Endpoints: endpoints,
		Users: []params.OfferUserDetails{
			{UserName: "fred", DisplayName: "Fred", Access: "consume"},
		},
	}
	ress := params.QueryApplicationOffersResultsV5{
		Results: []params.ApplicationOfferAdminDetailsV5{{
			ApplicationOfferDetailsV5: offer,
			ApplicationName:           "db2-app",
			CharmURL:                  "ch:db2-5",
			Connections: []params.OfferConnection{
				{SourceModelTag: testing.ModelTag.String(), Username: "fred", RelationId: 3,
					Endpoint: "db", Status: params.EntityStatus{Status: "joined", Info: "message", Since: &since},
					IngressSubnets: []string{"10.0.0.0/8"},
				},
			},
		}},
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "ListApplicationOffers", args, res).SetArg(3, ress).Return(nil)
	client := applicationoffers.NewClientFromCaller(mockFacadeCaller)

	results, err := client.ListOffers(c.Context(), filter)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, []*jujucrossmodel.ApplicationOfferDetails{{
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

func (s *crossmodelMockSuite) TestListError(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	msg := "find failure"
	filter := jujucrossmodel.ApplicationOfferFilter{
		OfferName: "foo",
	}

	args := params.OfferFilters{
		Filters: []params.OfferFilter{{
			ModelQualifier:      filter.ModelQualifier.String(),
			ModelName:           filter.ModelName,
			OfferName:           filter.OfferName,
			ApplicationName:     filter.ApplicationName,
			Endpoints:           make([]params.EndpointFilterAttributes, len(filter.Endpoints)),
			AllowedConsumerTags: make([]string, len(filter.AllowedConsumers)),
			ConnectedUserTags:   make([]string, len(filter.ConnectedUsers)),
		}},
	}

	res := new(params.QueryApplicationOffersResultsV5)

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "ListApplicationOffers", args, res).Return(errors.New(msg))
	client := applicationoffers.NewClientFromCaller(mockFacadeCaller)

	results, err := client.ListOffers(c.Context(), filter)
	c.Assert(errors.Cause(err), tc.ErrorMatches, msg)
	c.Assert(results, tc.IsNil)
}

func (s *crossmodelMockSuite) TestShow(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	url := "prod/model.db2"

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
			{Result: &params.ApplicationOfferAdminDetailsV5{
				ApplicationOfferDetailsV5: params.ApplicationOfferDetailsV5{
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
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "ApplicationOffers", args, res).SetArg(3, ress).Return(nil)
	client := applicationoffers.NewClientFromCaller(mockFacadeCaller)

	results, err := client.ApplicationOffer(c.Context(), url)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(results, tc.DeepEquals, &jujucrossmodel.ApplicationOfferDetails{
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

func (s *crossmodelMockSuite) TestShowURLError(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	url := "prod/model.db2"
	msg := "facade failure"

	args := params.OfferURLs{OfferURLs: []string{url}, BakeryVersion: bakery.LatestVersion}

	res := new(params.ApplicationOffersResults)
	ress := params.ApplicationOffersResults{
		Results: []params.ApplicationOfferResult{
			{Error: apiservererrors.ServerError(errors.New(msg))}},
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "ApplicationOffers", args, res).SetArg(3, ress).Return(nil)
	client := applicationoffers.NewClientFromCaller(mockFacadeCaller)

	found, err := client.ApplicationOffer(c.Context(), url)

	c.Assert(errors.Cause(err), tc.ErrorMatches, msg)
	c.Assert(found, tc.IsNil)
}

func (s *crossmodelMockSuite) TestShowMultiple(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	url := "prod/model.db2"

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
			{Result: &params.ApplicationOfferAdminDetailsV5{
				ApplicationOfferDetailsV5: params.ApplicationOfferDetailsV5{
					ApplicationDescription: desc,
					Endpoints:              endpoints,
					OfferURL:               url,
					OfferName:              offerName,
				},
			}},
			{Result: &params.ApplicationOfferAdminDetailsV5{
				ApplicationOfferDetailsV5: params.ApplicationOfferDetailsV5{
					ApplicationDescription: desc,
					Endpoints:              endpoints,
					OfferURL:               url,
					OfferName:              offerName,
				},
			}}},
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "ApplicationOffers", args, res).SetArg(3, ress).Return(nil)
	client := applicationoffers.NewClientFromCaller(mockFacadeCaller)

	found, err := client.ApplicationOffer(c.Context(), url)
	c.Assert(err, tc.ErrorMatches, fmt.Sprintf(`expected to find one result for url %q but found 2`, url))
	c.Assert(found, tc.IsNil)
}

func (s *crossmodelMockSuite) TestShowFacadeCallError(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	msg := "facade failure"
	args := params.OfferURLs{OfferURLs: []string{"prod/model.db2"}, BakeryVersion: bakery.LatestVersion}

	res := new(params.ApplicationOffersResults)
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "ApplicationOffers", args, res).Return(errors.New(msg))
	client := applicationoffers.NewClientFromCaller(mockFacadeCaller)

	found, err := client.ApplicationOffer(c.Context(), "prod/model.db2")
	c.Assert(errors.Cause(err), tc.ErrorMatches, msg)
	c.Assert(found, tc.IsNil)
}

func (s *crossmodelMockSuite) TestFind(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	offerName := "hosted-db2"
	modelQualifier := coremodel.Qualifier("prod")
	modelName := "model"
	url := fmt.Sprintf("%s/%s.%s", modelQualifier, modelName, offerName)
	endpoints := []params.RemoteEndpoint{{Name: "endPointA"}}
	relations := []jujucrossmodel.EndpointFilterTerm{{Name: "endPointA", Interface: "http"}}

	filter := jujucrossmodel.ApplicationOfferFilter{
		ModelQualifier: modelQualifier,
		ModelName:      modelName,
		OfferName:      offerName,
		Endpoints:      relations,
	}

	args := params.OfferFilters{
		Filters: []params.OfferFilter{{
			ModelQualifier:  filter.ModelQualifier.String(),
			ModelName:       filter.ModelName,
			OfferName:       filter.OfferName,
			ApplicationName: filter.ApplicationName,
			Endpoints: []params.EndpointFilterAttributes{{
				Name:      "endPointA",
				Interface: "http",
			}},
		}},
	}

	res := new(params.QueryApplicationOffersResultsV5)
	offer := params.ApplicationOfferDetailsV5{
		OfferURL:  url,
		OfferName: offerName,
		Endpoints: endpoints,
		Users: []params.OfferUserDetails{
			{UserName: "fred", DisplayName: "Fred", Access: "consume"},
		},
	}
	ress := params.QueryApplicationOffersResultsV5{
		Results: []params.ApplicationOfferAdminDetailsV5{{
			ApplicationOfferDetailsV5: offer,
		}},
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "FindApplicationOffers", args, res).SetArg(3, ress).Return(nil)
	client := applicationoffers.NewClientFromCaller(mockFacadeCaller)

	results, err := client.FindApplicationOffers(c.Context(), filter)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, []*jujucrossmodel.ApplicationOfferDetails{{
		OfferURL:  url,
		OfferName: offerName,
		Endpoints: []charm.Relation{{Name: "endPointA"}},
		Users: []jujucrossmodel.OfferUserDetails{
			{UserName: "fred", DisplayName: "Fred", Access: "consume"},
		},
	}})
}

func (s *crossmodelMockSuite) TestFindNoFilter(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	client := applicationoffers.NewClientFromCaller(mockFacadeCaller)
	_, err := client.FindApplicationOffers(c.Context())
	c.Assert(err, tc.ErrorMatches, "at least one filter must be specified")
}

func (s *crossmodelMockSuite) TestFindError(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	msg := "find failure"
	filter := jujucrossmodel.ApplicationOfferFilter{
		OfferName: "foo",
	}
	args := params.OfferFilters{
		Filters: []params.OfferFilter{{
			ModelQualifier:  filter.ModelQualifier.String(),
			ModelName:       filter.ModelName,
			OfferName:       filter.OfferName,
			ApplicationName: filter.ApplicationName,
			Endpoints:       []params.EndpointFilterAttributes{},
		}},
	}

	res := new(params.QueryApplicationOffersResultsV5)
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "FindApplicationOffers", args, res).Return(errors.New(msg))
	client := applicationoffers.NewClientFromCaller(mockFacadeCaller)

	_, err := client.FindApplicationOffers(c.Context(), filter)
	c.Assert(errors.Cause(err), tc.ErrorMatches, msg)
}

func (s *crossmodelMockSuite) TestGetConsumeDetails(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	offer := params.ApplicationOfferDetailsV5{
		SourceModelTag:         "source model",
		OfferName:              "an offer",
		OfferURL:               "offer url",
		ApplicationDescription: "description",
		Endpoints:              []params.RemoteEndpoint{{Name: "endpoint"}},
	}
	controllerInfo := &params.ExternalControllerInfo{
		Addrs: []string{"1.2.3.4"},
	}
	mac, err := jujutesting.NewMacaroon("id")
	c.Assert(err, tc.ErrorIsNil)

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
					ControllerInfo: controllerInfo,
				},
			},
		},
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "GetConsumeDetails", args, res).SetArg(3, ress).Return(nil)
	client := applicationoffers.NewClientFromCaller(mockFacadeCaller)

	details, err := client.GetConsumeDetails(c.Context(), "me/prod.app")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(details, tc.DeepEquals, params.ConsumeOfferDetails{
		Offer:          &offer,
		Macaroon:       mac,
		ControllerInfo: controllerInfo,
	})
}

func (s *crossmodelMockSuite) TestGetConsumeDetailsBadURL(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	client := applicationoffers.NewClientFromCaller(mockFacadeCaller)
	_, err := client.GetConsumeDetails(c.Context(), "badurl")
	c.Assert(err, tc.ErrorMatches, "offer URL is missing the name")
}

func (s *crossmodelMockSuite) TestDestroyOffers(c *tc.C) {
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
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "DestroyOffers", args, res).SetArg(3, ress).Return(nil)
	client := applicationoffers.NewClientFromCaller(mockFacadeCaller)

	err := client.DestroyOffers(c.Context(), true, "me/prod.app")
	c.Assert(err, tc.ErrorMatches, "fail")
}
