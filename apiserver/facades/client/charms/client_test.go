// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/charm/v9"
	csparams "github.com/juju/charmrepo/v7/csclient/params"
	"github.com/juju/juju/state/errors"
	"github.com/juju/mgo/v2"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	apiservermocks "github.com/juju/juju/apiserver/facade/mocks"
	"github.com/juju/juju/apiserver/facades/client/charms"
	"github.com/juju/juju/apiserver/facades/client/charms/interfaces"
	"github.com/juju/juju/apiserver/facades/client/charms/mocks"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/cache"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/multiwatcher"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/storage"
	"github.com/juju/juju/testing"
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

func (ctx *charmsSuiteContext) LeadershipClaimer(string) (leadership.Claimer, error) { return nil, nil }
func (ctx *charmsSuiteContext) LeadershipRevoker(string) (leadership.Revoker, error) { return nil, nil }
func (ctx *charmsSuiteContext) LeadershipChecker() (leadership.Checker, error)       { return nil, nil }
func (ctx *charmsSuiteContext) LeadershipPinner(string) (leadership.Pinner, error)   { return nil, nil }
func (ctx *charmsSuiteContext) LeadershipReader(string) (leadership.Reader, error)   { return nil, nil }
func (ctx *charmsSuiteContext) SingularClaimer() (lease.Claimer, error)              { return nil, nil }

func (s *charmsSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	s.auth = apiservertesting.FakeAuthorizer{
		Tag:        s.AdminUserTag(c),
		Controller: true,
	}

	var err error
	s.api, err = charms.NewFacadeV4(&charmsSuiteContext{cs: s})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *charmsSuite) TestMeteredCharmInfo(c *gc.C) {
	meteredCharm := s.Factory.MakeCharm(
		c, &factory.CharmParams{Name: "metered", URL: "cs:xenial/metered"})
	info, err := s.api.CharmInfo(params.CharmURL{
		URL: meteredCharm.URL().String(),
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
		URL: charm.URL().String(),
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metered.Metered, jc.IsFalse)
}

func (s *charmsSuite) TestIsMeteredTrue(c *gc.C) {
	meteredCharm := s.Factory.MakeCharm(c, &factory.CharmParams{Name: "metered", URL: "cs:quantal/metered"})
	metered, err := s.api.IsMetered(params.CharmURL{
		URL: meteredCharm.URL().String(),
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metered.Metered, jc.IsTrue)
}

type charmsMockSuite struct {
	model       *mocks.MockBackendModel
	state       *mocks.MockBackendState
	authorizer  *apiservermocks.MockAuthorizer
	repository  *mocks.MockCSRepository
	strategy    *mocks.MockStrategy
	charm       *mocks.MockStoreCharm
	storage     *mocks.MockStorage
	application *mocks.MockApplication
	unit        *mocks.MockUnit
	unit2       *mocks.MockUnit
	machine     *mocks.MockMachine
	machine2    *mocks.MockMachine
}

var _ = gc.Suite(&charmsMockSuite{})

func (s *charmsMockSuite) TestResolveCharms(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectResolveWithPreferredChannel(3, nil)
	s.expectControllerConfig(c)
	api := s.api(c)

	curl, err := charm.ParseURL("cs:testme")
	c.Assert(err, jc.ErrorIsNil)
	seriesCurl, err := charm.ParseURL("cs:focal/testme")
	c.Assert(err, jc.ErrorIsNil)

	edge := string(csparams.EdgeChannel)
	stable := string(csparams.StableChannel)
	edgeOrigin := params.CharmOrigin{
		Source:       corecharm.CharmStore.String(),
		Type:         "charm",
		Risk:         edge,
		Architecture: "amd64",
	}
	stableOrigin := params.CharmOrigin{
		Source:       corecharm.CharmStore.String(),
		Type:         "charm",
		Risk:         stable,
		Architecture: "amd64",
	}

	args := params.ResolveCharmsWithChannel{
		Resolve: []params.ResolveCharmWithChannel{
			{Reference: curl.String(), Origin: params.CharmOrigin{
				Source:       corecharm.CharmStore.String(),
				Architecture: "amd64",
			}},
			{Reference: curl.String(), Origin: stableOrigin},
			{Reference: seriesCurl.String(), Origin: edgeOrigin},
		},
	}

	expected := []params.ResolveCharmWithChannelResult{
		{
			URL:             curl.String(),
			Origin:          stableOrigin,
			SupportedSeries: []string{"bionic", "focal", "xenial"},
		}, {
			URL:             curl.String(),
			Origin:          stableOrigin,
			SupportedSeries: []string{"bionic", "focal", "xenial"},
		},
		{
			URL:             seriesCurl.String(),
			Origin:          edgeOrigin,
			SupportedSeries: []string{"bionic", "focal", "xenial"},
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
	s.expectControllerConfig(c)
	api := s.api(c)

	seriesCurl, err := charm.ParseURL("cs:focal/testme")
	c.Assert(err, jc.ErrorIsNil)

	edge := string(csparams.EdgeChannel)
	edgeOrigin := params.CharmOrigin{
		Source:       corecharm.CharmStore.String(),
		Type:         "charm",
		Risk:         edge,
		Architecture: "amd64",
	}

	args := params.ResolveCharmsWithChannel{
		Resolve: []params.ResolveCharmWithChannel{
			{Reference: seriesCurl.String(), Origin: edgeOrigin},
		},
	}

	expected := []params.ResolveCharmWithChannelResult{{
		URL:             seriesCurl.String(),
		Origin:          edgeOrigin,
		SupportedSeries: []string{"focal"},
	}}
	result, err := api.ResolveCharms(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results, jc.DeepEquals, expected)
}

func (s *charmsMockSuite) TestAddCharmWithLocalSource(c *gc.C) {
	defer s.setupMocks(c).Finish()
	api := s.api(c)

	curl, err := charm.ParseURL("local:testme")
	c.Assert(err, jc.ErrorIsNil)
	args := params.AddCharmWithOrigin{
		URL: curl.String(),
		Origin: params.CharmOrigin{
			Source: "local",
		},
		Force: false,
	}
	_, err = api.AddCharm(args)
	c.Assert(err, gc.ErrorMatches, `unknown schema for charm URL "local:testme"`)
}

func (s *charmsMockSuite) TestAddCharm(c *gc.C) {
	curl, err := charm.ParseURL("cs:testme-8")
	c.Assert(err, jc.ErrorIsNil)

	defer s.setupMocks(c).Finish()
	s.expectControllerConfig(c)
	s.expectValidate()
	s.expectRun(corecharm.DownloadResult{
		Charm: s.charm,
	}, false, nil)
	s.expectFinish()
	s.expectCharmURL(curl)
	s.expectVersion()
	s.expectMongoSession()
	s.expectPut()
	s.expectUpdateUploadedCharm(nil)

	api := s.api(c)

	args := params.AddCharmWithOrigin{
		URL: curl.String(),
		Origin: params.CharmOrigin{
			Source: "charm-store",
		},
		Force: false,
	}
	obtained, err := api.AddCharm(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtained, gc.DeepEquals, params.CharmOriginResult{
		Origin: params.CharmOrigin{Source: "charm-store"},
	})
}

func (s *charmsMockSuite) TestAddCharmRemoveIfUpdateFails(c *gc.C) {
	curl, err := charm.ParseURL("cs:testme-8")
	c.Assert(err, jc.ErrorIsNil)

	defer s.setupMocks(c).Finish()
	s.expectControllerConfig(c)
	s.expectValidate()
	s.expectRun(corecharm.DownloadResult{
		Charm: s.charm,
	}, false, nil)
	s.expectFinish()
	s.expectCharmURL(curl)
	s.expectVersion()
	s.expectMongoSession()
	s.expectPut()
	s.expectUpdateUploadedCharm(errors.NewErrCharmAlreadyUploaded(curl))
	s.expectRemove()

	api := s.api(c)

	args := params.AddCharmWithOrigin{
		URL: curl.String(),
		Origin: params.CharmOrigin{
			Source: "charm-store",
		},
		Force: false,
	}
	obtained, err := api.AddCharm(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtained, gc.DeepEquals, params.CharmOriginResult{
		Origin: params.CharmOrigin{Source: "charm-store"},
	})
}

func (s *charmsMockSuite) TestAddCharmAlreadyExists(c *gc.C) {
	curl, err := charm.ParseURL("cs:testme-8")
	c.Assert(err, jc.ErrorIsNil)

	defer s.setupMocks(c).Finish()
	s.expectControllerConfig(c)
	s.expectValidate()
	s.expectRun(corecharm.DownloadResult{}, true, nil)
	s.expectFinish()
	api := s.api(c)

	args := params.AddCharmWithOrigin{
		URL: curl.String(),
		Origin: params.CharmOrigin{
			Source: "charm-store",
		},
		Force: false,
	}
	obtained, err := api.AddCharm(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtained, gc.DeepEquals, params.CharmOriginResult{Origin: params.CharmOrigin{Source: "charm-store"}})
}

func (s *charmsMockSuite) TestAddCharmWithAuthorization(c *gc.C) {
	curl, err := charm.ParseURL("cs:testme-8")
	c.Assert(err, jc.ErrorIsNil)

	defer s.setupMocks(c).Finish()
	s.expectControllerConfig(c)
	s.expectValidate()
	s.expectRun(corecharm.DownloadResult{
		Charm: s.charm,
	}, false, nil)
	s.expectFinish()
	s.expectCharmURL(curl)
	s.expectVersion()
	s.expectMongoSession()
	s.expectPut()
	s.expectUpdateUploadedCharm(nil)

	api := s.api(c)

	args := params.AddCharmWithAuth{
		URL: curl.String(),
		Origin: params.CharmOrigin{
			Source: "charm-store",
		},
		Force: false,
	}
	obtained, err := api.AddCharmWithAuthorization(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtained, gc.DeepEquals, params.CharmOriginResult{
		Origin: params.CharmOrigin{Source: "charm-store"},
	})
}

func (s *charmsMockSuite) TestCheckCharmPlacementWithSubordinate(c *gc.C) {
	appName := "winnie"

	curl, err := charm.ParseURL("ch:poo")
	c.Assert(err, jc.ErrorIsNil)

	defer s.setupMocks(c).Finish()
	s.expectSubordinateApplication(appName)

	api := s.api(c)

	args := params.ApplicationCharmPlacements{
		Placements: []params.ApplicationCharmPlacement{{
			Application: appName,
			CharmURL:    curl.String(),
		}},
	}

	result, err := api.CheckCharmPlacement(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.OneError(), jc.ErrorIsNil)
}

func (s *charmsMockSuite) TestCheckCharmPlacementWithConstraintArch(c *gc.C) {
	arch := arch.DefaultArchitecture
	appName := "winnie"

	curl, err := charm.ParseURL("ch:poo")
	c.Assert(err, jc.ErrorIsNil)

	defer s.setupMocks(c).Finish()
	s.expectApplication(appName)
	s.expectApplicationConstraints(constraints.Value{Arch: &arch})

	api := s.api(c)

	args := params.ApplicationCharmPlacements{
		Placements: []params.ApplicationCharmPlacement{{
			Application: appName,
			CharmURL:    curl.String(),
		}},
	}

	result, err := api.CheckCharmPlacement(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.OneError(), jc.ErrorIsNil)
}

func (s *charmsMockSuite) TestCheckCharmPlacementWithNoConstraintArch(c *gc.C) {
	appName := "winnie"

	curl, err := charm.ParseURL("ch:poo")
	c.Assert(err, jc.ErrorIsNil)

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
			CharmURL:    curl.String(),
		}},
	}

	result, err := api.CheckCharmPlacement(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.OneError(), jc.ErrorIsNil)
}

func (s *charmsMockSuite) TestCheckCharmPlacementWithNoConstraintArchMachine(c *gc.C) {
	arch := arch.DefaultArchitecture
	appName := "winnie"

	curl, err := charm.ParseURL("ch:poo")
	c.Assert(err, jc.ErrorIsNil)

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
			CharmURL:    curl.String(),
		}},
	}

	result, err := api.CheckCharmPlacement(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.OneError(), jc.ErrorIsNil)
}

func (s *charmsMockSuite) TestCheckCharmPlacementWithNoConstraintArchAndHardwareArch(c *gc.C) {
	appName := "winnie"

	curl, err := charm.ParseURL("ch:poo")
	c.Assert(err, jc.ErrorIsNil)

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
			CharmURL:    curl.String(),
		}},
	}

	result, err := api.CheckCharmPlacement(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.OneError(), jc.ErrorIsNil)
}

func (s *charmsMockSuite) TestCheckCharmPlacementWithHeterogeneous(c *gc.C) {
	appName := "winnie"

	curl, err := charm.ParseURL("ch:poo")
	c.Assert(err, jc.ErrorIsNil)

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
			CharmURL:    curl.String(),
		}},
	}

	result, err := api.CheckCharmPlacement(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.OneError(), gc.ErrorMatches, "charm can not be placed in a heterogeneous environment")
}

func (s *charmsMockSuite) api(c *gc.C) *charms.API {
	repoFunc := func(_ charms.ResolverGetterParams) (charms.CSRepository, error) {
		return s.repository, nil
	}
	stratFuc := func(source string) charms.StrategyFunc {
		return func(charmRepo corecharm.Repository, url string, force bool) (charms.Strategy, error) {
			return s.strategy, nil
		}
	}
	storageFunc := func(modelUUID string, session *mgo.Session) storage.Storage {
		return s.storage
	}

	api, err := charms.NewCharmsAPI(
		s.authorizer,
		s.state,
		s.model,
		repoFunc,
		stratFuc,
		storageFunc,
	)
	c.Assert(err, jc.ErrorIsNil)
	return api
}

func (s *charmsMockSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.authorizer = apiservermocks.NewMockAuthorizer(ctrl)
	s.authorizer.EXPECT().HasPermission(gomock.Any(), gomock.Any()).Return(true, nil).AnyTimes()

	s.model = mocks.NewMockBackendModel(ctrl)
	s.model.EXPECT().ModelTag().Return(names.NewModelTag("deadbeef-abcd-4fd2-967d-db9663db7bea")).AnyTimes()

	s.state = mocks.NewMockBackendState(ctrl)
	s.state.EXPECT().ControllerTag().Return(names.NewControllerTag("deadbeef-abcd-dead-beef-db9663db7b42")).AnyTimes()
	s.state.EXPECT().ModelUUID().Return("deadbeef-abcd-dead-beef-db9663db7b42").AnyTimes()

	s.repository = mocks.NewMockCSRepository(ctrl)
	s.strategy = mocks.NewMockStrategy(ctrl)
	s.charm = mocks.NewMockStoreCharm(ctrl)
	s.storage = mocks.NewMockStorage(ctrl)

	s.application = mocks.NewMockApplication(ctrl)
	s.unit = mocks.NewMockUnit(ctrl)
	s.unit2 = mocks.NewMockUnit(ctrl)
	s.machine = mocks.NewMockMachine(ctrl)
	s.machine2 = mocks.NewMockMachine(ctrl)

	return ctrl
}

func (s *charmsMockSuite) expectResolveWithPreferredChannel(times int, err error) {
	s.repository.EXPECT().ResolveWithPreferredChannel(
		gomock.AssignableToTypeOf(&charm.URL{}),
		gomock.Any(),
	).DoAndReturn(
		// Ensure the same curl that is provided, is returned.
		func(curl *charm.URL, channel csparams.Channel) (*charm.URL, csparams.Channel, []string, error) {
			if channel == csparams.NoChannel {
				// minor attempt at mimicking charmrepo/charmstore.go.bestChannel()
				channel = csparams.StableChannel
			}
			return curl, channel, []string{"bionic", "focal", "xenial"}, err
		}).Times(times)
}

func (s *charmsMockSuite) expectResolveWithPreferredChannelNoSeries() {
	s.repository.EXPECT().ResolveWithPreferredChannel(
		gomock.AssignableToTypeOf(&charm.URL{}),
		gomock.Any(),
	).DoAndReturn(
		// Ensure the same curl that is provided, is returned.
		func(curl *charm.URL, channel csparams.Channel) (*charm.URL, csparams.Channel, []string, error) {
			if channel == csparams.NoChannel {
				// minor attempt at mimicing charmrepo/charmstore.go.bestChannel()
				channel = csparams.StableChannel
			}
			return curl, channel, []string{}, nil
		})
}

func (s *charmsMockSuite) expectControllerConfig(c *gc.C) {
	cfg, err := controller.NewConfig("deadbeef-1bad-500d-9000-4b1d0d06f00d", testing.CACert,
		map[string]interface{}{
			controller.CharmStoreURL: "http://www.testme.com",
		})
	c.Assert(err, jc.ErrorIsNil)
	s.state.EXPECT().ControllerConfig().Return(cfg, nil).AnyTimes()
}

func (s *charmsMockSuite) expectFinish() {
	s.strategy.EXPECT().Finish().Return(nil)
}

func (s *charmsMockSuite) expectRun(download corecharm.DownloadResult, already bool, err error) {
	s.strategy.EXPECT().Run(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(corecharm.Origin{})).DoAndReturn(
		func(_ corecharm.State, _ corecharm.JujuVersionValidator, origin corecharm.Origin) (corecharm.DownloadResult, bool, corecharm.Origin, error) {
			return download, already, origin, err
		},
	)
}

func (s *charmsMockSuite) expectValidate() {
	s.strategy.EXPECT().Validate().Return(nil)
}

func (s *charmsMockSuite) expectCharmURL(curl *charm.URL) {
	s.strategy.EXPECT().CharmURL().Return(curl)
}

func (s *charmsMockSuite) expectVersion() {
	s.charm.EXPECT().Version().Return("1.42")
}

func (s *charmsMockSuite) expectMongoSession() {
	s.state.EXPECT().MongoSession().Return(&mgo.Session{})
}

func (s *charmsMockSuite) expectPut() {
	s.storage.EXPECT().Put(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
}

func (s *charmsMockSuite) expectRemove() {
	s.storage.EXPECT().Remove(gomock.Any()).Return(nil)
}

func (s *charmsMockSuite) expectUpdateUploadedCharm(err error) {
	s.state.EXPECT().UpdateUploadedCharm(gomock.Any()).Return(nil, err)
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
