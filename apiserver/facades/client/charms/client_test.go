// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms_test

import (
	"context"
	"fmt"
	"net/url"

	"github.com/juju/charm/v13"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/facade/facadetest"
	apiservermocks "github.com/juju/juju/apiserver/facade/mocks"
	"github.com/juju/juju/apiserver/facades/client/charms"
	"github.com/juju/juju/apiserver/facades/client/charms/interfaces"
	"github.com/juju/juju/apiserver/facades/client/charms/mocks"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/arch"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing/factory"
)

type charmsSuite struct {
	// TODO(anastasiamac) mock to remove ApiServerSuite
	jujutesting.ApiServerSuite
	api  *charms.API
	auth facade.Authorizer
}

var _ = gc.Suite(&charmsSuite{})

func (s *charmsSuite) SetUpTest(c *gc.C) {
	s.ApiServerSuite.SetUpTest(c)

	s.auth = apiservertesting.FakeAuthorizer{
		Tag:        jujutesting.AdminUser,
		Controller: true,
	}

	var err error
	s.api, err = charms.NewFacade(facadetest.ModelContext{
		Auth_:   s.auth,
		State_:  s.ControllerModel(c).State(),
		Logger_: loggertesting.WrapCheckLog(c),
	})
	c.Assert(err, jc.ErrorIsNil)
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
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	for _, aCharm := range someCharms {
		f.MakeCharm(c, &factory.CharmParams{
			Name: aCharm, URL: fmt.Sprintf("local:quantal/%s-1", aCharm),
		})
	}
	found, err := s.api.List(context.Background(), params.CharmsList{Names: args})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(found.CharmURLs, gc.HasLen, len(expected))
	c.Check(found.CharmURLs, jc.DeepEquals, expected)
}

type charmsMockSuite struct {
	model        *mocks.MockBackendModel
	state        *mocks.MockBackendState
	authorizer   *apiservermocks.MockAuthorizer
	repoFactory  *mocks.MockRepositoryFactory
	repository   *mocks.MockRepository
	charmArchive *mocks.MockCharmArchive
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
	fullCurl := "ch:amd64/testme"

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
	result, err := api.ResolveCharms(context.Background(), args)
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

	result, err := api.ResolveCharms(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.ErrorMatches, `unknown schema for charm URL "local:testme"`)
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
	_, err := api.AddCharm(context.Background(), args)
	c.Assert(err, gc.ErrorMatches, `unknown schema for charm URL "local:testme"`)
}

func (s *charmsMockSuite) TestAddCharmCharmhub(c *gc.C) {
	// Charmhub charms are downloaded asynchronously
	defer s.setupMocks(c).Finish()

	curl := "chtest"

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

	s.state.EXPECT().Charm(curl).Return(nil, errors.NotFoundf("%q", curl))
	s.repoFactory.EXPECT().GetCharmRepository(gomock.Any(), gomock.Any()).Return(s.repository, nil)

	expMeta := new(charm.Meta)
	expManifest := new(charm.Manifest)
	expConfig := new(charm.Config)
	s.repository.EXPECT().GetEssentialMetadata(gomock.Any(), corecharm.MetadataRequest{
		CharmName: curl,
		Origin:    requestedOrigin,
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
			c.Assert(ci.ID, gc.DeepEquals, curl)
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
		URL: curl,
		Origin: params.CharmOrigin{
			Source: "charm-hub",
			Base:   params.Base{Name: "ubuntu", Channel: "20.04/stable"},
			Risk:   "edge",
		},
	}
	obtained, err := api.AddCharm(context.Background(), args)
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

	curl := "chtest"
	resURL, err := url.Parse(curl)
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

	s.state.EXPECT().Charm(curl).Return(nil, nil) // a nil error indicates that the charm doc already exists
	s.repoFactory.EXPECT().GetCharmRepository(gomock.Any(), gomock.Any()).Return(s.repository, nil)
	s.repository.EXPECT().GetDownloadURL(gomock.Any(), curl, gomock.Any()).Return(resURL, resolvedOrigin, nil)

	api := s.api(c)

	args := params.AddCharmWithOrigin{
		URL: curl,
		Origin: params.CharmOrigin{
			Source: "charm-hub",
			Risk:   "edge",
			Base:   params.Base{Name: "ubuntu", Channel: "20.04/stable"},
		},
		Force: false,
	}
	obtained, err := api.AddCharm(context.Background(), args)
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

	result, err := api.CheckCharmPlacement(context.Background(), args)
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

	result, err := api.CheckCharmPlacement(context.Background(), args)
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

	result, err := api.CheckCharmPlacement(context.Background(), args)
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

	result, err := api.CheckCharmPlacement(context.Background(), args)
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

	result, err := api.CheckCharmPlacement(context.Background(), args)
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

	result, err := api.CheckCharmPlacement(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.OneError(), gc.ErrorMatches, "charm can not be placed in a heterogeneous environment")
}

func (s *charmsMockSuite) api(c *gc.C) *charms.API {
	api, err := charms.NewCharmsAPI(
		s.authorizer,
		s.state,
		s.model,
		s.repoFactory,
		loggertesting.WrapCheckLog(c),
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

	s.application = mocks.NewMockApplication(ctrl)
	s.unit = mocks.NewMockUnit(ctrl)
	s.unit2 = mocks.NewMockUnit(ctrl)
	s.machine = mocks.NewMockMachine(ctrl)
	s.machine2 = mocks.NewMockMachine(ctrl)

	return ctrl
}

func (s *charmsMockSuite) expectResolveWithPreferredChannel(times int, err error) {
	s.repoFactory.EXPECT().GetCharmRepository(gomock.Any(), gomock.Any()).Return(s.repository, nil).Times(times)
	s.repository.EXPECT().ResolveWithPreferredChannel(
		gomock.Any(),
		gomock.AssignableToTypeOf(""),
		gomock.AssignableToTypeOf(corecharm.Origin{}),
	).DoAndReturn(
		func(_ context.Context, name string, requestedOrigin corecharm.Origin) (*charm.URL, corecharm.Origin, []corecharm.Platform, error) {
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
			// ensure the charm url returned is filled out
			curl := &charm.URL{
				Schema:       "ch",
				Name:         name,
				Architecture: "amd64",
				Revision:     -1,
			}
			return curl, resolvedOrigin, bases, err
		}).Times(times)
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
