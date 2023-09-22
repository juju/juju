// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms_test

import (
	"net/url"

	"github.com/juju/charm/v11"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	apiservermocks "github.com/juju/juju/apiserver/facade/mocks"
	"github.com/juju/juju/apiserver/facades/client/charms"
	"github.com/juju/juju/apiserver/facades/client/charms/interfaces"
	charmsinterfaces "github.com/juju/juju/apiserver/facades/client/charms/interfaces"
	"github.com/juju/juju/apiserver/facades/client/charms/mocks"
	"github.com/juju/juju/apiserver/facades/client/charms/services"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/cache"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/constraints"
	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/multiwatcher"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing/factory"
)

type charmsSuite struct {
	// TODO(anastasiamac) mock to remove JujuConnSuite
	jujutesting.JujuConnSuite
	api  *charms.API
	auth facade.Authorizer
}

var _ = gc.Suite(&charmsSuite{})

// charmsSuiteContext implements the facade.Context interface.
type charmsSuiteContext struct{ cs *charmsSuite }

func (ctx *charmsSuiteContext) Abort() <-chan struct{}                        { return nil }
func (ctx *charmsSuiteContext) Auth() facade.Authorizer                       { return ctx.cs.auth }
func (ctx *charmsSuiteContext) Cancel() <-chan struct{}                       { return nil }
func (ctx *charmsSuiteContext) Dispose()                                      {}
func (ctx *charmsSuiteContext) Resources() facade.Resources                   { return common.NewResources() }
func (ctx *charmsSuiteContext) State() *state.State                           { return ctx.cs.State }
func (ctx *charmsSuiteContext) StatePool() *state.StatePool                   { return nil }
func (ctx *charmsSuiteContext) ID() string                                    { return "" }
func (ctx *charmsSuiteContext) RequestRecorder() facade.RequestRecorder       { return nil }
func (ctx *charmsSuiteContext) Presence() facade.Presence                     { return nil }
func (ctx *charmsSuiteContext) Hub() facade.Hub                               { return nil }
func (ctx *charmsSuiteContext) Controller() *cache.Controller                 { return nil }
func (ctx *charmsSuiteContext) CachedModel(uuid string) (*cache.Model, error) { return nil, nil }
func (ctx *charmsSuiteContext) MultiwatcherFactory() multiwatcher.Factory     { return nil }

func (ctx *charmsSuiteContext) LeadershipClaimer(string) (leadership.Claimer, error)  { return nil, nil }
func (ctx *charmsSuiteContext) LeadershipRevoker(string) (leadership.Revoker, error)  { return nil, nil }
func (ctx *charmsSuiteContext) LeadershipChecker() (leadership.Checker, error)        { return nil, nil }
func (ctx *charmsSuiteContext) LeadershipPinner(string) (leadership.Pinner, error)    { return nil, nil }
func (ctx *charmsSuiteContext) LeadershipReader(string) (leadership.Reader, error)    { return nil, nil }
func (ctx *charmsSuiteContext) SingularClaimer() (lease.Claimer, error)               { return nil, nil }
func (ctx *charmsSuiteContext) HTTPClient(facade.HTTPClientPurpose) facade.HTTPClient { return nil }
func (ctx *charmsSuiteContext) ControllerDB() (coredatabase.TrackedDB, error)         { return nil, nil }

func (s *charmsSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	s.auth = apiservertesting.FakeAuthorizer{
		Tag:        s.AdminUserTag(c),
		Controller: true,
	}

	var err error
	s.api, err = charms.NewFacade(&charmsSuiteContext{cs: s})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *charmsSuite) TestMeteredCharmInfo(c *gc.C) {
	meteredCharm := s.Factory.MakeCharm(
		c, &factory.CharmParams{Name: "metered", URL: "ch:amd64/xenial/metered"})
	info, err := s.api.CharmInfo(params.CharmURL{
		URL: meteredCharm.URL(),
	})
	c.Assert(err, jc.ErrorIsNil)
	expected := &params.CharmMetrics{
		Plan: params.CharmPlan{
			Required: true,
		},
		Metrics: map[string]params.CharmMetric{
			"pings": {
				Type:        "gauge",
				Description: "Description of the metric."},
			"pongs": {
				Type:        "gauge",
				Description: "Description of the metric."},
			"juju-units": {
				Type:        "",
				Description: ""}}}
	c.Assert(info.Metrics, jc.DeepEquals, expected)
}

func (s *charmsSuite) TestListCharmsNoFilter(c *gc.C) {
	s.assertListCharms(c, []string{"dummy"}, []string{}, []string{"local:quantal/dummy-1"})
}

func (s *charmsSuite) TestListCharmsWithFilterMatchingNone(c *gc.C) {
	s.assertListCharms(c, []string{"dummy"}, []string{"notdummy"}, []string{})
}

func (s *charmsSuite) TestListCharmsFilteredOnly(c *gc.C) {
	s.assertListCharms(c, []string{"dummy", "wordpress"}, []string{"dummy"}, []string{"local:quantal/dummy-1"})
}

func (s *charmsSuite) assertListCharms(c *gc.C, someCharms, args, expected []string) {
	for _, aCharm := range someCharms {
		s.AddTestingCharm(c, aCharm)
	}
	found, err := s.api.List(params.CharmsList{Names: args})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(found.CharmURLs, gc.HasLen, len(expected))
	c.Check(found.CharmURLs, jc.DeepEquals, expected)
}

func (s *charmsSuite) TestIsMeteredFalse(c *gc.C) {
	charm := s.Factory.MakeCharm(c, &factory.CharmParams{Name: "wordpress"})
	metered, err := s.api.IsMetered(params.CharmURL{
		URL: charm.URL(),
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metered.Metered, jc.IsFalse)
}

func (s *charmsSuite) TestIsMeteredTrue(c *gc.C) {
	meteredCharm := s.Factory.MakeCharm(c, &factory.CharmParams{Name: "metered", URL: "ch:amd64/quantal/metered"})
	metered, err := s.api.IsMetered(params.CharmURL{
		URL: meteredCharm.URL(),
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metered.Metered, jc.IsTrue)
}

type charmsMockSuite struct {
	model        *mocks.MockBackendModel
	state        *mocks.MockBackendState
	authorizer   *apiservermocks.MockAuthorizer
	repoFactory  *mocks.MockRepositoryFactory
	repository   *mocks.MockRepository
	charmArchive *mocks.MockCharmArchive
	downloader   *mocks.MockDownloader
	application  *mocks.MockApplication
	unit         *mocks.MockUnit
	unit2        *mocks.MockUnit
	machine      *mocks.MockMachine
	machine2     *mocks.MockMachine
}

var _ = gc.Suite(&charmsMockSuite{})

func (s *charmsMockSuite) TestResolveCharms(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectResolveWithPreferredChannel(3, nil)
	api := s.api(c)

	curl := "ch:testme"
	seriesCurl := "ch:amd64/focal/testme"

	edgeOrigin := params.CharmOrigin{
		Source:       corecharm.CharmHub.String(),
		Type:         "charm",
		Risk:         "edge",
		Architecture: "amd64",
	}
	stableOrigin := params.CharmOrigin{
		Source:       corecharm.CharmHub.String(),
		Type:         "charm",
		Risk:         "stable",
		Architecture: "amd64",
	}

	args := params.ResolveCharmsWithChannel{
		Resolve: []params.ResolveCharmWithChannel{
			{Reference: curl, Origin: params.CharmOrigin{
				Source:       corecharm.CharmHub.String(),
				Architecture: "amd64",
			}},
			{Reference: curl, Origin: stableOrigin},
			{Reference: seriesCurl, Origin: edgeOrigin},
		},
	}

	expected := []params.ResolveCharmWithChannelResult{
		{
			URL:    curl,
			Origin: stableOrigin,
			SupportedBases: []params.Base{
				{Name: "ubuntu", Channel: "18.04"},
				{Name: "ubuntu", Channel: "20.04"},
				{Name: "ubuntu", Channel: "16.04"},
			},
		}, {
			URL:    curl,
			Origin: stableOrigin,
			SupportedBases: []params.Base{
				{Name: "ubuntu", Channel: "18.04"},
				{Name: "ubuntu", Channel: "20.04"},
				{Name: "ubuntu", Channel: "16.04"},
			},
		},
		{
			URL:    seriesCurl,
			Origin: edgeOrigin,
			SupportedBases: []params.Base{
				{Name: "ubuntu", Channel: "18.04"},
				{Name: "ubuntu", Channel: "20.04"},
				{Name: "ubuntu", Channel: "16.04"},
			},
		},
	}
	result, err := api.ResolveCharms(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 3)
	c.Assert(result.Results, jc.DeepEquals, expected)
}

func (s *charmsMockSuite) TestResolveCharmsUnknownSchema(c *gc.C) {
	defer s.setupMocks(c).Finish()
	api := s.api(c)

	curl, err := charm.ParseURL("local:testme")
	c.Assert(err, jc.ErrorIsNil)
	args := params.ResolveCharmsWithChannel{
		Resolve: []params.ResolveCharmWithChannel{{Reference: curl.String()}},
	}

	result, err := api.ResolveCharms(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.ErrorMatches, `unknown schema for charm URL "local:testme"`)
}

func (s *charmsMockSuite) TestResolveCharmNoDefinedSeries(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectResolveWithPreferredChannelNoSeries()
	api := s.api(c)

	seriesCurl := "ch:focal/testme"

	edgeOrigin := params.CharmOrigin{
		Source:       corecharm.CharmHub.String(),
		Type:         "charm",
		Risk:         "edge",
		Architecture: "amd64",
	}

	args := params.ResolveCharmsWithChannel{
		Resolve: []params.ResolveCharmWithChannel{
			{Reference: seriesCurl, Origin: edgeOrigin},
		},
	}

	expected := []params.ResolveCharmWithChannelResult{{
		URL:            seriesCurl,
		Origin:         edgeOrigin,
		SupportedBases: []params.Base{{Name: "ubuntu", Channel: "20.04/stable"}},
	}}
	result, err := api.ResolveCharms(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results, jc.DeepEquals, expected)
}

func (s *charmsMockSuite) TestResolveCharmV6(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectResolveWithPreferredChannel(3, nil)
	apiv6 := charms.APIv6{
		&charms.APIv7{
			API: s.api(c),
		},
	}

	curl := "ch:testme"
	seriesCurl := "ch:amd64/focal/testme"

	edgeOrigin := params.CharmOrigin{
		Source:       corecharm.CharmHub.String(),
		Type:         "charm",
		Risk:         "edge",
		Architecture: "amd64",
	}
	stableOrigin := params.CharmOrigin{
		Source:       corecharm.CharmHub.String(),
		Type:         "charm",
		Risk:         "stable",
		Architecture: "amd64",
	}

	args := params.ResolveCharmsWithChannel{
		Resolve: []params.ResolveCharmWithChannel{
			{Reference: curl, Origin: params.CharmOrigin{
				Source:       corecharm.CharmHub.String(),
				Architecture: "amd64",
			}},
			{Reference: curl, Origin: stableOrigin},
			{Reference: seriesCurl, Origin: edgeOrigin},
		},
	}

	expected := []params.ResolveCharmWithChannelResultV6{
		{
			URL:             curl,
			Origin:          stableOrigin,
			SupportedSeries: []string{"bionic", "focal", "xenial"},
		}, {
			URL:             curl,
			Origin:          stableOrigin,
			SupportedSeries: []string{"bionic", "focal", "xenial"},
		},
		{
			URL:             seriesCurl,
			Origin:          edgeOrigin,
			SupportedSeries: []string{"bionic", "focal", "xenial"},
		},
	}
	result, err := apiv6.ResolveCharms(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 3)
	c.Assert(result.Results, jc.DeepEquals, expected)
}

func (s *charmsMockSuite) TestAddCharmWithLocalSource(c *gc.C) {
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
	_, err := api.AddCharm(args)
	c.Assert(err, gc.ErrorMatches, `unknown schema for charm URL "local:testme"`)
}

func (s *charmsMockSuite) TestAddCharmCharmhub(c *gc.C) {
	// Charmhub charms are downloaded asynchronously
	defer s.setupMocks(c).Finish()

	curl, err := charm.ParseURL("chtest")
	c.Assert(err, jc.ErrorIsNil)

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

	s.state.EXPECT().Charm(curl.String()).Return(nil, errors.NotFoundf("%q", curl))
	s.repoFactory.EXPECT().GetCharmRepository(gomock.Any()).Return(s.repository, nil)

	expMeta := new(charm.Meta)
	expManifest := new(charm.Manifest)
	expConfig := new(charm.Config)
	s.repository.EXPECT().GetEssentialMetadata(corecharm.MetadataRequest{
		CharmURL: curl,
		Origin:   requestedOrigin,
	}).Return([]corecharm.EssentialMetadata{
		{
			Meta:           expMeta,
			Manifest:       expManifest,
			Config:         expConfig,
			ResolvedOrigin: resolvedOrigin,
		},
	}, nil)

	s.state.EXPECT().AddCharmMetadata(gomock.Any()).DoAndReturn(
		func(ci state.CharmInfo) (*state.Charm, error) {
			c.Assert(ci.ID, gc.DeepEquals, curl.String())
			// Check that the essential metadata matches what
			// the repository returned. We use pointer checks here.
			c.Assert(ci.Charm.Meta(), gc.Equals, expMeta)
			c.Assert(ci.Charm.Manifest(), gc.Equals, expManifest)
			c.Assert(ci.Charm.Config(), gc.Equals, expConfig)
			return nil, nil
		},
	)

	api := s.api(c)

	args := params.AddCharmWithOrigin{
		URL: curl.String(),
		Origin: params.CharmOrigin{
			Source: "charm-hub",
			Base:   params.Base{Name: "ubuntu", Channel: "20.04/stable"},
			Risk:   "edge",
		},
	}
	obtained, err := api.AddCharm(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtained, gc.DeepEquals, params.CharmOriginResult{
		Origin: params.CharmOrigin{
			Source: "charm-hub",
			Base:   params.Base{Name: "ubuntu", Channel: "20.04/stable"},
			Risk:   "stable",
		},
	})
}

func (s *charmsMockSuite) TestQueueAsyncCharmDownloadResolvesAgainOriginForAlreadyDownloadedCharm(c *gc.C) {
	defer s.setupMocks(c).Finish()

	curl, err := charm.ParseURL("chtest")
	c.Assert(err, jc.ErrorIsNil)
	resURL, err := url.Parse(curl.String())
	c.Assert(err, jc.ErrorIsNil)

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

	s.state.EXPECT().Charm(curl.String()).Return(nil, nil) // a nil error indicates that the charm doc already exists
	s.repoFactory.EXPECT().GetCharmRepository(gomock.Any()).Return(s.repository, nil)
	s.repository.EXPECT().GetDownloadURL(curl, gomock.Any()).Return(resURL, resolvedOrigin, nil)

	api := s.api(c)

	args := params.AddCharmWithOrigin{
		URL: curl.String(),
		Origin: params.CharmOrigin{
			Source: "charm-hub",
			Risk:   "edge",
			Base:   params.Base{Name: "ubuntu", Channel: "20.04/stable"},
		},
		Force: false,
	}
	obtained, err := api.AddCharm(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtained, gc.DeepEquals, params.CharmOriginResult{
		Origin: params.CharmOrigin{
			Source: "charm-hub",
			Risk:   "stable",
			Base:   params.Base{Name: "ubuntu", Channel: "20.04/stable"},
		},
	}, gc.Commentf("expected to get back the origin recorded by the application"))
}

func (s *charmsMockSuite) TestCheckCharmPlacementWithSubordinate(c *gc.C) {
	appName := "winnie"

	curl := "ch:poo"

	defer s.setupMocks(c).Finish()
	s.expectSubordinateApplication(appName)

	api := s.api(c)

	args := params.ApplicationCharmPlacements{
		Placements: []params.ApplicationCharmPlacement{{
			Application: appName,
			CharmURL:    curl,
		}},
	}

	result, err := api.CheckCharmPlacement(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.OneError(), jc.ErrorIsNil)
}

func (s *charmsMockSuite) TestCheckCharmPlacementWithConstraintArch(c *gc.C) {
	arch := arch.DefaultArchitecture
	appName := "winnie"

	curl := "ch:poo"

	defer s.setupMocks(c).Finish()
	s.expectApplication(appName)
	s.expectApplicationConstraints(constraints.Value{Arch: &arch})

	api := s.api(c)

	args := params.ApplicationCharmPlacements{
		Placements: []params.ApplicationCharmPlacement{{
			Application: appName,
			CharmURL:    curl,
		}},
	}

	result, err := api.CheckCharmPlacement(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.OneError(), jc.ErrorIsNil)
}

func (s *charmsMockSuite) TestCheckCharmPlacementWithNoConstraintArch(c *gc.C) {
	appName := "winnie"

	curl := "ch:poo"

	defer s.setupMocks(c).Finish()
	s.expectApplication(appName)
	s.expectApplicationConstraints(constraints.Value{})
	s.expectAllUnits()
	s.expectUnitMachineID()
	s.expectMachine()
	s.expectMachineConstraints(constraints.Value{})
	s.expectHardwareCharacteristics()

	api := s.api(c)

	args := params.ApplicationCharmPlacements{
		Placements: []params.ApplicationCharmPlacement{{
			Application: appName,
			CharmURL:    curl,
		}},
	}

	result, err := api.CheckCharmPlacement(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.OneError(), jc.ErrorIsNil)
}

func (s *charmsMockSuite) TestCheckCharmPlacementWithNoConstraintArchMachine(c *gc.C) {
	arch := arch.DefaultArchitecture
	appName := "winnie"

	curl := "ch:poo"

	defer s.setupMocks(c).Finish()
	s.expectApplication(appName)
	s.expectApplicationConstraints(constraints.Value{})
	s.expectAllUnits()
	s.expectUnitMachineID()
	s.expectMachine()
	s.expectMachineConstraints(constraints.Value{Arch: &arch})

	api := s.api(c)

	args := params.ApplicationCharmPlacements{
		Placements: []params.ApplicationCharmPlacement{{
			Application: appName,
			CharmURL:    curl,
		}},
	}

	result, err := api.CheckCharmPlacement(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.OneError(), jc.ErrorIsNil)
}

func (s *charmsMockSuite) TestCheckCharmPlacementWithNoConstraintArchAndHardwareArch(c *gc.C) {
	appName := "winnie"

	curl := "ch:poo"

	defer s.setupMocks(c).Finish()
	s.expectApplication(appName)
	s.expectApplicationConstraints(constraints.Value{})
	s.expectAllUnits()
	s.expectUnitMachineID()
	s.expectMachine()
	s.expectMachineConstraints(constraints.Value{})
	s.expectEmptyHardwareCharacteristics()

	api := s.api(c)

	args := params.ApplicationCharmPlacements{
		Placements: []params.ApplicationCharmPlacement{{
			Application: appName,
			CharmURL:    curl,
		}},
	}

	result, err := api.CheckCharmPlacement(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.OneError(), jc.ErrorIsNil)
}

func (s *charmsMockSuite) TestCheckCharmPlacementWithHeterogeneous(c *gc.C) {
	appName := "winnie"

	curl := "ch:poo"

	defer s.setupMocks(c).Finish()
	s.expectApplication(appName)
	s.expectApplicationConstraints(constraints.Value{})
	s.expectHeterogeneousUnits()

	s.expectUnitMachineID()
	s.expectMachine()
	s.expectMachineConstraints(constraints.Value{})
	s.expectHardwareCharacteristics()

	s.expectUnit2MachineID()
	s.expectMachine2()
	s.expectMachineConstraints2(constraints.Value{})
	s.expectHardwareCharacteristics2()

	api := s.api(c)

	args := params.ApplicationCharmPlacements{
		Placements: []params.ApplicationCharmPlacement{{
			Application: appName,
			CharmURL:    curl,
		}},
	}

	result, err := api.CheckCharmPlacement(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.OneError(), gc.ErrorMatches, "charm can not be placed in a heterogeneous environment")
}

func (s *charmsMockSuite) api(c *gc.C) *charms.API {
	api, err := charms.NewCharmsAPI(
		s.authorizer,
		s.state,
		s.model,
		nil,
		s.repoFactory,
		func(services.CharmDownloaderConfig) (charmsinterfaces.Downloader, error) {
			return s.downloader, nil
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	return api
}

func (s *charmsMockSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.authorizer = apiservermocks.NewMockAuthorizer(ctrl)
	s.authorizer.EXPECT().HasPermission(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	s.model = mocks.NewMockBackendModel(ctrl)
	s.model.EXPECT().ModelTag().Return(names.NewModelTag("deadbeef-abcd-4fd2-967d-db9663db7bea")).AnyTimes()

	s.state = mocks.NewMockBackendState(ctrl)
	s.state.EXPECT().ControllerTag().Return(names.NewControllerTag("deadbeef-abcd-dead-beef-db9663db7b42")).AnyTimes()
	s.state.EXPECT().ModelUUID().Return("deadbeef-abcd-dead-beef-db9663db7b42").AnyTimes()

	s.repoFactory = mocks.NewMockRepositoryFactory(ctrl)
	s.repository = mocks.NewMockRepository(ctrl)
	s.charmArchive = mocks.NewMockCharmArchive(ctrl)
	s.downloader = mocks.NewMockDownloader(ctrl)

	s.application = mocks.NewMockApplication(ctrl)
	s.unit = mocks.NewMockUnit(ctrl)
	s.unit2 = mocks.NewMockUnit(ctrl)
	s.machine = mocks.NewMockMachine(ctrl)
	s.machine2 = mocks.NewMockMachine(ctrl)

	return ctrl
}

func (s *charmsMockSuite) expectResolveWithPreferredChannel(times int, err error) {
	s.repoFactory.EXPECT().GetCharmRepository(gomock.Any()).Return(s.repository, nil).Times(times)
	s.repository.EXPECT().ResolveWithPreferredChannel(
		gomock.AssignableToTypeOf(&charm.URL{}),
		gomock.AssignableToTypeOf(corecharm.Origin{}),
	).DoAndReturn(
		// Ensure the same curl that is provided, is returned.
		func(curl *charm.URL, requestedOrigin corecharm.Origin) (*charm.URL, corecharm.Origin, []corecharm.Platform, error) {
			resolvedOrigin := requestedOrigin
			resolvedOrigin.Type = "charm"

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
			return curl, resolvedOrigin, bases, err
		}).Times(times)
}

func (s *charmsMockSuite) expectResolveWithPreferredChannelNoSeries() {
	s.repoFactory.EXPECT().GetCharmRepository(gomock.Any()).Return(s.repository, nil)
	s.repository.EXPECT().ResolveWithPreferredChannel(
		gomock.AssignableToTypeOf(&charm.URL{}),
		gomock.AssignableToTypeOf(corecharm.Origin{}),
	).DoAndReturn(
		// Ensure the same curl that is provided, is returned.
		func(curl *charm.URL, requestedOrigin corecharm.Origin) (*charm.URL, corecharm.Origin, []string, error) {
			resolvedOrigin := requestedOrigin
			resolvedOrigin.Type = "charm"

			if requestedOrigin.Channel == nil || requestedOrigin.Channel.Risk == "" {
				if requestedOrigin.Channel == nil {
					resolvedOrigin.Channel = new(charm.Channel)
				}

				resolvedOrigin.Channel.Risk = "stable"
			}

			return curl, resolvedOrigin, []string{}, nil
		})
}

func (s *charmsMockSuite) expectApplication(name string) {
	s.state.EXPECT().Application(name).Return(s.application, nil)
	s.application.EXPECT().IsPrincipal().Return(true)
}

func (s *charmsMockSuite) expectSubordinateApplication(name string) {
	s.state.EXPECT().Application(name).Return(s.application, nil)
	s.application.EXPECT().IsPrincipal().Return(false)
}

func (s *charmsMockSuite) expectApplicationConstraints(cons constraints.Value) {
	s.application.EXPECT().Constraints().Return(cons, nil)
}

func (s *charmsMockSuite) expectAllUnits() {
	s.application.EXPECT().AllUnits().Return([]interfaces.Unit{s.unit}, nil)
}

func (s *charmsMockSuite) expectHeterogeneousUnits() {
	s.application.EXPECT().AllUnits().Return([]interfaces.Unit{
		s.unit,
		s.unit2,
	}, nil)
}

func (s *charmsMockSuite) expectUnitMachineID() {
	s.unit.EXPECT().AssignedMachineId().Return("winnie-poo", nil)
}

func (s *charmsMockSuite) expectUnit2MachineID() {
	s.unit2.EXPECT().AssignedMachineId().Return("piglet", nil)
}

func (s *charmsMockSuite) expectMachine() {
	s.state.EXPECT().Machine("winnie-poo").Return(s.machine, nil)
}

func (s *charmsMockSuite) expectMachine2() {
	s.state.EXPECT().Machine("piglet").Return(s.machine2, nil)
}

func (s *charmsMockSuite) expectMachineConstraints(cons constraints.Value) {
	s.machine.EXPECT().Constraints().Return(cons, nil)
}

func (s *charmsMockSuite) expectHardwareCharacteristics() {
	arch := arch.DefaultArchitecture
	s.machine.EXPECT().HardwareCharacteristics().Return(&instance.HardwareCharacteristics{
		Arch: &arch,
	}, nil)
}

func (s *charmsMockSuite) expectEmptyHardwareCharacteristics() {
	s.machine.EXPECT().HardwareCharacteristics().Return(&instance.HardwareCharacteristics{}, nil)
}

func (s *charmsMockSuite) expectHardwareCharacteristics2() {
	arch := "arm64"
	s.machine2.EXPECT().HardwareCharacteristics().Return(&instance.HardwareCharacteristics{
		Arch: &arch,
	}, nil)
}

func (s *charmsMockSuite) expectMachineConstraints2(cons constraints.Value) {
	s.machine2.EXPECT().Constraints().Return(cons, nil)
}
