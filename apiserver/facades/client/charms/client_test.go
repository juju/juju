// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms

import (
	"context"
	stdtesting "testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/facade"
	apiservermocks "github.com/juju/juju/apiserver/facade/mocks"
	"github.com/juju/juju/apiserver/facades/client/charms/mocks"
	applicationtesting "github.com/juju/juju/core/application/testing"
	"github.com/juju/juju/core/arch"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/constraints"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/domain/application/architecture"
	domaincharm "github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/charm/repository"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type charmsMockSuite struct {
	authorizer   *apiservermocks.MockAuthorizer
	repository   *mocks.MockRepository
	charmArchive *mocks.MockCharmArchive

	modelConfigService *MockModelConfigService
	applicationService *MockApplicationService
	machineService     *MockMachineService
}

func TestCharmsMockSuite(t *stdtesting.T) {
	tc.Run(t, &charmsMockSuite{})
}

func (s *charmsMockSuite) TestListCharmsNoNames(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.applicationService.EXPECT().ListCharmLocators(gomock.Any(), []string{}).Return([]domaincharm.CharmLocator{{
		Name:         "dummy",
		Source:       domaincharm.CharmHubSource,
		Revision:     1,
		Architecture: architecture.AMD64,
	}}, nil)

	api := s.api(c)
	found, err := api.List(c.Context(), params.CharmsList{Names: []string{}})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(found.CharmURLs, tc.HasLen, 1)
	c.Check(found, tc.DeepEquals, params.CharmsListResult{
		CharmURLs: []string{"ch:amd64/dummy-1"},
	})
}

func (s *charmsMockSuite) TestListCharmsWithNames(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// We only return one of the names, because we couldn't find foo. This
	// shouldn't stop us from returning the other charm url.

	s.applicationService.EXPECT().ListCharmLocators(gomock.Any(), []string{"dummy", "foo"}).Return([]domaincharm.CharmLocator{{
		Name:         "dummy",
		Source:       domaincharm.CharmHubSource,
		Revision:     1,
		Architecture: architecture.AMD64,
	}}, nil)

	api := s.api(c)
	found, err := api.List(c.Context(), params.CharmsList{Names: []string{"dummy", "foo"}})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(found.CharmURLs, tc.HasLen, 1)
	c.Check(found, tc.DeepEquals, params.CharmsListResult{
		CharmURLs: []string{"ch:amd64/dummy-1"},
	})
}

func (s *charmsMockSuite) TestResolveCharms(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.expectResolveWithPreferredChannel(3, nil)
	api := s.api(c)

	curl := "ch:testme"
	fullCurl := "ch:amd64/testme"

	edgeOrigin := params.CharmOrigin{
		Source:       corecharm.CharmHub.String(),
		Type:         "charm",
		Risk:         "edge",
		Architecture: "amd64",
		Base: params.Base{
			Name:    "ubuntu",
			Channel: "20.04/stable",
		},
	}
	stableOrigin := params.CharmOrigin{
		Source:       corecharm.CharmHub.String(),
		Type:         "charm",
		Risk:         "stable",
		Architecture: "amd64",
		Base: params.Base{
			Name:    "ubuntu",
			Channel: "20.04/stable",
		},
	}

	args := params.ResolveCharmsWithChannel{
		Resolve: []params.ResolveCharmWithChannel{
			{Reference: curl, Origin: params.CharmOrigin{
				Source:       corecharm.CharmHub.String(),
				Architecture: "amd64",
			}},
			{Reference: curl, Origin: stableOrigin},
			{Reference: fullCurl, Origin: edgeOrigin},
		},
	}

	expected := []params.ResolveCharmWithChannelResult{
		{
			URL:    fullCurl,
			Origin: stableOrigin,
			SupportedBases: []params.Base{
				{Name: "ubuntu", Channel: "18.04"},
				{Name: "ubuntu", Channel: "20.04"},
				{Name: "ubuntu", Channel: "16.04"},
			},
		}, {
			URL:    fullCurl,
			Origin: stableOrigin,
			SupportedBases: []params.Base{
				{Name: "ubuntu", Channel: "18.04"},
				{Name: "ubuntu", Channel: "20.04"},
				{Name: "ubuntu", Channel: "16.04"},
			},
		},
		{
			URL:    fullCurl,
			Origin: edgeOrigin,
			SupportedBases: []params.Base{
				{Name: "ubuntu", Channel: "18.04"},
				{Name: "ubuntu", Channel: "20.04"},
				{Name: "ubuntu", Channel: "16.04"},
			},
		},
	}
	result, err := api.ResolveCharms(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 3)
	c.Assert(result.Results, tc.DeepEquals, expected)
}

func (s *charmsMockSuite) TestResolveCharmsUnknownSchema(c *tc.C) {
	defer s.setupMocks(c).Finish()
	api := s.api(c)

	curl, err := charm.ParseURL("local:testme")
	c.Assert(err, tc.ErrorIsNil)
	args := params.ResolveCharmsWithChannel{
		Resolve: []params.ResolveCharmWithChannel{{Reference: curl.String()}},
	}

	result, err := api.ResolveCharms(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	c.Assert(result.Results[0].Error, tc.ErrorMatches, `unknown schema for charm URL "local:testme"`)
}

func (s *charmsMockSuite) TestAddCharmWithLocalSource(c *tc.C) {
	defer s.setupMocks(c).Finish()
	api := s.api(c)

	curl := "local:testme"
	args := params.AddCharmWithOrigin{
		URL: curl,
		Origin: params.CharmOrigin{
			Source: "local",
		},
		Force: false,
	}
	_, err := api.AddCharm(c.Context(), args)
	c.Assert(err, tc.ErrorMatches, `unknown schema for charm URL "local:testme"`)
}

func (s *charmsMockSuite) TestAddCharmCharmhub(c *tc.C) {
	// Charmhub charms are downloaded asynchronously
	defer s.setupMocks(c).Finish()

	curl := charm.MustParseURL("chtest")

	requestedOrigin := corecharm.Origin{
		Source: "charm-hub",
		Channel: &charm.Channel{
			Risk: "edge",
		},
		Platform: corecharm.Platform{
			OS:      "ubuntu",
			Channel: "20.04",
		},
	}
	resolvedOrigin := corecharm.Origin{
		Source: "charm-hub",
		Channel: &charm.Channel{
			Risk: "stable",
		},
		Platform: corecharm.Platform{
			OS:      "ubuntu",
			Channel: "20.04",
		},
	}

	expMeta := new(charm.Meta)
	expManifest := new(charm.Manifest)
	expConfig := new(charm.Config)
	s.repository.EXPECT().ResolveForDeploy(gomock.Any(), corecharm.CharmID{
		URL:    curl,
		Origin: requestedOrigin,
	}).Return(corecharm.ResolvedDataForDeploy{
		URL: curl,
		EssentialMetadata: corecharm.EssentialMetadata{
			Meta:           expMeta,
			Manifest:       expManifest,
			Config:         expConfig,
			ResolvedOrigin: resolvedOrigin,
		},
	}, nil)

	s.applicationService.EXPECT().SetCharm(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, args domaincharm.SetCharmArgs) (corecharm.ID, []string, error) {
		c.Check(args.Charm.Meta(), tc.Equals, expMeta)
		c.Check(args.Charm.Manifest(), tc.Equals, expManifest)
		c.Check(args.Charm.Config(), tc.Equals, expConfig)
		return corecharm.ID(""), nil, nil
	})

	api := s.api(c)

	args := params.AddCharmWithOrigin{
		URL: curl.String(),
		Origin: params.CharmOrigin{
			Source: "charm-hub",
			Base:   params.Base{Name: "ubuntu", Channel: "20.04/stable"},
			Risk:   "edge",
		},
	}
	obtained, err := api.AddCharm(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(obtained, tc.DeepEquals, params.CharmOriginResult{
		Origin: params.CharmOrigin{
			Source: "charm-hub",
			Base:   params.Base{Name: "ubuntu", Channel: "20.04/stable"},
			Risk:   "stable",
		},
	})
}

func (s *charmsMockSuite) TestCheckCharmPlacementWithSubordinate(c *tc.C) {
	appName := "winnie"

	curl := "ch:poo"

	defer s.setupMocks(c).Finish()

	appUUID := applicationtesting.GenApplicationUUID(c)
	s.applicationService.EXPECT().GetApplicationIDByName(gomock.Any(), appName).Return(appUUID, nil)
	s.applicationService.EXPECT().IsSubordinateApplication(gomock.Any(), appUUID).Return(true, nil)

	api := s.api(c)

	args := params.ApplicationCharmPlacements{
		Placements: []params.ApplicationCharmPlacement{{
			Application: appName,
			CharmURL:    curl,
		}},
	}

	result, err := api.CheckCharmPlacement(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.OneError(), tc.ErrorIsNil)
}

func (s *charmsMockSuite) TestCheckCharmPlacementWithConstraintArch(c *tc.C) {
	arch := arch.DefaultArchitecture
	appName := "winnie"

	curl := "ch:poo"

	defer s.setupMocks(c).Finish()

	appUUID := applicationtesting.GenApplicationUUID(c)
	s.applicationService.EXPECT().GetApplicationIDByName(gomock.Any(), appName).Return(appUUID, nil)
	s.applicationService.EXPECT().IsSubordinateApplication(gomock.Any(), appUUID).Return(false, nil)
	s.applicationService.EXPECT().GetApplicationConstraints(gomock.Any(), appUUID).Return(constraints.Value{Arch: &arch}, nil)

	api := s.api(c)

	args := params.ApplicationCharmPlacements{
		Placements: []params.ApplicationCharmPlacement{{
			Application: appName,
			CharmURL:    curl,
		}},
	}

	result, err := api.CheckCharmPlacement(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.OneError(), tc.ErrorIsNil)
}

func (s *charmsMockSuite) TestCheckCharmPlacementWithHomogeneous(c *tc.C) {
	appName := "winnie"

	curl := "ch:poo"

	defer s.setupMocks(c).Finish()

	appUUID := applicationtesting.GenApplicationUUID(c)
	s.applicationService.EXPECT().GetApplicationIDByName(gomock.Any(), appName).Return(appUUID, nil)
	s.applicationService.EXPECT().IsSubordinateApplication(gomock.Any(), appUUID).Return(false, nil)
	s.applicationService.EXPECT().GetApplicationConstraints(gomock.Any(), appUUID).Return(constraints.Value{}, nil)

	s.machineService.EXPECT().GetMachineArchesForApplication(gomock.Any(), appUUID).
		Return([]arch.Arch{arch.AMD64}, nil)

	api := s.api(c)

	args := params.ApplicationCharmPlacements{
		Placements: []params.ApplicationCharmPlacement{{
			Application: appName,
			CharmURL:    curl,
		}},
	}

	result, err := api.CheckCharmPlacement(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.OneError(), tc.ErrorIsNil)
}

func (s *charmsMockSuite) TestCheckCharmPlacementWithHeterogeneous(c *tc.C) {
	appName := "winnie"

	curl := "ch:poo"

	defer s.setupMocks(c).Finish()

	appUUID := applicationtesting.GenApplicationUUID(c)
	s.applicationService.EXPECT().GetApplicationIDByName(gomock.Any(), appName).Return(appUUID, nil)
	s.applicationService.EXPECT().IsSubordinateApplication(gomock.Any(), appUUID).Return(false, nil)
	s.applicationService.EXPECT().GetApplicationConstraints(gomock.Any(), appUUID).Return(constraints.Value{}, nil)

	s.machineService.EXPECT().GetMachineArchesForApplication(gomock.Any(), appUUID).
		Return([]arch.Arch{arch.AMD64, arch.ARM64}, nil)

	api := s.api(c)

	args := params.ApplicationCharmPlacements{
		Placements: []params.ApplicationCharmPlacement{{
			Application: appName,
			CharmURL:    curl,
		}},
	}

	result, err := api.CheckCharmPlacement(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.OneError(), tc.ErrorMatches, "charm can not be placed in a heterogeneous environment")
}

// NewCharmsAPI is only used for testing.
func NewCharmsAPI(
	authorizer facade.Authorizer,
	modelConfigService ModelConfigService,
	applicationService ApplicationService,
	machineService MachineService,
	controllerTag names.ControllerTag,
	modelTag names.ModelTag,
	repo corecharm.Repository,
	logger corelogger.Logger,
) (*API, error) {
	return &API{
		authorizer:         authorizer,
		modelConfigService: modelConfigService,
		applicationService: applicationService,
		machineService:     machineService,
		controllerTag:      controllerTag,
		modelTag:           modelTag,
		requestRecorder:    noopRequestRecorder{},
		newCharmHubRepository: func(repository.CharmHubRepositoryConfig) (corecharm.Repository, error) {
			return repo, nil
		},
		logger: logger,
	}, nil
}

func (s *charmsMockSuite) api(c *tc.C) *API {
	api, err := NewCharmsAPI(
		s.authorizer,
		s.modelConfigService,
		s.applicationService,
		s.machineService,
		names.NewControllerTag("deadbeef-abcd-4fd2-967d-db9663db7bef"),
		names.NewModelTag("deadbeef-abcd-4fd2-967d-db9663db7bea"),
		s.repository,
		loggertesting.WrapCheckLog(c),
	)
	c.Assert(err, tc.ErrorIsNil)
	return api
}

func (s *charmsMockSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.authorizer = apiservermocks.NewMockAuthorizer(ctrl)
	s.authorizer.EXPECT().HasPermission(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	s.repository = mocks.NewMockRepository(ctrl)
	s.charmArchive = mocks.NewMockCharmArchive(ctrl)

	s.modelConfigService = NewMockModelConfigService(ctrl)
	uuid := testing.ModelTag.Id()
	cfg, err := config.New(config.UseDefaults, map[string]interface{}{
		"name":         "model",
		"type":         "type",
		"uuid":         uuid,
		"charmhub-url": "https://api.staging.charmhub.io",
	})
	c.Assert(err, tc.ErrorIsNil)
	s.modelConfigService.EXPECT().ModelConfig(gomock.Any()).Return(cfg, nil).AnyTimes()

	s.applicationService = NewMockApplicationService(ctrl)
	s.machineService = NewMockMachineService(ctrl)

	return ctrl
}

func (s *charmsMockSuite) expectResolveWithPreferredChannel(times int, err error) {
	s.repository.EXPECT().ResolveWithPreferredChannel(
		gomock.Any(),
		gomock.AssignableToTypeOf(""),
		gomock.AssignableToTypeOf(corecharm.Origin{}),
	).DoAndReturn(
		func(_ context.Context, name string, requestedOrigin corecharm.Origin) (corecharm.ResolvedData, error) {
			resolvedOrigin := requestedOrigin
			resolvedOrigin.Type = "charm"
			resolvedOrigin.Platform = corecharm.Platform{
				Architecture: "amd64",
				OS:           "ubuntu",
				Channel:      "20.04",
			}

			if requestedOrigin.Channel == nil || requestedOrigin.Channel.Risk == "" {
				if requestedOrigin.Channel == nil {
					resolvedOrigin.Channel = new(charm.Channel)
				}

				resolvedOrigin.Channel.Risk = "stable"
			}
			bases := []corecharm.Platform{
				{OS: "ubuntu", Channel: "18.04"},
				{OS: "ubuntu", Channel: "20.04"},
				{OS: "ubuntu", Channel: "16.04"},
			}
			// ensure the charm url returned is filled out
			curl := &charm.URL{
				Schema:       "ch",
				Name:         name,
				Architecture: "amd64",
				Revision:     -1,
			}
			return corecharm.ResolvedData{
				URL:               curl,
				EssentialMetadata: corecharm.EssentialMetadata{},
				Origin:            resolvedOrigin,
				Platform:          bases,
			}, err
		}).Times(times)
}
