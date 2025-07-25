// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrader_test

import (
	"testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/common"
	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	"github.com/juju/juju/apiserver/facades/agent/upgrader"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	coreagentbinary "github.com/juju/juju/core/agentbinary"
	coreerrors "github.com/juju/juju/core/errors"
	coremachine "github.com/juju/juju/core/machine"
	coremodel "github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/semversion"
	coreunit "github.com/juju/juju/core/unit"
	jujuversion "github.com/juju/juju/core/version"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	coretesting "github.com/juju/juju/internal/testing"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
)

type upgraderSuite struct {
	jujutesting.ApiServerSuite

	mockModelUUID coremodel.UUID

	rawMachineTag names.MachineTag
	apiMachineTag names.MachineTag

	// These are raw State objects. Use them for setup and assertions, but
	// should never be touched by the API calls themselves
	upgrader   *upgrader.UpgraderAPI
	resources  *common.Resources
	authorizer apiservertesting.FakeAuthorizer
	store      objectstore.ObjectStore

	controllerConfigGetter *MockControllerConfigGetter
	agentService           *MockModelAgentService
	controllerNodeService  *MockControllerNodeService
	machineService         *MockMachineService

	isUpgrader      *MockUpgrader
	watcherRegistry *facademocks.MockWatcherRegistry
}

func TestUpgraderSuite(t *testing.T) {
	tc.Run(t, &upgraderSuite{})
}

func (s *upgraderSuite) SetUpTest(c *tc.C) {
	s.mockModelUUID = modeltesting.GenModelUUID(c)
	s.ControllerModelConfigAttrs = map[string]interface{}{
		"agent-version": coretesting.CurrentVersion().Number.String(),
	}
	s.ApiServerSuite.SetUpTest(c)
	s.resources = common.NewResources()
	s.AddCleanup(func(_ *tc.C) { s.resources.StopAll() })

	s.rawMachineTag = names.NewMachineTag("0")
	s.apiMachineTag = names.NewMachineTag("1")

	// The default auth is as the machine agent
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: s.apiMachineTag,
	}

	// For now, test with the controller model, but
	// we may add a different hosted model later.
	domainServices := s.ControllerDomainServices(c)

	s.store = jujutesting.NewObjectStore(c, s.ControllerModelUUID())

	s.upgrader = upgrader.NewUpgraderAPI(
		nil,
		s.authorizer,
		loggertesting.WrapCheckLog(c),
		s.watcherRegistry,
		nil,
		domainServices.Agent(),
		domainServices.Machine(),
	)
}

func (s *upgraderSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.controllerConfigGetter = NewMockControllerConfigGetter(ctrl)
	s.agentService = NewMockModelAgentService(ctrl)
	s.controllerNodeService = NewMockControllerNodeService(ctrl)
	s.machineService = NewMockMachineService(ctrl)

	s.isUpgrader = NewMockUpgrader(ctrl)
	s.isUpgrader.EXPECT().IsUpgrading().Return(false, nil).AnyTimes()

	s.watcherRegistry = facademocks.NewMockWatcherRegistry(ctrl)

	return ctrl
}

func (s *upgraderSuite) makeMockedUpgraderAPI(c *tc.C) *upgrader.UpgraderAPI {
	return upgrader.NewUpgraderAPI(
		nil,
		s.authorizer,
		loggertesting.WrapCheckLog(c),
		s.watcherRegistry,
		s.controllerNodeService,
		s.agentService,
		s.machineService,
	)
}

func (s *upgraderSuite) TearDownTest(c *tc.C) {
	if s.resources != nil {
		s.resources.StopAll()
	}
	s.ApiServerSuite.TearDownTest(c)
}

func (s *upgraderSuite) TestToolsNothing(c *tc.C) {
	c.Skip("tlm")
	defer s.setupMocks(c).Finish()

	// Not an error to watch nothing
	results, err := s.upgrader.Tools(c.Context(), params.Entities{})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(results.Results, tc.HasLen, 0)
}

func (s *upgraderSuite) TestToolsRefusesWrongAgent(c *tc.C) {
	c.Skip("(tlm) skipping till we can move this test to mocks")
	anAuthorizer := s.authorizer
	anAuthorizer.Tag = names.NewMachineTag("12354")

	domainServices := s.DefaultModelDomainServices(c)

	anUpgrader := upgrader.NewUpgraderAPI(
		nil,
		anAuthorizer,
		loggertesting.WrapCheckLog(c),
		s.watcherRegistry,
		s.controllerNodeService,
		domainServices.Agent(),
		s.machineService,
	)

	args := params.Entities{
		Entities: []params.Entity{{Tag: s.rawMachineTag.String()}},
	}
	results, err := anUpgrader.Tools(c.Context(), args)
	// It is not an error to make the request, but the specific item is rejected
	c.Assert(err, tc.ErrorIsNil)
	c.Check(results.Results, tc.HasLen, 1)
	toolResult := results.Results[0]
	c.Assert(toolResult.Error, tc.DeepEquals, apiservertesting.ErrUnauthorized)
}

// TestSetToolsNothing tests that SetTools does nothing and returns no errors
// when called.
func (s *upgraderSuite) TestSetToolsNothing(c *tc.C) {
	defer s.setupMocks(c).Finish()
	results, err := s.makeMockedUpgraderAPI(c).SetTools(c.Context(), params.EntitiesVersion{})
	c.Check(err, tc.ErrorIsNil)
	c.Check(results.Results, tc.HasLen, 0)
}

// TestSetToolsRefusesWrongAgent tests that SetTools refused to set the agent
// for a tag that isn't authorized. We test many tag types here to prove that
// a tag of one type cannot set the tools version of another tag type.
func (s *upgraderSuite) TestSetToolsRefusesWrongAgent(c *tc.C) {
	s.authorizer.Tag = names.NewMachineTag("12354")
	defer s.setupMocks(c).Finish()

	tests := []struct {
		TagToSet names.Tag
	}{
		{
			TagToSet: names.NewMachineTag("0"),
		},
		{
			TagToSet: names.NewUnitTag("foo/0"),
		},
		{
			TagToSet: names.NewControllerTag("0"),
		},
		{
			TagToSet: names.NewApplicationTag("foo"),
		},
	}

	api := s.makeMockedUpgraderAPI(c)

	for i, test := range tests {
		c.Logf("running TestSetToolsRefusesWrongAgent test %d", i)
		args := params.EntitiesVersion{
			AgentTools: []params.EntityVersion{{
				Tag: test.TagToSet.String(),
				Tools: &params.Version{
					Version: coretesting.CurrentVersion(),
				},
			}},
		}
		results, err := api.SetTools(c.Context(), args)
		c.Check(err, tc.ErrorIsNil)
		c.Assert(results.Results, tc.HasLen, 1)
		c.Check(results.Results[0].Error, tc.DeepEquals, apiservertesting.ErrUnauthorized)
	}
}

// TestSetToolsForUnknownTagEntity is checking what the response behaviour is
// when we try and set the reported agent tools version for an entity that we
// don't support setting agent tools version for.
//
// This is a new test implemented with the move to DQlite. The contract we had
// around this was that under this scenario a typed error was not returned but
// just the error string "entity "foo" does not support agent binaries".
//
// While this is a week contract it is still one we need to validate that isn't
// broken in the move.
func (s *upgraderSuite) TestSetToolsForUnknownTagEntity(c *tc.C) {
	// We use an application tag because we know that this isn't supported.
	s.authorizer.Tag = names.NewApplicationTag("foo")
	defer s.setupMocks(c).Finish()

	args := params.EntitiesVersion{
		AgentTools: []params.EntityVersion{{
			Tag: names.NewApplicationTag("foo").String(),
			Tools: &params.Version{
				Version: coretesting.CurrentVersion(),
			},
		}},
	}
	result, err := s.makeMockedUpgraderAPI(c).SetTools(c.Context(), args)
	c.Check(err, tc.ErrorIsNil)
	c.Assert(len(result.Results), tc.Equals, 1)
	c.Check(result.Results[0].Error.Error(), tc.Matches, "entity \"application-foo\" does not support agent binaries")
}

// TestSetToolsMachine is testing the ability to set tools for a machine. This
// is a happy path test.
func (s *upgraderSuite) TestSetToolsMachine(c *tc.C) {
	machineTag := names.NewMachineTag("0")
	s.authorizer.Tag = machineTag
	defer s.setupMocks(c).Finish()

	s.agentService.EXPECT().SetMachineReportedAgentVersion(
		gomock.Any(),
		coremachine.Name("0"),
		coreagentbinary.Version{
			Number: coretesting.CurrentVersion().Number,
			Arch:   coretesting.CurrentVersion().Arch,
		},
	)

	args := params.EntitiesVersion{
		AgentTools: []params.EntityVersion{{
			Tag: machineTag.String(),
			Tools: &params.Version{
				Version: coretesting.CurrentVersion(),
			}},
		},
	}
	api := s.makeMockedUpgraderAPI(c)
	results, err := api.SetTools(c.Context(), args)
	c.Check(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Assert(results.Results[0].Error, tc.IsNil)
}

// TestSetToolsMachineNotFound is testing that when we try and set the reported
// tools version for a machine that doesn't exist we get an a not found api
// error back.
func (s *upgraderSuite) TestSetToolsMachineNotFound(c *tc.C) {
	machineTag := names.NewMachineTag("0")
	s.authorizer.Tag = machineTag
	defer s.setupMocks(c).Finish()

	s.agentService.EXPECT().SetMachineReportedAgentVersion(
		gomock.Any(),
		coremachine.Name("0"),
		coreagentbinary.Version{
			Number: coretesting.CurrentVersion().Number,
			Arch:   coretesting.CurrentVersion().Arch,
		},
	).Return(machineerrors.MachineNotFound)

	args := params.EntitiesVersion{
		AgentTools: []params.EntityVersion{{
			Tag: machineTag.String(),
			Tools: &params.Version{
				Version: coretesting.CurrentVersion(),
			}},
		},
	}
	api := s.makeMockedUpgraderAPI(c)
	results, err := api.SetTools(c.Context(), args)
	c.Check(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Assert(results.Results[0].Error.ErrorCode(), tc.Equals, "not found")
}

// TestSetToolsUnit is testing the ability to set tools for a unit. This
// is a happy path test.
func (s *upgraderSuite) TestSetToolsUnit(c *tc.C) {
	unitTag := names.NewUnitTag("foo/0")
	s.authorizer.Tag = unitTag
	defer s.setupMocks(c).Finish()

	unitName, err := coreunit.NewName("foo/0")
	c.Assert(err, tc.ErrorIsNil)

	s.agentService.EXPECT().SetUnitReportedAgentVersion(
		gomock.Any(),
		unitName,
		coreagentbinary.Version{
			Number: coretesting.CurrentVersion().Number,
			Arch:   coretesting.CurrentVersion().Arch,
		},
	)

	args := params.EntitiesVersion{
		AgentTools: []params.EntityVersion{{
			Tag: unitTag.String(),
			Tools: &params.Version{
				Version: coretesting.CurrentVersion(),
			}},
		},
	}
	api := s.makeMockedUpgraderAPI(c)
	results, err := api.SetTools(c.Context(), args)
	c.Check(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Assert(results.Results[0].Error, tc.IsNil)
}

// TestSetToolsUnitNotFound is testing that when we try and set the reported
// tools version for a unit that doesn't exist we get an a not found api
// error back.
func (s *upgraderSuite) TestSetToolsUnitNotFound(c *tc.C) {
	unitTag := names.NewUnitTag("foo/0")
	s.authorizer.Tag = unitTag
	defer s.setupMocks(c).Finish()

	unitName, err := coreunit.NewName("foo/0")
	c.Assert(err, tc.ErrorIsNil)

	s.agentService.EXPECT().SetUnitReportedAgentVersion(
		gomock.Any(),
		unitName,
		coreagentbinary.Version{
			Number: coretesting.CurrentVersion().Number,
			Arch:   coretesting.CurrentVersion().Arch,
		},
	).Return(applicationerrors.UnitNotFound)

	args := params.EntitiesVersion{
		AgentTools: []params.EntityVersion{{
			Tag: unitTag.String(),
			Tools: &params.Version{
				Version: coretesting.CurrentVersion(),
			}},
		},
	}
	api := s.makeMockedUpgraderAPI(c)
	results, err := api.SetTools(c.Context(), args)
	c.Check(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Assert(results.Results[0].Error.ErrorCode(), tc.Equals, "not found")
}

// TestSetToolsControllerNode is testing the ability to set tools for a
// controller node. This is a happy path test.
func (s *upgraderSuite) TestSetToolsControllerNode(c *tc.C) {
	controllerTag := names.NewControllerAgentTag("1234")
	s.authorizer.Tag = controllerTag
	defer s.setupMocks(c).Finish()

	s.controllerNodeService.EXPECT().SetControllerNodeReportedAgentVersion(
		gomock.Any(),
		controllerTag.Id(),
		coreagentbinary.Version{
			Number: coretesting.CurrentVersion().Number,
			Arch:   coretesting.CurrentVersion().Arch,
		},
	)

	args := params.EntitiesVersion{
		AgentTools: []params.EntityVersion{{
			Tag: controllerTag.String(),
			Tools: &params.Version{
				Version: coretesting.CurrentVersion(),
			}},
		},
	}
	api := s.makeMockedUpgraderAPI(c)
	results, err := api.SetTools(c.Context(), args)
	c.Check(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Assert(results.Results[0].Error, tc.IsNil)
}

// TestSetToolsControllerNotFound is testing that when we try and set the
// reported tools version for a unit that doesn't exist we get an a not found
// api error back.
func (s *upgraderSuite) TestSetToolsControllerNotFound(c *tc.C) {
	controllerTag := names.NewControllerAgentTag("1234")
	s.authorizer.Tag = controllerTag
	defer s.setupMocks(c).Finish()

	s.controllerNodeService.EXPECT().SetControllerNodeReportedAgentVersion(
		gomock.Any(),
		controllerTag.Id(),
		coreagentbinary.Version{
			Number: coretesting.CurrentVersion().Number,
			Arch:   coretesting.CurrentVersion().Arch,
		},
	).Return(applicationerrors.UnitNotFound)

	args := params.EntitiesVersion{
		AgentTools: []params.EntityVersion{{
			Tag: controllerTag.String(),
			Tools: &params.Version{
				Version: coretesting.CurrentVersion(),
			}},
		},
	}
	api := s.makeMockedUpgraderAPI(c)
	results, err := api.SetTools(c.Context(), args)
	c.Check(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Assert(results.Results[0].Error.ErrorCode(), tc.Equals, "not found")
}

// TestSetToolsUnsupportedArchitecture is checking that when an attempt is made
// to set the reported agent binary tools version for a given entity and we
// don't support the architecture that the error returned is the expected.
func (s *upgraderSuite) TestSetToolsUnsupportedArchitecture(c *tc.C) {
	defer s.setupMocks(c).Finish()

	tags := []names.Tag{
		names.NewMachineTag("0"),
		names.NewUnitTag("foo/0"),
		names.NewControllerAgentTag("123"),
	}

	ver := semversion.Number{Major: 4, Minor: 0, Patch: 0}
	s.controllerNodeService.EXPECT().SetControllerNodeReportedAgentVersion(
		gomock.Any(),
		gomock.Any(),
		coreagentbinary.Version{
			Number: ver,
			Arch:   "unknown",
		},
	).Return(coreerrors.NotSupported)
	s.agentService.EXPECT().SetMachineReportedAgentVersion(
		gomock.Any(),
		gomock.Any(),
		coreagentbinary.Version{
			Number: ver,
			Arch:   "unknown",
		},
	).Return(coreerrors.NotSupported)
	s.agentService.EXPECT().SetUnitReportedAgentVersion(
		gomock.Any(),
		gomock.Any(),
		coreagentbinary.Version{
			Number: ver,
			Arch:   "unknown",
		},
	).Return(coreerrors.NotSupported)

	for _, tag := range tags {
		s.authorizer.Tag = tag
		args := params.EntitiesVersion{
			AgentTools: []params.EntityVersion{{
				Tag: tag.String(),
				Tools: &params.Version{
					Version: semversion.Binary{
						Number: ver,
						Arch:   "unknown",
					},
				}},
			},
		}

		api := s.makeMockedUpgraderAPI(c)
		result, err := api.SetTools(c.Context(), args)
		c.Check(err, tc.ErrorIsNil)
		c.Check(len(result.Results), tc.Equals, 1)
		c.Check(result.Results[0].Error.ErrorCode(), tc.Equals, "not supported")
	}
}

// TestSetToolsUnsupportedArchitecture is checking that when an attempt is made
// to set the reported agent binary tools version for a given entity and we
// don't support the architecture that the error returned is the expected.
func (s *upgraderSuite) TestSetToolsInvalidVersion(c *tc.C) {
	defer s.setupMocks(c).Finish()

	tags := []names.Tag{
		names.NewMachineTag("0"),
		names.NewUnitTag("foo/0"),
		names.NewControllerAgentTag("123"),
	}

	ver := semversion.Number{Major: 0, Minor: 0, Patch: 0}
	s.controllerNodeService.EXPECT().SetControllerNodeReportedAgentVersion(
		gomock.Any(),
		gomock.Any(),
		coreagentbinary.Version{
			Number: ver,
			Arch:   "arm64",
		},
	).Return(coreerrors.NotValid)
	s.agentService.EXPECT().SetMachineReportedAgentVersion(
		gomock.Any(),
		gomock.Any(),
		coreagentbinary.Version{
			Number: ver,
			Arch:   "arm64",
		},
	).Return(coreerrors.NotValid)
	s.agentService.EXPECT().SetUnitReportedAgentVersion(
		gomock.Any(),
		gomock.Any(),
		coreagentbinary.Version{
			Number: ver,
			Arch:   "arm64",
		},
	).Return(coreerrors.NotValid)

	for _, tag := range tags {
		s.authorizer.Tag = tag
		args := params.EntitiesVersion{
			AgentTools: []params.EntityVersion{{
				Tag: tag.String(),
				Tools: &params.Version{
					Version: semversion.Binary{
						Number: ver,
						Arch:   "arm64",
					},
				}},
			},
		}

		api := s.makeMockedUpgraderAPI(c)
		result, err := api.SetTools(c.Context(), args)
		c.Check(err, tc.ErrorIsNil)
		c.Check(len(result.Results), tc.Equals, 1)
		c.Check(result.Results[0].Error.ErrorCode(), tc.Equals, "not valid")
	}
}

func (s *upgraderSuite) TestDesiredVersionNothing(c *tc.C) {
	c.Skip("skipping till we can move this test to mocks")

	defer s.setupMocks(c).Finish()

	// Not an error to watch nothing
	results, err := s.upgrader.DesiredVersion(c.Context(), params.Entities{})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(results.Results, tc.HasLen, 0)
}

func (s *upgraderSuite) TestDesiredVersionRefusesWrongAgent(c *tc.C) {
	c.Skip("skipping till we can move this test to mocks")

	defer s.setupMocks(c).Finish()

	anAuthorizer := s.authorizer
	anAuthorizer.Tag = names.NewMachineTag("12354")
	domainServices := s.DefaultModelDomainServices(c)

	anUpgrader := upgrader.NewUpgraderAPI(
		nil,
		anAuthorizer,
		loggertesting.WrapCheckLog(c),
		s.watcherRegistry,
		s.controllerNodeService,
		domainServices.Agent(),
		s.machineService,
	)
	args := params.Entities{
		Entities: []params.Entity{{Tag: s.rawMachineTag.String()}},
	}
	results, err := anUpgrader.DesiredVersion(c.Context(), args)
	// It is not an error to make the request, but the specific item is rejected
	c.Assert(err, tc.ErrorIsNil)
	c.Check(results.Results, tc.HasLen, 1)
	toolResult := results.Results[0]
	c.Assert(toolResult.Error, tc.DeepEquals, apiservertesting.ErrUnauthorized)
}

func (s *upgraderSuite) TestDesiredVersionNoticesMixedAgents(c *tc.C) {
	c.Skip("skipping till we can move this test to mocks")

	defer s.setupMocks(c).Finish()

	args := params.Entities{Entities: []params.Entity{
		{Tag: s.rawMachineTag.String()},
		{Tag: "machine-12345"},
	}}
	results, err := s.upgrader.DesiredVersion(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(results.Results, tc.HasLen, 2)
	c.Assert(results.Results[0].Error, tc.IsNil)
	agentVersion := results.Results[0].Version
	c.Assert(agentVersion, tc.NotNil)
	c.Check(*agentVersion, tc.DeepEquals, jujuversion.Current)

	c.Assert(results.Results[1].Error, tc.DeepEquals, apiservertesting.ErrUnauthorized)
	c.Assert(results.Results[1].Version, tc.IsNil)

}

func (s *upgraderSuite) TestDesiredVersionForAgent(c *tc.C) {
	c.Skip("skipping till we can move this test to mocks")

	defer s.setupMocks(c).Finish()

	args := params.Entities{Entities: []params.Entity{{Tag: s.rawMachineTag.String()}}}
	results, err := s.upgrader.DesiredVersion(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(results.Results, tc.HasLen, 1)
	c.Assert(results.Results[0].Error, tc.IsNil)
	agentVersion := results.Results[0].Version
	c.Assert(agentVersion, tc.NotNil)
	c.Check(*agentVersion, tc.DeepEquals, jujuversion.Current)
}

func (s *upgraderSuite) TestDesiredVersionUnrestrictedForAPIAgents(c *tc.C) {
	c.Skip("skipping till we can move this test to mocks")

	defer s.setupMocks(c).Finish()

	newVersion := coretesting.CurrentVersion()
	newVersion.Patch++
	s.agentService.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(newVersion.Number, nil)

	// Grab a different Upgrader for the apiMachine
	authorizer := apiservertesting.FakeAuthorizer{
		Tag: s.apiMachineTag,
	}

	upgraderAPI := upgrader.NewUpgraderAPI(
		nil,
		authorizer,
		loggertesting.WrapCheckLog(c),
		s.watcherRegistry,
		s.controllerNodeService,
		s.agentService,
		s.machineService,
	)
	args := params.Entities{Entities: []params.Entity{{Tag: s.apiMachineTag.String()}}}
	results, err := upgraderAPI.DesiredVersion(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(results.Results, tc.HasLen, 1)
	c.Assert(results.Results[0].Error, tc.IsNil)
	agentVersion := results.Results[0].Version
	c.Assert(agentVersion, tc.NotNil)
	c.Check(*agentVersion, tc.DeepEquals, newVersion.Number)
}

func (s *upgraderSuite) TestDesiredVersionRestrictedForNonAPIAgents(c *tc.C) {
	c.Skip("skipping till we can move this test to mocks")

	defer s.setupMocks(c).Finish()
	args := params.Entities{Entities: []params.Entity{{Tag: s.rawMachineTag.String()}}}
	results, err := s.upgrader.DesiredVersion(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(results.Results, tc.HasLen, 1)
	c.Assert(results.Results[0].Error, tc.IsNil)
	agentVersion := results.Results[0].Version
	c.Assert(agentVersion, tc.NotNil)
	c.Check(*agentVersion, tc.DeepEquals, jujuversion.Current)
}
