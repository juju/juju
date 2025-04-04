// // Copyright 2015 Canonical Ltd.
// // Licensed under the AGPLv3, see LICENCE file for details.
package applicationoffers_test

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/checkers"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common/crossmodel"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facades/client/applicationoffers"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	jujucrossmodel "github.com/juju/juju/core/crossmodel"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/permission"
	coreuser "github.com/juju/juju/core/user"
	usertesting "github.com/juju/juju/core/user/testing"
	applicationcharm "github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/domain/relation"
	"github.com/juju/juju/internal/charm"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/uuid"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type applicationOffersSuite struct {
	baseSuite
	api *applicationoffers.OffersAPIv5
}

var _ = gc.Suite(&applicationOffersSuite{})

func (s *applicationOffersSuite) SetUpTest(c *gc.C) {
	s.baseSuite.SetUpTest(c)
	s.applicationOffers = &stubApplicationOffers{
		// Ensure that calls to "Offer" made by the test suite call
		// AddOffer by default.
		applicationOffer: func(string) (*jujucrossmodel.ApplicationOffer, error) {
			return nil, errors.NotFoundf("offer")
		},
	}

	var err error
	s.bakery = &mockBakeryService{caveats: make(map[string][]checkers.Caveat)}
	thirdPartyKey := bakery.MustGenerateKey()
	s.authContext, err = crossmodel.NewAuthContext(
		s.mockState, nil, testing.ModelTag, thirdPartyKey,
		crossmodel.NewOfferBakeryForTest(s.bakery, clock.WallClock),
	)
	c.Assert(err, jc.ErrorIsNil)
}

// Creates the API to use in testing.
// Call baseSuite.setupMocks before this.
func (s *applicationOffersSuite) setupAPI(c *gc.C) {
	getApplicationOffers := func(interface{}) jujucrossmodel.ApplicationOffers {
		return s.applicationOffers
	}
	api, err := applicationoffers.CreateOffersAPI(
		getApplicationOffers, getFakeControllerInfo,
		s.mockState, s.mockStatePool, s.mockAccessService,
		s.mockModelDomainServicesGetter,
		s.authorizer, s.authContext,
		c.MkDir(), loggertesting.WrapCheckLog(c),
		uuid.MustNewUUID().String(),
		s.mockModelService,
	)
	c.Assert(err, jc.ErrorIsNil)
	s.api = api
}

func (s *applicationOffersSuite) assertOffer(c *gc.C, expectedErr error) {
	applicationName := "test"
	s.addApplication(c, applicationName)
	one := params.AddApplicationOffer{
		ModelTag:        names.NewModelTag(s.mockState.modelUUID).String(),
		OfferName:       "offer-test",
		ApplicationName: applicationName,
		Endpoints:       map[string]string{"db": "db"},
		OwnerTag:        "user-fred",
	}
	all := params.AddApplicationOffers{Offers: []params.AddApplicationOffer{one}}
	s.applicationOffers.addOffer = func(offer jujucrossmodel.AddApplicationOfferArgs) (*jujucrossmodel.ApplicationOffer, error) {
		c.Assert(offer.OfferName, gc.Equals, one.OfferName)
		c.Assert(offer.ApplicationName, gc.Equals, one.ApplicationName)
		c.Assert(offer.ApplicationDescription, gc.Equals, "A pretty popular blog engine")
		c.Assert(offer.Owner, gc.Equals, "fred")
		c.Assert(offer.HasRead, gc.DeepEquals, []string{"everyone@external"})
		return &jujucrossmodel.ApplicationOffer{}, nil
	}
	s.mockState.applications = map[string]crossmodel.Application{
		applicationName: &mockApplication{bindings: map[string]string{"db": "myspace"}},
	}

	if expectedErr == nil {
		s.mockModelDomainServicesGetter.EXPECT().DomainServicesForModel(gomock.Any(), coremodel.UUID(s.mockState.modelUUID)).Return(s.mockModelDomainServices, nil)
		s.mockModelDomainServices.EXPECT().Application().Return(s.mockApplicationService)

		locator := applicationcharm.CharmLocator{
			Name: "wordpresssss",
		}
		s.mockApplicationService.EXPECT().GetCharmLocatorByApplicationName(gomock.Any(), applicationName).Return(locator, nil)
		s.mockApplicationService.EXPECT().GetCharmMetadataDescription(gomock.Any(), locator).Return("A pretty popular blog engine", nil)
		// Expect the creator getting admin access on the offer.
		s.mockAccessService.EXPECT().CreatePermission(gomock.Any(), permission.UserAccessSpec{
			AccessSpec: offerAccessSpec("", permission.AdminAccess),
			User:       usertesting.GenNewName(c, "fred"),
		})
		// Expect everyone@external getting read access. everyone@exteral gets
		// read access on all offers.
		s.mockAccessService.EXPECT().CreatePermission(gomock.Any(), permission.UserAccessSpec{
			AccessSpec: offerAccessSpec("", permission.ReadAccess),
			User:       usertesting.GenNewName(c, "everyone@external"),
		})
	}

	errs, err := s.api.Offer(context.Background(), all)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errs.Results, gc.HasLen, len(all.Offers))
	if expectedErr != nil {
		c.Assert(errs.Results[0].Error, gc.ErrorMatches, expectedErr.Error())
		return
	}
	c.Assert(errs.Results[0].Error, gc.IsNil)
	s.applicationOffers.CheckCallNames(c, offerCall, addOffersBackendCall)
}

func (s *applicationOffersSuite) TestOffer(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.setupAPI(c)

	s.authorizer.Tag = names.NewUserTag("admin")
	s.assertOffer(c, nil)
}

func (s *applicationOffersSuite) TestAddOfferUpdatesExistingOffer(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.setupAPI(c)

	s.authorizer.Tag = names.NewUserTag("admin")
	applicationName := "test"
	s.addApplication(c, applicationName)
	one := params.AddApplicationOffer{
		ModelTag:        names.NewModelTag(s.mockState.modelUUID).String(),
		OfferName:       "offer-test",
		ApplicationName: applicationName,
		Endpoints:       map[string]string{"db": "db"},
	}
	all := params.AddApplicationOffers{Offers: []params.AddApplicationOffer{one}}
	s.applicationOffers.applicationOffer = func(name string) (*jujucrossmodel.ApplicationOffer, error) {
		c.Assert(name, gc.Equals, one.OfferName)
		return &jujucrossmodel.ApplicationOffer{}, nil
	}
	s.applicationOffers.addOffer = func(offer jujucrossmodel.AddApplicationOfferArgs) (*jujucrossmodel.ApplicationOffer, error) {
		return nil, errors.BadRequestf("unexpected call to AddOffer; expected a call to UpdateOffer instead")
	}
	s.applicationOffers.updateOffer = func(offer jujucrossmodel.AddApplicationOfferArgs) (*jujucrossmodel.ApplicationOffer, error) {
		c.Assert(offer.OfferName, gc.Equals, one.OfferName)
		c.Assert(offer.ApplicationName, gc.Equals, one.ApplicationName)
		c.Assert(offer.ApplicationDescription, gc.Equals, "A pretty popular blog engine")
		c.Assert(offer.Owner, gc.Equals, "admin")
		c.Assert(offer.HasRead, gc.DeepEquals, []string{"everyone@external"})
		return &jujucrossmodel.ApplicationOffer{}, nil
	}
	s.mockState.applications = map[string]crossmodel.Application{
		applicationName: &mockApplication{bindings: map[string]string{"db": "myspace"}},
	}

	modelUUID := coremodel.UUID(s.mockState.modelUUID)
	s.mockModelDomainServicesGetter.EXPECT().DomainServicesForModel(gomock.Any(), modelUUID).Return(s.mockModelDomainServices, nil)
	s.mockModelDomainServices.EXPECT().Application().Return(s.mockApplicationService)

	locator := applicationcharm.CharmLocator{
		Name: "wordpresssss",
	}
	s.mockApplicationService.EXPECT().GetCharmLocatorByApplicationName(gomock.Any(), applicationName).Return(locator, nil)
	s.mockApplicationService.EXPECT().GetCharmMetadataDescription(gomock.Any(), locator).Return("A pretty popular blog engine", nil)

	errs, err := s.api.Offer(context.Background(), all)

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errs.Results, gc.HasLen, len(all.Offers))
	c.Assert(errs.Results[0].Error, gc.IsNil)
	s.applicationOffers.CheckCallNames(c, offerCall, updateOfferBackendCall)
}

func (s *applicationOffersSuite) TestOfferPermission(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.setupAPI(c)

	s.authorizer.Tag = names.NewUserTag("mary")
	s.assertOffer(c, apiservererrors.ErrPerm)
}

func (s *applicationOffersSuite) TestOfferSomeFail(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.setupAPI(c)

	s.authorizer.Tag = names.NewUserTag("admin")
	one := params.AddApplicationOffer{
		ModelTag:        names.NewModelTag(s.mockState.modelUUID).String(),
		OfferName:       "offer-one",
		ApplicationName: "one",
		Endpoints:       map[string]string{"db": "db"},
	}
	two := params.AddApplicationOffer{
		ModelTag:        names.NewModelTag(s.mockState.modelUUID).String(),
		OfferName:       "offer-two",
		ApplicationName: "two",
		Endpoints:       map[string]string{"db": "db"},
	}
	all := params.AddApplicationOffers{Offers: []params.AddApplicationOffer{one, two}}
	s.applicationOffers.addOffer = func(offer jujucrossmodel.AddApplicationOfferArgs) (*jujucrossmodel.ApplicationOffer, error) {
		if offer.ApplicationName == "paramsfail" {
			return nil, errors.New("params fail")
		}
		return &jujucrossmodel.ApplicationOffer{}, nil
	}

	_, err := s.api.Offer(context.Background(), all)
	c.Assert(err, gc.ErrorMatches, `expected exactly one offer, got 2`)
	s.applicationOffers.CheckCallNames(c)
}

func (s *applicationOffersSuite) TestOfferError(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.setupAPI(c)

	s.authorizer.Tag = names.NewUserTag("admin")
	applicationName := "test"
	s.addApplication(c, applicationName)
	one := params.AddApplicationOffer{
		ModelTag:        names.NewModelTag(s.mockState.modelUUID).String(),
		OfferName:       "offer-test",
		ApplicationName: applicationName,
		Endpoints:       map[string]string{"db": "db"},
	}
	all := params.AddApplicationOffers{Offers: []params.AddApplicationOffer{one}}

	msg := "fail"

	s.applicationOffers.addOffer = func(offer jujucrossmodel.AddApplicationOfferArgs) (*jujucrossmodel.ApplicationOffer, error) {
		return nil, errors.New(msg)
	}
	s.mockState.applications = map[string]crossmodel.Application{
		applicationName: &mockApplication{bindings: map[string]string{"db": "myspace"}},
	}

	modelUUID := coremodel.UUID(s.mockState.modelUUID)
	s.mockModelDomainServicesGetter.EXPECT().DomainServicesForModel(gomock.Any(), modelUUID).Return(s.mockModelDomainServices, nil)
	s.mockModelDomainServices.EXPECT().Application().Return(s.mockApplicationService)

	locator := applicationcharm.CharmLocator{
		Name: "wordpresssss",
	}
	s.mockApplicationService.EXPECT().GetCharmLocatorByApplicationName(gomock.Any(), applicationName).Return(locator, nil)
	s.mockApplicationService.EXPECT().GetCharmMetadataDescription(gomock.Any(), locator).Return("A pretty popular blog engine", nil)

	errs, err := s.api.Offer(context.Background(), all)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errs.Results, gc.HasLen, len(all.Offers))
	c.Assert(errs.Results[0].Error, gc.ErrorMatches, fmt.Sprintf(".*%v.*", msg))
	s.applicationOffers.CheckCallNames(c, offerCall, addOffersBackendCall)
}

func (s *applicationOffersSuite) TestOfferErrorApplicationError(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.setupAPI(c)

	s.authorizer.Tag = names.NewUserTag("admin")
	applicationName := "test"
	s.addApplication(c, applicationName)
	one := params.AddApplicationOffer{
		ModelTag:        names.NewModelTag(s.mockState.modelUUID).String(),
		OfferName:       "offer-test",
		ApplicationName: applicationName,
		Endpoints:       map[string]string{"db": "db"},
	}
	all := params.AddApplicationOffers{Offers: []params.AddApplicationOffer{one}}

	s.applicationOffers.addOffer = func(offer jujucrossmodel.AddApplicationOfferArgs) (*jujucrossmodel.ApplicationOffer, error) {
		return &jujucrossmodel.ApplicationOffer{}, nil
	}
	s.mockState.applications = map[string]crossmodel.Application{
		applicationName: &mockApplication{bindings: map[string]string{"db": "myspace"}},
	}

	modelUUID := coremodel.UUID(s.mockState.modelUUID)
	s.mockModelDomainServicesGetter.EXPECT().DomainServicesForModel(gomock.Any(), modelUUID).Return(s.mockModelDomainServices, nil)
	s.mockModelDomainServices.EXPECT().Application().Return(s.mockApplicationService)

	locator := applicationcharm.CharmLocator{
		Name: "wordpresssss",
	}
	s.mockApplicationService.EXPECT().GetCharmLocatorByApplicationName(gomock.Any(), applicationName).Return(locator, applicationerrors.ApplicationNotFound)

	errs, err := s.api.Offer(context.Background(), all)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errs.Results, gc.HasLen, len(all.Offers))
	c.Assert(errs.Results[0].Error, gc.ErrorMatches, `getting offered application "test" not found`)
	s.applicationOffers.CheckCallNames(c)
}

func (s *applicationOffersSuite) TestOfferErrorApplicationCharmError(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.setupAPI(c)

	s.authorizer.Tag = names.NewUserTag("admin")
	applicationName := "test"
	s.addApplication(c, applicationName)
	one := params.AddApplicationOffer{
		ModelTag:        names.NewModelTag(s.mockState.modelUUID).String(),
		OfferName:       "offer-test",
		ApplicationName: applicationName,
		Endpoints:       map[string]string{"db": "db"},
	}
	all := params.AddApplicationOffers{Offers: []params.AddApplicationOffer{one}}

	s.applicationOffers.addOffer = func(offer jujucrossmodel.AddApplicationOfferArgs) (*jujucrossmodel.ApplicationOffer, error) {
		return &jujucrossmodel.ApplicationOffer{}, nil
	}
	s.mockState.applications = map[string]crossmodel.Application{
		applicationName: &mockApplication{bindings: map[string]string{"db": "myspace"}},
	}

	modelUUID := coremodel.UUID(s.mockState.modelUUID)
	s.mockModelDomainServicesGetter.EXPECT().DomainServicesForModel(gomock.Any(), modelUUID).Return(s.mockModelDomainServices, nil)
	s.mockModelDomainServices.EXPECT().Application().Return(s.mockApplicationService)

	locator := applicationcharm.CharmLocator{
		Name: "wordpresssss",
	}
	s.mockApplicationService.EXPECT().GetCharmLocatorByApplicationName(gomock.Any(), applicationName).Return(locator, nil)
	s.mockApplicationService.EXPECT().GetCharmMetadataDescription(gomock.Any(), locator).Return("", applicationerrors.CharmNotFound)

	errs, err := s.api.Offer(context.Background(), all)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errs.Results, gc.HasLen, len(all.Offers))
	c.Assert(errs.Results[0].Error, gc.ErrorMatches, `getting offered application "test" charm not found`)
	s.applicationOffers.CheckCallNames(c)
}

func (s *applicationOffersSuite) assertList(c *gc.C, offerUUID string, expectedCIDRS []string) {
	filter := params.OfferFilters{
		Filters: []params.OfferFilter{
			{
				OwnerName:       "fred@external",
				ModelName:       "prod",
				OfferName:       "hosted-db2",
				ApplicationName: "test",
			},
		},
	}

	// Since user is admin, get everyone else's access on the offer.
	s.mockAccessService.EXPECT().ReadAllUserAccessForTarget(gomock.Any(), permission.ID{
		ObjectType: permission.Offer,
		Key:        offerUUID,
	}).Return([]permission.UserAccess{{
		UserName:    usertesting.GenNewName(c, "mary"),
		DisplayName: "mary",
		Access:      permission.ConsumeAccess,
	}}, nil)

	// Get the admin user from the database to retrieve their display name.
	s.mockAccessService.EXPECT().GetUserByName(gomock.Any(), coreuser.AdminUserName).Return(coreuser.User{
		DisplayName: "admin",
	}, nil)

	found, err := s.api.ListApplicationOffers(context.Background(), filter)
	c.Assert(err, jc.ErrorIsNil)

	expectedOfferDetails := []params.ApplicationOfferAdminDetailsV5{
		{
			ApplicationOfferDetailsV5: params.ApplicationOfferDetailsV5{
				SourceModelTag:         names.NewModelTag(s.mockState.modelUUID).String(),
				ApplicationDescription: "description",
				OfferName:              "hosted-db2",
				OfferUUID:              offerUUID,
				OfferURL:               "fred@external/prod.hosted-db2",
				Endpoints:              []params.RemoteEndpoint{{Name: "db"}},
				Users: []params.OfferUserDetails{
					{UserName: "admin", DisplayName: "admin", Access: "admin"},
					{UserName: "mary", DisplayName: "mary", Access: "consume"},
				},
			},
			ApplicationName: "test",
			CharmURL:        "ch:db2-2",
			Connections: []params.OfferConnection{{
				SourceModelTag: names.NewModelTag(s.mockState.modelUUID).String(),
				RelationId:     1,
				Endpoint:       "db",
				Username:       "fred@external",
				Status:         params.EntityStatus{Status: "joined"},
				IngressSubnets: expectedCIDRS,
			}},
		},
	}
	c.Assert(found, jc.DeepEquals, params.QueryApplicationOffersResultsV5{
		Results: expectedOfferDetails,
	})
	s.applicationOffers.CheckCallNames(c, listOffersBackendCall)
}

func (s *applicationOffersSuite) TestList(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.setupAPI(c)

	s.authorizer.Tag = names.NewUserTag("admin")
	offerUUID := s.setupOffers(c, "test", false)

	s.assertList(c, offerUUID, []string{"192.168.1.0/32", "10.0.0.0/8"})
}

func (s *applicationOffersSuite) TestListCAAS(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.setupAPI(c)

	s.authorizer.Tag = names.NewUserTag("admin")
	offerUUID := s.setupOffers(c, "test", false)
	s.mockState.model.modelType = state.ModelTypeCAAS
	s.assertList(c, offerUUID, []string{"192.168.1.0/32", "10.0.0.0/8"})
}

func (s *applicationOffersSuite) TestListNoRelationNetworks(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.setupAPI(c)

	s.authorizer.Tag = names.NewUserTag("admin")
	s.mockState.relationNetworks = nil
	offerUUID := s.setupOffers(c, "test", false)
	s.assertList(c, offerUUID, nil)
}

func (s *applicationOffersSuite) TestListPermission(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.setupAPI(c)

	_ = s.setupOffers(c, "test", false)
	filter := params.OfferFilters{
		Filters: []params.OfferFilter{
			{
				OwnerName:       "fred@external",
				ModelName:       "prod",
				OfferName:       "hosted-db2",
				ApplicationName: "test",
			},
		},
	}
	_, err := s.api.ListApplicationOffers(context.Background(), filter)
	c.Assert(errors.Cause(err), gc.ErrorMatches, apiservererrors.ErrPerm.Error())
}

func (s *applicationOffersSuite) TestListError(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.setupAPI(c)

	s.setupOffers(c, "test", false)
	s.authorizer.Tag = names.NewUserTag("admin")
	filter := params.OfferFilters{
		Filters: []params.OfferFilter{
			{
				OwnerName:       "fred@external",
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

	_, err := s.api.ListApplicationOffers(context.Background(), filter)
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf(".*%v.*", msg))
	s.applicationOffers.CheckCallNames(c, listOffersBackendCall)
}

func (s *applicationOffersSuite) TestListFilterRequiresModel(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.setupAPI(c)

	s.setupOffers(c, "test", false)
	filter := params.OfferFilters{
		Filters: []params.OfferFilter{
			{
				OfferName:       "hosted-db2",
				ApplicationName: "test",
			},
		},
	}
	_, err := s.api.ListApplicationOffers(context.Background(), filter)
	c.Assert(err, gc.ErrorMatches, "application offer filter must specify a model name")
}

func (s *applicationOffersSuite) TestListRequiresFilter(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.setupAPI(c)

	s.setupOffers(c, "test", false)
	_, err := s.api.ListApplicationOffers(context.Background(), params.OfferFilters{})
	c.Assert(err, gc.ErrorMatches, "at least one offer filter is required")
}

func (s *applicationOffersSuite) assertShow(c *gc.C, url, offerUUID string, expected []params.ApplicationOfferResult) {
	s.setupOffersForUUID(c, offerUUID, "", false)

	filter := params.OfferURLs{OfferURLs: []string{url}, BakeryVersion: bakery.LatestVersion}

	found, err := s.api.ApplicationOffers(context.Background(), filter)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.Results, jc.DeepEquals, expected)
	s.applicationOffers.CheckCallNames(c, listOffersBackendCall)
	if len(expected) > 0 {
		return
	}
}

func (s *applicationOffersSuite) TestShow(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.setupAPI(c)

	offerUUID := uuid.MustNewUUID().String()
	expected := []params.ApplicationOfferResult{{
		Result: &params.ApplicationOfferAdminDetailsV5{
			ApplicationOfferDetailsV5: params.ApplicationOfferDetailsV5{
				SourceModelTag:         names.NewModelTag(s.mockState.modelUUID).String(),
				ApplicationDescription: "description",
				OfferURL:               "fred@external/prod.hosted-db2",
				OfferName:              "hosted-db2",
				OfferUUID:              offerUUID,
				Endpoints:              []params.RemoteEndpoint{{Name: "db"}},
				Users: []params.OfferUserDetails{
					{UserName: "fred@external", DisplayName: "", Access: "admin"},
					{UserName: "mary", DisplayName: "mary", Access: "consume"},
				},
			},
			ApplicationName: "test",
			CharmURL:        "ch:db2-2",
			Connections: []params.OfferConnection{{
				SourceModelTag: names.NewModelTag(s.mockState.modelUUID).String(),
				RelationId:     1, Username: "fred@external", Endpoint: "db",
				Status:         params.EntityStatus{Status: "joined"},
				IngressSubnets: []string{"192.168.1.0/32", "10.0.0.0/8"},
			}},
		},
	}}

	// Expect call to get all permissions on the offer
	s.mockAccessService.EXPECT().ReadAllUserAccessForTarget(gomock.Any(), permission.ID{
		ObjectType: permission.Offer,
		Key:        offerUUID,
	}).Return([]permission.UserAccess{{
		UserName:    usertesting.GenNewName(c, "mary"),
		DisplayName: "mary",
		Access:      permission.ConsumeAccess,
	}}, nil).Times(2)

	s.authorizer.Tag = names.NewUserTag("admin")
	expected[0].Result.Users[0].UserName = "admin"
	// Expect getting api user from the database to retrieve their display name.
	s.mockAccessService.EXPECT().GetUserByName(gomock.Any(), coreuser.AdminUserName).Return(coreuser.User{
		DisplayName: "",
	}, nil)
	s.assertShow(c, "fred@external/prod.hosted-db2", offerUUID, expected)
	// Again with an unqualified model path.
	s.mockState.AdminTag = names.NewUserTag("fred@external")
	s.authorizer.AdminTag = s.mockState.AdminTag
	s.authorizer.Tag = s.mockState.AdminTag
	expected[0].Result.Users[0].UserName = "fred@external"
	s.applicationOffers.ResetCalls()
	// Expect getting api user from the database to retrieve their display name.
	s.mockAccessService.EXPECT().GetUserByName(gomock.Any(), usertesting.GenNewName(c, "fred@external")).Return(coreuser.User{
		DisplayName: "",
	}, nil)
	s.assertShow(c, "prod.hosted-db2", offerUUID, expected)
}

func (s *applicationOffersSuite) TestShowNoPermission(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.setupAPI(c)

	offerUUID := uuid.MustNewUUID().String()
	user := names.NewUserTag("someone")
	userName := coreuser.NameFromTag(user)
	// Expect getting api user from the database to retrieve their display name.
	s.mockAccessService.EXPECT().GetUserByName(gomock.Any(), userName).Return(coreuser.User{
		DisplayName: "",
	}, nil)
	// Expect call to get permissions on the offer
	s.mockAccessService.EXPECT().ReadUserAccessLevelForTarget(gomock.Any(), userName, permission.ID{
		ObjectType: permission.Offer,
		Key:        offerUUID,
	}).Return(permission.NoAccess, nil)

	s.authorizer.Tag = user
	expected := []params.ApplicationOfferResult{{
		Error: apiservererrors.ServerError(errors.NotFoundf("application offer %q", "fred@external/prod.hosted-db2")),
	}}
	s.assertShow(c, "fred@external/prod.hosted-db2", offerUUID, expected)
}

func (s *applicationOffersSuite) TestShowNonSuperuser(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.setupAPI(c)

	offerUUID := uuid.MustNewUUID().String()
	user := names.NewUserTag("someone")
	userName := coreuser.NameFromTag(user)
	s.authorizer.Tag = user
	expected := []params.ApplicationOfferResult{{
		Result: &params.ApplicationOfferAdminDetailsV5{
			ApplicationOfferDetailsV5: params.ApplicationOfferDetailsV5{
				SourceModelTag:         names.NewModelTag(s.mockState.modelUUID).String(),
				ApplicationDescription: "description",
				OfferURL:               "fred@external/prod.hosted-db2",
				OfferName:              "hosted-db2",
				OfferUUID:              offerUUID,
				Endpoints:              []params.RemoteEndpoint{{Name: "db"}},
				Users: []params.OfferUserDetails{
					{UserName: "someone", DisplayName: "someone", Access: "read"},
				},
			},
		}}}
	// Expect getting api user from the database to retrieve their display name.
	s.mockAccessService.EXPECT().GetUserByName(gomock.Any(), userName).Return(coreuser.User{
		DisplayName: "someone",
	}, nil)
	// Expect call to get permissions on the offer
	s.mockAccessService.EXPECT().ReadUserAccessLevelForTarget(gomock.Any(), userName, permission.ID{
		ObjectType: permission.Offer,
		Key:        offerUUID,
	}).Return(permission.ReadAccess, nil)
	s.assertShow(c, "fred@external/prod.hosted-db2", offerUUID, expected)
}

func (s *applicationOffersSuite) TestShowNonSuperuserExternal(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.setupAPI(c)

	offerUUID := uuid.MustNewUUID().String()
	user := names.NewUserTag("fred@external")
	userName := coreuser.NameFromTag(user)
	s.authorizer.Tag = user
	expected := []params.ApplicationOfferResult{{
		Result: &params.ApplicationOfferAdminDetailsV5{
			ApplicationOfferDetailsV5: params.ApplicationOfferDetailsV5{
				SourceModelTag:         names.NewModelTag(s.mockState.modelUUID).String(),
				ApplicationDescription: "description",
				OfferURL:               "fred@external/prod.hosted-db2",
				OfferName:              "hosted-db2",
				OfferUUID:              offerUUID,
				Endpoints:              []params.RemoteEndpoint{{Name: "db"}},
				Users: []params.OfferUserDetails{
					{UserName: "fred@external", DisplayName: "fred@external", Access: "read"},
				},
			},
		}}}
	// Expect getting api user from the database to retrieve their display name.
	s.mockAccessService.EXPECT().GetUserByName(gomock.Any(), userName).Return(coreuser.User{
		DisplayName: "fred@external",
	}, nil)
	// Expect call to get permissions on the offer
	s.mockAccessService.EXPECT().ReadUserAccessLevelForTarget(gomock.Any(), userName, permission.ID{
		ObjectType: permission.Offer,
		Key:        offerUUID,
	}).Return(permission.ReadAccess, nil)
	s.assertShow(c, "fred@external/prod.hosted-db2", offerUUID, expected)
}

func (s *applicationOffersSuite) TestShowError(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.setupAPI(c)

	url := "fred@external/prod.hosted-db2"
	filter := params.OfferURLs{OfferURLs: []string{url}, BakeryVersion: bakery.LatestVersion}
	msg := "fail"

	s.applicationOffers.listOffers = func(filters ...jujucrossmodel.ApplicationOfferFilter) ([]jujucrossmodel.ApplicationOffer, error) {
		return nil, errors.New(msg)
	}
	s.mockState.model = &mockModel{uuid: s.mockState.modelUUID, name: "prod", owner: "fred@external", modelType: state.ModelTypeIAAS}

	s.mockModelService.EXPECT().GetModelByNameAndOwner(gomock.Any(), s.mockState.model.name, coreuser.NameFromTag(s.mockState.model.Owner())).Return(
		coremodel.Model{
			Name:      s.mockState.model.name,
			OwnerName: coreuser.NameFromTag(s.mockState.model.Owner()),
			UUID:      coremodel.UUID(s.mockState.model.uuid),
			ModelType: coremodel.ModelType(s.mockState.model.modelType),
		}, nil,
	)

	_, err := s.api.ApplicationOffers(context.Background(), filter)
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf(".*%v.*", msg))
	s.applicationOffers.CheckCallNames(c, listOffersBackendCall)
}

func (s *applicationOffersSuite) TestShowNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.setupAPI(c)

	urls := []string{"fred@external/prod.hosted-db2"}
	filter := params.OfferURLs{OfferURLs: urls, BakeryVersion: bakery.LatestVersion}

	s.applicationOffers.listOffers = func(filters ...jujucrossmodel.ApplicationOfferFilter) ([]jujucrossmodel.ApplicationOffer, error) {
		return nil, nil
	}
	s.mockState.model = &mockModel{uuid: s.mockState.modelUUID, name: "prod", owner: "fred@external", modelType: state.ModelTypeIAAS}

	s.mockModelService.EXPECT().GetModelByNameAndOwner(gomock.Any(), s.mockState.model.name, coreuser.NameFromTag(s.mockState.model.Owner())).Return(
		coremodel.Model{
			Name:      s.mockState.model.name,
			OwnerName: coreuser.NameFromTag(s.mockState.model.Owner()),
			UUID:      coremodel.UUID(s.mockState.model.uuid),
			ModelType: coremodel.ModelType(s.mockState.model.modelType),
		}, nil,
	)
	// Expect getting api users display name from database.
	s.mockAccessService.EXPECT().GetUserByName(gomock.Any(), coreuser.NameFromTag(s.authorizer.Tag.(names.UserTag))).Return(coreuser.User{
		DisplayName: "",
	}, nil)

	found, err := s.api.ApplicationOffers(context.Background(), filter)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.Results, gc.HasLen, 1)
	c.Assert(found.Results[0].Error.Error(), gc.Matches, `application offer "fred@external/prod.hosted-db2" not found`)
	s.applicationOffers.CheckCallNames(c, listOffersBackendCall)
}

func (s *applicationOffersSuite) TestShowRejectsEndpoints(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.setupAPI(c)

	urls := []string{"fred@external/prod.hosted-db2:db"}
	filter := params.OfferURLs{OfferURLs: urls, BakeryVersion: bakery.LatestVersion}
	s.mockState.model = &mockModel{uuid: s.mockState.modelUUID, name: "prod", owner: "fred@external", modelType: state.ModelTypeIAAS}

	found, err := s.api.ApplicationOffers(context.Background(), filter)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.Results, gc.HasLen, 1)
	c.Assert(found.Results[0].Error.Message, gc.Equals, `saas application "fred@external/prod.hosted-db2:db" shouldn't include endpoint`)
}

func (s *applicationOffersSuite) TestShowErrorMsgMultipleURLs(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.setupAPI(c)

	urls := []string{"fred@external/prod.hosted-mysql", "fred@external/test.hosted-db2"}
	filter := params.OfferURLs{OfferURLs: urls, BakeryVersion: bakery.LatestVersion}

	s.applicationOffers.listOffers = func(filters ...jujucrossmodel.ApplicationOfferFilter) ([]jujucrossmodel.ApplicationOffer, error) {
		return nil, nil
	}
	s.mockState.model = &mockModel{uuid: s.mockState.modelUUID, name: "prod", owner: "fred@external", modelType: state.ModelTypeIAAS}
	anotherModel := &mockModel{uuid: "uuid2", name: "test", owner: "fred@external", modelType: state.ModelTypeIAAS}

	s.mockStatePool.st["uuid2"] = &mockState{
		modelUUID: "uuid2",
		model:     anotherModel,
	}
	s.mockState.allmodels = []applicationoffers.Model{s.mockState.model, anotherModel}

	// Expect getting api users display name from database.
	s.mockAccessService.EXPECT().GetUserByName(gomock.Any(), coreuser.NameFromTag(s.authorizer.Tag.(names.UserTag))).Return(coreuser.User{
		DisplayName: "",
	}, nil).Times(2)

	s.mockModelService.EXPECT().GetModelByNameAndOwner(gomock.Any(), s.mockState.model.name, coreuser.NameFromTag(s.mockState.model.Owner())).Return(
		coremodel.Model{
			Name:      s.mockState.model.name,
			OwnerName: coreuser.NameFromTag(s.mockState.model.Owner()),
			UUID:      coremodel.UUID(s.mockState.model.uuid),
			ModelType: coremodel.ModelType(s.mockState.model.modelType),
		}, nil,
	)
	s.mockModelService.EXPECT().GetModelByNameAndOwner(gomock.Any(), anotherModel.name, coreuser.NameFromTag(anotherModel.Owner())).Return(
		coremodel.Model{
			Name:      anotherModel.Name(),
			OwnerName: coreuser.NameFromTag(anotherModel.Owner()),
			UUID:      coremodel.UUID("uuid2"),
			ModelType: coremodel.IAAS,
		}, nil,
	)

	found, err := s.api.ApplicationOffers(context.Background(), filter)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.Results, gc.HasLen, 2)
	c.Assert(found.Results[0].Error.Error(), gc.Matches, `application offer "fred@external/prod.hosted-mysql" not found`)
	c.Assert(found.Results[1].Error.Error(), gc.Matches, `application offer "fred@external/test.hosted-db2" not found`)
	s.applicationOffers.CheckCallNames(c, listOffersBackendCall, listOffersBackendCall)
}

func (s *applicationOffersSuite) TestShowFoundMultiple(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.setupAPI(c)

	name := "test"
	url := "fred@external/prod.hosted-" + name
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

	filter := params.OfferURLs{OfferURLs: []string{url, url2}, BakeryVersion: bakery.LatestVersion}

	s.applicationOffers.listOffers = func(filters ...jujucrossmodel.ApplicationOfferFilter) ([]jujucrossmodel.ApplicationOffer, error) {
		c.Assert(filters, gc.HasLen, 1)
		if filters[0].OfferName == "hosted-test" {
			return []jujucrossmodel.ApplicationOffer{anOffer}, nil
		}
		return []jujucrossmodel.ApplicationOffer{anOffer2}, nil
	}
	s.mockState.applications = map[string]crossmodel.Application{
		"test": &mockApplication{
			curl: "ch:db2-2", bindings: map[string]string{"db": "myspace"}},
	}

	fakeModel := &mockModel{uuid: s.mockState.modelUUID, name: "prod", owner: "fred@external", modelType: state.ModelTypeIAAS}
	anotherModel := &mockModel{uuid: "uuid2", name: "test", owner: "mary", modelType: state.ModelTypeIAAS}

	s.mockState.model = fakeModel
	s.mockState.allmodels = []applicationoffers.Model{fakeModel, anotherModel}

	user := names.NewUserTag("someone")
	userName := coreuser.NameFromTag(user)
	s.authorizer.Tag = user

	anotherState := &mockState{
		modelUUID: "uuid2",
		model:     anotherModel,
	}
	anotherState.applications = map[string]crossmodel.Application{
		"testagain": &mockApplication{
			curl: "ch:mysql-2", bindings: map[string]string{"db2": "anotherspace"}},
	}
	s.mockStatePool.st["uuid2"] = anotherState

	// Read targets access level on the offer.
	s.mockAccessService.EXPECT().ReadUserAccessLevelForTarget(gomock.Any(), userName, permission.ID{
		ObjectType: permission.Offer,
		Key:        "hosted-test-uuid",
	}).Return(permission.ReadAccess, nil)
	// Read targets access level on the offer.
	s.mockAccessService.EXPECT().ReadUserAccessLevelForTarget(gomock.Any(), userName, permission.ID{
		ObjectType: permission.Offer,
		Key:        "hosted-testagain-uuid",
	}).Return(permission.ConsumeAccess, nil)
	// Expect getting api users display name from database.
	s.mockAccessService.EXPECT().GetUserByName(gomock.Any(), userName).Return(coreuser.User{
		DisplayName: "someone",
	}, nil).Times(2)

	s.mockModelService.EXPECT().GetModelByNameAndOwner(gomock.Any(), "prod", coreuser.NameFromTag(names.NewUserTag("fred@external"))).Return(
		coremodel.Model{
			Name:      "prod",
			OwnerName: coreuser.NameFromTag(names.NewUserTag("fred@external")),
			UUID:      coremodel.UUID(s.mockState.modelUUID),
			ModelType: coremodel.IAAS,
		}, nil,
	)
	s.mockModelService.EXPECT().GetModelByNameAndOwner(gomock.Any(), "test", coreuser.NameFromTag(names.NewUserTag("mary"))).Return(
		coremodel.Model{
			Name:      "test",
			OwnerName: coreuser.NameFromTag(names.NewUserTag("mary")),
			UUID:      coremodel.UUID("uuid2"),
			ModelType: coremodel.IAAS,
		}, nil,
	)

	found, err := s.api.ApplicationOffers(context.Background(), filter)
	c.Assert(err, jc.ErrorIsNil)
	var results []params.ApplicationOfferAdminDetailsV5
	for _, r := range found.Results {
		c.Assert(r.Error, gc.IsNil)
		results = append(results, *r.Result)
	}
	c.Assert(results, jc.DeepEquals, []params.ApplicationOfferAdminDetailsV5{
		{
			ApplicationOfferDetailsV5: params.ApplicationOfferDetailsV5{
				SourceModelTag:         names.NewModelTag(s.mockState.modelUUID).String(),
				ApplicationDescription: "description",
				OfferName:              "hosted-" + name,
				OfferUUID:              "hosted-" + name + "-uuid",
				OfferURL:               url,
				Endpoints:              []params.RemoteEndpoint{{Name: "db"}},
				Users: []params.OfferUserDetails{
					{UserName: "someone", DisplayName: "someone", Access: "read"},
				},
			},
		}, {
			ApplicationOfferDetailsV5: params.ApplicationOfferDetailsV5{
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

func (s *applicationOffersSuite) assertFind(c *gc.C, expected []params.ApplicationOfferAdminDetailsV5) {
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
	found, err := s.api.FindApplicationOffers(context.Background(), filter)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found, jc.DeepEquals, params.QueryApplicationOffersResultsV5{
		Results: expected,
	})
	s.applicationOffers.CheckCallNames(c, listOffersBackendCall)
	if len(expected) == 0 {
		return
	}
}

func (s *applicationOffersSuite) TestFind(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.setupAPI(c)

	offerUUID := s.setupOffers(c, "", true)

	// Get the admin user from the database to retrieve their display name.
	s.mockAccessService.EXPECT().GetUserByName(gomock.Any(), coreuser.AdminUserName).Return(coreuser.User{
		DisplayName: "admin",
	}, nil)
	// Since user is admin, get everyone else's access on the offer.
	s.mockAccessService.EXPECT().ReadAllUserAccessForTarget(gomock.Any(), permission.ID{
		ObjectType: permission.Offer,
		Key:        offerUUID,
	})

	s.authorizer.Tag = names.NewUserTag("admin")
	expected := []params.ApplicationOfferAdminDetailsV5{
		{
			ApplicationOfferDetailsV5: params.ApplicationOfferDetailsV5{
				SourceModelTag:         names.NewModelTag(s.mockState.modelUUID).String(),
				ApplicationDescription: "description",
				OfferName:              "hosted-db2",
				OfferUUID:              offerUUID,
				OfferURL:               "fred@external/prod.hosted-db2",
				Endpoints:              []params.RemoteEndpoint{{Name: "db"}},
				Users: []params.OfferUserDetails{
					{UserName: "admin", DisplayName: "admin", Access: "admin"},
				}},
			ApplicationName: "test",
			CharmURL:        "ch:db2-2",
			Connections: []params.OfferConnection{{
				SourceModelTag: names.NewModelTag(s.mockState.modelUUID).String(),
				RelationId:     1, Username: "fred@external", Endpoint: "db",
				Status:         params.EntityStatus{Status: "joined"},
				IngressSubnets: []string{"192.168.1.0/32", "10.0.0.0/8"},
			}},
		},
	}
	s.assertFind(c, expected)
}

func (s *applicationOffersSuite) TestFindNoPermission(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.setupAPI(c)

	user := names.NewUserTag("someone")
	userName := coreuser.NameFromTag(user)
	offerUUID := s.setupOffers(c, "", true)

	// Get api user from the database to retrieve their display name.
	s.mockAccessService.EXPECT().GetUserByName(gomock.Any(), userName).Return(coreuser.User{
		DisplayName: "admin",
	}, nil)
	// Read targets access level on the offer.
	s.mockAccessService.EXPECT().ReadUserAccessLevelForTarget(gomock.Any(), userName, permission.ID{
		ObjectType: permission.Offer,
		Key:        offerUUID,
	}).Return(permission.NoAccess, nil)

	s.authorizer.Tag = names.NewUserTag("someone")
	s.assertFind(c, []params.ApplicationOfferAdminDetailsV5{})
}

func (s *applicationOffersSuite) TestFindPermission(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.setupAPI(c)

	offerUUID := s.setupOffers(c, "", true)
	user := names.NewUserTag("someone")
	userName := coreuser.NameFromTag(user)
	s.authorizer.Tag = user

	// Get api user from the database to retrieve their display name.
	s.mockAccessService.EXPECT().GetUserByName(gomock.Any(), userName).Return(coreuser.User{
		DisplayName: "someone",
	}, nil)
	// Read targets access level on the offer.
	s.mockAccessService.EXPECT().ReadUserAccessLevelForTarget(gomock.Any(), userName, permission.ID{
		ObjectType: permission.Offer,
		Key:        offerUUID,
	}).Return(permission.ReadAccess, nil)

	expected := []params.ApplicationOfferAdminDetailsV5{
		{
			ApplicationOfferDetailsV5: params.ApplicationOfferDetailsV5{
				SourceModelTag:         names.NewModelTag(s.mockState.modelUUID).String(),
				ApplicationDescription: "description",
				OfferName:              "hosted-db2",
				OfferUUID:              offerUUID,
				OfferURL:               "fred@external/prod.hosted-db2",
				Endpoints:              []params.RemoteEndpoint{{Name: "db"}},
				Users: []params.OfferUserDetails{
					{UserName: "someone", DisplayName: "someone", Access: "read"},
				}},
		},
	}
	s.assertFind(c, expected)
}

func (s *applicationOffersSuite) TestFindFiltersRequireModel(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.setupAPI(c)

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
	_, err := s.api.FindApplicationOffers(context.Background(), filter)
	c.Assert(err, gc.ErrorMatches, "application offer filter must specify a model name")
}

func (s *applicationOffersSuite) TestFindRequiresFilter(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.setupAPI(c)

	s.setupOffers(c, "", true)

	_, err := s.api.FindApplicationOffers(context.Background(), params.OfferFilters{})
	c.Assert(err, gc.ErrorMatches, "at least one offer filter is required")
}

func (s *applicationOffersSuite) TestFindMulti(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.setupAPI(c)

	oneOfferUUID := uuid.MustNewUUID().String()
	twoOfferUUID := uuid.MustNewUUID().String()
	db2Offer := jujucrossmodel.ApplicationOffer{
		OfferName:              "hosted-db2",
		OfferUUID:              oneOfferUUID,
		ApplicationName:        "db2",
		ApplicationDescription: "db2 description",
		Endpoints:              map[string]charm.Relation{"db": {Name: "db2"}},
	}
	mysqlOffer := jujucrossmodel.ApplicationOffer{
		OfferName:              "hosted-mysql",
		OfferUUID:              twoOfferUUID,
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
	s.mockState.applications = map[string]crossmodel.Application{
		"db2": &mockApplication{
			name: "db2",
			curl: "ch:db2-2",
			bindings: map[string]string{
				"db2": "myspace",
			},
		},
	}
	s.mockState.model = &mockModel{
		uuid:      s.mockState.modelUUID,
		name:      "prod",
		owner:     "fred@external",
		modelType: state.ModelTypeIAAS,
	}

	anotherState := &mockState{
		modelUUID: "uuid2",
	}
	s.mockStatePool.st["uuid2"] = anotherState
	anotherState.applications = map[string]crossmodel.Application{
		"mysql": &mockApplication{
			name: "mysql",
			curl: "ch:mysql-2",
			bindings: map[string]string{
				"mysql": "anotherspace",
			},
		},
		"postgresql": &mockApplication{
			curl: "ch:postgresql-2",
			bindings: map[string]string{
				"postgresql": "anotherspace",
			},
		},
	}
	anotherState.model = &mockModel{
		uuid:      "uuid2",
		name:      "another",
		owner:     "mary",
		modelType: state.ModelTypeIAAS,
	}
	s.mockState.relations["hosted-mysql:server wordpress:db"] = &mockRelation{
		id: 1,
		endpoint: relation.Endpoint{
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
			username:    "fred@external",
			modelUUID:   s.mockState.modelUUID,
			relationKey: "hosted-db2:db wordpress:db",
			relationId:  1,
		},
	}

	s.mockState.allmodels = []applicationoffers.Model{
		s.mockState.model,
		anotherState.model,
	}

	filter := params.OfferFilters{
		Filters: []params.OfferFilter{
			{
				OfferName: "hosted-db2",
				OwnerName: "fred@external",
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

	user := names.NewUserTag("someone")
	userName := coreuser.NameFromTag(user)
	s.authorizer.Tag = user
	// Read user access level on each offer.
	s.mockAccessService.EXPECT().ReadUserAccessLevelForTarget(gomock.Any(), userName, permission.ID{
		ObjectType: permission.Offer,
		Key:        oneOfferUUID,
	}).Return(permission.ConsumeAccess, nil)
	s.mockAccessService.EXPECT().ReadUserAccessLevelForTarget(gomock.Any(), userName, permission.ID{
		ObjectType: permission.Offer,
		Key:        twoOfferUUID,
	}).Return(permission.ReadAccess, nil)
	s.mockAccessService.EXPECT().ReadUserAccessLevelForTarget(gomock.Any(), userName, permission.ID{
		ObjectType: permission.Offer,
		Key:        "hosted-postgresql-uuid",
	}).Return(permission.AdminAccess, nil)
	// Since user is admin, get everyone else's access on the offer.
	s.mockAccessService.EXPECT().ReadAllUserAccessForTarget(gomock.Any(), permission.ID{
		ObjectType: permission.Offer,
		Key:        "hosted-postgresql-uuid",
	}).Return([]permission.UserAccess{{UserName: userName, DisplayName: "someone", Access: "read"}}, nil)
	// Get the user from the database to retrieve their display name.
	s.mockAccessService.EXPECT().GetUserByName(gomock.Any(), userName).Return(coreuser.User{
		DisplayName: "someone",
	}, nil).AnyTimes()
	s.mockModelService.EXPECT().GetModelByNameAndOwner(gomock.Any(), "prod", coreuser.NameFromTag(names.NewUserTag("fred@external"))).Return(
		coremodel.Model{
			Name:      "prod",
			OwnerName: coreuser.NameFromTag(names.NewUserTag("fred@external")),
			UUID:      coremodel.UUID(s.mockState.modelUUID),
			ModelType: coremodel.IAAS,
		}, nil,
	)
	s.mockModelService.EXPECT().GetModelByNameAndOwner(gomock.Any(), "another", coreuser.NameFromTag(names.NewUserTag("mary"))).Return(
		coremodel.Model{
			Name:      "another",
			OwnerName: coreuser.NameFromTag(names.NewUserTag("mary")),
			UUID:      coremodel.UUID("uuid2"),
			ModelType: coremodel.IAAS,
		}, nil,
	)

	found, err := s.api.FindApplicationOffers(context.Background(), filter)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found, jc.DeepEquals, params.QueryApplicationOffersResultsV5{
		Results: []params.ApplicationOfferAdminDetailsV5{
			{
				ApplicationOfferDetailsV5: params.ApplicationOfferDetailsV5{
					SourceModelTag:         names.NewModelTag(s.mockState.modelUUID).String(),
					ApplicationDescription: "db2 description",
					OfferName:              "hosted-db2",
					OfferUUID:              oneOfferUUID,
					OfferURL:               "fred@external/prod.hosted-db2",
					Endpoints: []params.RemoteEndpoint{
						{Name: "db"},
					},
					Users: []params.OfferUserDetails{
						{UserName: "someone", DisplayName: "someone", Access: "consume"},
					},
				},
			},
			{
				ApplicationOfferDetailsV5: params.ApplicationOfferDetailsV5{
					SourceModelTag:         "model-uuid2",
					ApplicationDescription: "mysql description",
					OfferName:              "hosted-mysql",
					OfferUUID:              twoOfferUUID,
					OfferURL:               "mary/another.hosted-mysql",
					Endpoints: []params.RemoteEndpoint{
						{Name: "db"},
					},
					Users: []params.OfferUserDetails{
						{UserName: "someone", DisplayName: "someone", Access: "read"},
					},
				},
			},
			{
				ApplicationOfferDetailsV5: params.ApplicationOfferDetailsV5{
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
				CharmURL: "ch:postgresql-2",
			},
		},
	})
	s.applicationOffers.CheckCallNames(c, listOffersBackendCall, listOffersBackendCall)
}

func (s *applicationOffersSuite) TestFindError(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.setupAPI(c)

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
	s.mockState.model = &mockModel{uuid: s.mockState.modelUUID, name: "prod", owner: "fred@external", modelType: state.ModelTypeIAAS}

	s.mockModelService.EXPECT().ListAllModels(gomock.Any()).Return(
		[]coremodel.Model{
			{
				Name:      s.mockState.model.name,
				OwnerName: coreuser.NameFromTag(s.mockState.model.Owner()),
				UUID:      coremodel.UUID(s.mockState.model.uuid),
				ModelType: coremodel.ModelType(s.mockState.model.modelType),
			},
		}, nil,
	)
	s.mockModelService.EXPECT().GetModelByNameAndOwner(gomock.Any(), s.mockState.model.name, coreuser.NameFromTag(s.mockState.model.Owner())).Return(
		coremodel.Model{
			Name:      s.mockState.model.name,
			OwnerName: coreuser.NameFromTag(s.mockState.model.Owner()),
			UUID:      coremodel.UUID(s.mockState.model.uuid),
			ModelType: coremodel.ModelType(s.mockState.model.modelType),
		}, nil,
	)

	_, err := s.api.FindApplicationOffers(context.Background(), filter)
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf(".*%v.*", msg))
	s.applicationOffers.CheckCallNames(c, listOffersBackendCall)
}

func (s *applicationOffersSuite) TestFindMissingModelInMultipleFilters(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.setupAPI(c)

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

	_, err := s.api.FindApplicationOffers(context.Background(), filter)
	c.Assert(err, gc.ErrorMatches, "application offer filter must specify a model name")
	s.applicationOffers.CheckCallNames(c)
}

type consumeSuite struct {
	baseSuite
	api *applicationoffers.OffersAPIv5
}

var _ = gc.Suite(&consumeSuite{})

func (s *consumeSuite) SetUpTest(c *gc.C) {
	s.baseSuite.SetUpTest(c)
	s.bakery = &mockBakeryService{caveats: make(map[string][]checkers.Caveat)}
	var err error
	thirdPartyKey := bakery.MustGenerateKey()
	s.authContext, err = crossmodel.NewAuthContext(
		s.mockState, nil, testing.ModelTag, thirdPartyKey,
		crossmodel.NewOfferBakeryForTest(s.bakery, clock.WallClock),
	)
	c.Assert(err, jc.ErrorIsNil)
}

// Creates the API to use in testing.
// Call baseSuite.setupMocks before this.
func (s *consumeSuite) setupAPI(c *gc.C) {
	getApplicationOffers := func(st interface{}) jujucrossmodel.ApplicationOffers {
		return &mockApplicationOffers{st: st.(*mockState)}
	}
	api, err := applicationoffers.CreateOffersAPI(
		getApplicationOffers, getFakeControllerInfo,
		s.mockState, s.mockStatePool, s.mockAccessService,
		s.mockModelDomainServicesGetter,
		s.authorizer, s.authContext,
		c.MkDir(),
		loggertesting.WrapCheckLog(c),
		testing.ControllerTag.Id(),
		s.mockModelService,
	)
	c.Assert(err, jc.ErrorIsNil)
	s.api = api
}

func (s *consumeSuite) TestConsumeDetailsRejectsEndpoints(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.setupAPI(c)

	results, err := s.api.GetConsumeDetails(
		context.Background(),
		params.ConsumeOfferDetailsArg{
			OfferURLs: params.OfferURLs{
				OfferURLs: []string{"fred@external/prod.application:db"},
			}},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error != nil, jc.IsTrue)
	c.Assert(results.Results[0].Error.Message, gc.Equals, `saas application "fred@external/prod.application:db" shouldn't include endpoint`)
}

func (s *consumeSuite) TestConsumeDetailsNoPermission(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.setupAPI(c)

	offerUUID := s.setupOffer()

	apiUser := names.NewUserTag("someone")
	apiUserName := coreuser.NameFromTag(apiUser)

	// Expect call to get permissions on the offer
	s.mockAccessService.EXPECT().ReadUserAccessLevelForTarget(gomock.Any(), apiUserName, permission.ID{
		ObjectType: permission.Offer,
		Key:        offerUUID,
	}).Return(permission.NoAccess, nil)
	// Get the admin user from the database to retrieve their display name.
	s.mockAccessService.EXPECT().GetUserByName(gomock.Any(), apiUserName).Return(coreuser.User{
		DisplayName: "someone",
	}, nil)

	s.authorizer.Tag = apiUser
	results, err := s.api.GetConsumeDetails(
		context.Background(),
		params.ConsumeOfferDetailsArg{
			OfferURLs: params.OfferURLs{
				OfferURLs: []string{"fred@external/prod.hosted-mysql"},
			}},
	)
	c.Assert(err, jc.ErrorIsNil)
	expected := []params.ConsumeOfferDetailsResult{{
		Error: apiservererrors.ServerError(errors.NotFoundf("application offer %q", "fred@external/prod.hosted-mysql")),
	}}
	c.Assert(results.Results, jc.DeepEquals, expected)
}

func (s *consumeSuite) TestConsumeDetailsWithPermission(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.setupAPI(c)

	s.assertConsumeDetailsWithPermission(c,
		func(authorizer *apiservertesting.FakeAuthorizer, apiUser names.UserTag) string {
			authorizer.HasConsumeTag = apiUser
			authorizer.Tag = apiUser
			return ""
		},
	)
}

func (s *consumeSuite) TestConsumeDetailsSpecifiedUserHasPermission(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.setupAPI(c)

	s.assertConsumeDetailsWithPermission(c,
		func(authorizer *apiservertesting.FakeAuthorizer, apiUser names.UserTag) string {
			authorizer.HasConsumeTag = apiUser
			controllerAdmin := names.NewUserTag("superuser-joe")
			authorizer.Tag = controllerAdmin
			return apiUser.String()
		},
	)
}

func (s *consumeSuite) TestConsumeDetailsSpecifiedUserHasNoPermissionButSuperUserLoggedIn(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.setupAPI(c)

	s.assertConsumeDetailsWithPermission(c,
		func(authorizer *apiservertesting.FakeAuthorizer, apiUser names.UserTag) string {
			controllerAdmin := names.NewUserTag("superuser-joe")
			authorizer.Tag = controllerAdmin
			return apiUser.String()
		},
	)
}

func (s *consumeSuite) assertConsumeDetailsWithPermission(
	c *gc.C, configAuthorizer func(*apiservertesting.FakeAuthorizer, names.UserTag) string,
) {
	offerUUID := s.setupOffer()

	apiUser := names.NewUserTag("someone")

	userTag := configAuthorizer(s.authorizer, apiUser)
	// Expect call to get permissions on the offer
	s.mockAccessService.EXPECT().ReadUserAccessLevelForTarget(gomock.Any(), coreuser.NameFromTag(apiUser), permission.ID{
		ObjectType: permission.Offer,
		Key:        offerUUID,
	}).Return(permission.ConsumeAccess, nil)
	// Get the api user from the database to retrieve their display name.
	s.mockAccessService.EXPECT().GetUserByName(gomock.Any(), usertesting.GenNewName(c, "someone")).Return(coreuser.User{
		DisplayName: "someone",
	}, nil)

	results, err := s.api.GetConsumeDetails(
		context.Background(),
		params.ConsumeOfferDetailsArg{
			UserTag: userTag,
			OfferURLs: params.OfferURLs{
				OfferURLs: []string{"fred@external/prod.hosted-mysql"},
			}},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)
	c.Assert(results.Results[0].Offer, jc.DeepEquals, &params.ApplicationOfferDetailsV5{
		SourceModelTag:         names.NewModelTag(s.mockState.modelUUID).String(),
		OfferURL:               "fred@external/prod.hosted-mysql",
		OfferName:              "hosted-mysql",
		OfferUUID:              offerUUID,
		ApplicationDescription: "a database",
		Endpoints:              []params.RemoteEndpoint{{Name: "server", Role: "provider", Interface: "mysql"}},
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
	c.Check(cav[1].Condition, gc.Equals, fmt.Sprintf("declared source-model-uuid %v", s.mockState.modelUUID))
	c.Check(cav[2].Condition, gc.Equals, "declared username someone")
	c.Check(cav[3].Condition, gc.Equals, "declared offer-uuid "+offerUUID)
}

func (s *consumeSuite) TestConsumeDetailsNonAdminSpecifiedUser(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.setupAPI(c)

	_ = s.setupOffer()
	apiUser := names.NewUserTag("someone")

	s.authorizer.Tag = names.NewUserTag("joe-blow")
	_, err := s.api.GetConsumeDetails(
		context.Background(),
		params.ConsumeOfferDetailsArg{
			UserTag: apiUser.String(),
			OfferURLs: params.OfferURLs{
				OfferURLs: []string{"fred@external/prod.hosted-mysql"},
			}},
	)
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *consumeSuite) TestConsumeDetailsDefaultEndpoint(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.setupAPI(c)

	offerUUID := s.setupOffer()

	st := s.mockStatePool.st[s.mockState.modelUUID].(*mockState)

	delete(st.applications["mysql"].(*mockApplication).bindings, "database")

	// Add a default endpoint for the application.
	st.applications["mysql"].(*mockApplication).bindings[""] = "default-endpoint"

	apiUser := names.NewUserTag("someone")
	apiUserName := coreuser.NameFromTag(apiUser)
	s.authorizer.Tag = apiUser
	s.authorizer.HasConsumeTag = apiUser

	// Expect call to get permissions on the offer
	s.mockAccessService.EXPECT().ReadUserAccessLevelForTarget(gomock.Any(), apiUserName, permission.ID{
		ObjectType: permission.Offer,
		Key:        offerUUID,
	}).Return(permission.ConsumeAccess, nil)
	// Get the admin user from the database to retrieve their display name.
	s.mockAccessService.EXPECT().GetUserByName(gomock.Any(), apiUserName).Return(coreuser.User{
		DisplayName: "someone",
	}, nil)

	results, err := s.api.GetConsumeDetails(
		context.Background(),
		params.ConsumeOfferDetailsArg{
			OfferURLs: params.OfferURLs{
				OfferURLs: []string{"fred@external/prod.hosted-mysql"},
			},
		},
	)

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)
	c.Assert(results.Results[0].Offer, jc.DeepEquals, &params.ApplicationOfferDetailsV5{
		SourceModelTag:         names.NewModelTag(s.mockState.modelUUID).String(),
		OfferURL:               "fred@external/prod.hosted-mysql",
		OfferName:              "hosted-mysql",
		OfferUUID:              offerUUID,
		ApplicationDescription: "a database",
		Endpoints:              []params.RemoteEndpoint{{Name: "server", Role: "provider", Interface: "mysql"}},
		Users: []params.OfferUserDetails{
			{UserName: "someone", DisplayName: "someone", Access: "consume"},
		},
	})
}

func (s *consumeSuite) setupOffer() string {
	modelUUID := s.mockState.modelUUID
	offerName := "hosted-mysql"

	model := &mockModel{uuid: modelUUID, name: "prod", owner: "fred@external", modelType: state.ModelTypeIAAS}
	s.mockModelService.EXPECT().ListAllModels(gomock.Any()).Return(
		[]coremodel.Model{
			{
				Name:      model.name,
				OwnerName: coreuser.NameFromTag(names.NewUserTag(model.owner)),
				UUID:      coremodel.UUID(model.uuid),
				ModelType: coremodel.ModelType(model.modelType),
			},
		}, nil,
	).AnyTimes()
	s.mockModelService.EXPECT().GetModelByNameAndOwner(gomock.Any(), model.name, coreuser.NameFromTag(model.Owner())).Return(
		coremodel.Model{
			Name:      model.name,
			OwnerName: coreuser.NameFromTag(model.Owner()),
			UUID:      coremodel.UUID(model.uuid),
			ModelType: coremodel.ModelType(model.modelType),
		}, nil,
	).AnyTimes()
	s.mockState.allmodels = []applicationoffers.Model{model}
	st := &mockState{
		modelUUID:         modelUUID,
		model:             model,
		applications:      make(map[string]crossmodel.Application),
		applicationOffers: make(map[string]jujucrossmodel.ApplicationOffer),
		relations:         make(map[string]crossmodel.Relation),
	}
	s.mockStatePool.st[modelUUID] = st
	anOffer := jujucrossmodel.ApplicationOffer{
		ApplicationName:        "mysql",
		ApplicationDescription: "a database",
		OfferName:              offerName,
		OfferUUID:              uuid.MustNewUUID().String(),
		Endpoints: map[string]charm.Relation{
			"server": {Name: "database", Interface: "mysql", Role: "provider", Scope: "global"}},
	}
	st.applicationOffers[offerName] = anOffer
	st.applications["mysql"] = &mockApplication{
		name:     "mysql",
		bindings: map[string]string{"database": "myspace"},
		endpoints: []relation.Endpoint{
			{Relation: charm.Relation{Name: "juju-info", Role: "provider", Interface: "juju-info", Limit: 0, Scope: "global"}},
			{Relation: charm.Relation{Name: "server", Role: "provider", Interface: "mysql", Limit: 0, Scope: "global"}},
			{Relation: charm.Relation{Name: "server-admin", Role: "provider", Interface: "mysql-root", Limit: 0, Scope: "global"}}},
	}
	return anOffer.OfferUUID
}

func (s *consumeSuite) TestRemoteApplicationInfo(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.setupAPI(c)

	offerUUID := s.setupOffer()

	// Give user permission to see the offer.
	user := names.NewUserTag("foobar")
	userName := coreuser.NameFromTag(user)
	// Expect getting api user from the database to retrieve their display name.
	s.mockAccessService.EXPECT().GetUserByName(gomock.Any(), userName).Return(coreuser.User{
		DisplayName: "",
	}, nil).Times(2)
	// Expect call to get permissions on the offer
	s.mockAccessService.EXPECT().ReadUserAccessLevelForTarget(gomock.Any(), userName, permission.ID{
		ObjectType: permission.Offer,
		Key:        offerUUID,
	}).Return(permission.ConsumeAccess, nil)

	s.authorizer.Tag = user
	results, err := s.api.RemoteApplicationInfo(context.Background(), params.OfferURLs{
		OfferURLs: []string{"fred@external/prod.hosted-mysql", "fred@external/prod.unknown"},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 2)
	c.Assert(results.Results[0].Error, gc.IsNil)
	c.Assert(results.Results, jc.DeepEquals, []params.RemoteApplicationInfoResult{
		{Result: &params.RemoteApplicationInfo{
			ModelTag:         names.NewModelTag(s.mockState.modelUUID).String(),
			Name:             "hosted-mysql",
			Description:      "a database",
			OfferURL:         "fred@external/prod.hosted-mysql",
			SourceModelLabel: "prod",
			IconURLPath:      "rest/1.0/remote-application/hosted-mysql/icon",
			Endpoints: []params.RemoteEndpoint{
				{Name: "server", Role: "provider", Interface: "mysql"}},
		}},
		{
			Error: &params.Error{Message: `application offer "unknown" not found`, Code: "not found"},
		},
	})
}

func (s *consumeSuite) TestDestroyOffersNoForceV2(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.setupAPI(c)

	s.assertDestroyOffersNoForce(c)
}

func (s *consumeSuite) assertDestroyOffersNoForce(c *gc.C) {
	s.setupOffer()
	st := s.mockStatePool.st[s.mockState.modelUUID]
	st.(*mockState).connections = []applicationoffers.OfferConnection{
		&mockOfferConnection{
			username:    "fred@external",
			modelUUID:   s.mockState.modelUUID,
			relationKey: "hosted-db2:db wordpress:db",
			relationId:  1,
		},
	}

	s.authorizer.Tag = names.NewUserTag("admin")

	results, err := s.api.DestroyOffers(context.Background(), params.DestroyApplicationOffers{
		OfferURLs: []string{
			"fred@external/prod.hosted-mysql"},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results, jc.DeepEquals, []params.ErrorResult{
		{
			Error: &params.Error{Message: `offer has 1 relations`},
		},
	})

	urls := []string{"fred@external/prod.hosted-db2"}
	filter := params.OfferURLs{urls, bakery.LatestVersion}

	// Get the api user from the database to retrieve their display name.
	s.mockAccessService.EXPECT().GetUserByName(gomock.Any(), coreuser.AdminUserName).Return(coreuser.User{
		DisplayName: "admin",
	}, nil)

	found, err := s.api.ApplicationOffers(context.Background(), filter)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.Results, gc.HasLen, 1)
	c.Assert(found.Results[0].Error.Error(), gc.Matches, `application offer "fred@external/prod.hosted-db2" not found`)
}

func (s *consumeSuite) TestDestroyOffersForce(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.setupAPI(c)

	s.setupOffer()
	st := s.mockStatePool.st[s.mockState.modelUUID]

	st.(*mockState).connections = []applicationoffers.OfferConnection{
		&mockOfferConnection{
			username:    "fred@external",
			modelUUID:   s.mockState.modelUUID,
			relationKey: "hosted-db2:db wordpress:db",
			relationId:  1,
		},
	}
	s.authorizer.Tag = names.NewUserTag("admin")

	s.mockModelService.EXPECT().GetModelByNameAndOwner(gomock.Any(), "badmodel", coreuser.NameFromTag(names.NewUserTag("garbage"))).Return(
		coremodel.Model{}, modelerrors.NotFound,
	)
	s.mockModelService.EXPECT().GetModelByNameAndOwner(gomock.Any(), "badmodel", coreuser.NameFromTag(names.NewUserTag("admin"))).Return(
		coremodel.Model{}, modelerrors.NotFound,
	)

	results, err := s.api.DestroyOffers(context.Background(), params.DestroyApplicationOffers{
		Force: true,
		OfferURLs: []string{
			"fred@external/prod.hosted-mysql", "fred@external/prod.unknown", "garbage/badmodel.someoffer", "badmodel.someoffer"},
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

	urls := []string{"fred@external/prod.hosted-db2"}
	filter := params.OfferURLs{OfferURLs: urls, BakeryVersion: bakery.LatestVersion}

	// Get the api user from the database to retrieve their display name.
	s.mockAccessService.EXPECT().GetUserByName(gomock.Any(), coreuser.AdminUserName).Return(coreuser.User{
		DisplayName: "admin",
	}, nil)

	found, err := s.api.ApplicationOffers(context.Background(), filter)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.Results, gc.HasLen, 1)
	c.Assert(found.Results[0].Error.Error(), gc.Matches, `application offer "fred@external/prod.hosted-db2" not found`)
}

func (s *consumeSuite) TestDestroyOffersPermission(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.setupAPI(c)

	s.setupOffer()
	s.authorizer.Tag = names.NewUserTag("mary")

	results, err := s.api.DestroyOffers(context.Background(), params.DestroyApplicationOffers{
		OfferURLs: []string{"fred@external/prod.hosted-mysql"},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.ErrorMatches, apiservererrors.ErrPerm.Error())
}
