// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrader_test

import (
	"context"
	"fmt"
	"net/url"
	"path"
	"strings"

	"github.com/juju/names/v6"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	"github.com/juju/juju/apiserver/facades/agent/upgrader"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	coreagentbinary "github.com/juju/juju/core/agentbinary"
	"github.com/juju/juju/core/arch"
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
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/binarystorage"
)

type upgraderSuite struct {
	jujutesting.ApiServerSuite

	mockModelUUID coremodel.UUID

	// These are raw State objects. Use them for setup and assertions, but
	// should never be touched by the API calls themselves
	rawMachine *state.Machine
	apiMachine *state.Machine
	upgrader   *upgrader.UpgraderAPI
	resources  *common.Resources
	authorizer apiservertesting.FakeAuthorizer
	hosted     *state.State
	store      objectstore.ObjectStore

	controllerConfigGetter *MockControllerConfigGetter
	agentService           *MockModelAgentService
	controllerNodeService  *MockControllerNodeService
	machineService         *MockMachineService
	unitService            *MockUnitService

	isUpgrader      *MockUpgrader
	watcherRegistry *facademocks.MockWatcherRegistry
}

var _ = gc.Suite(&upgraderSuite{})

func (s *upgraderSuite) SetUpTest(c *gc.C) {
	s.mockModelUUID = modeltesting.GenModelUUID(c)
	s.ControllerModelConfigAttrs = map[string]interface{}{
		"agent-version": coretesting.CurrentVersion().Number.String(),
	}
	s.ApiServerSuite.SetUpTest(c)
	s.resources = common.NewResources()
	s.AddCleanup(func(_ *gc.C) { s.resources.StopAll() })

	// For now, test with the controller model, but
	// we may add a different hosted model later.
	s.hosted = s.ControllerModel(c).State()

	// Create a machine to work with
	var err error
	// The first machine created is the only one allowed to
	// JobManageModel
	s.apiMachine, err = s.hosted.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits,
		state.JobManageModel)
	c.Assert(err, jc.ErrorIsNil)
	s.rawMachine, err = s.hosted.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	// The default auth is as the machine agent
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: s.rawMachine.Tag(),
	}

	domainServices := s.ControllerDomainServices(c)

	s.store = jujutesting.NewObjectStore(c, s.ControllerModelUUID())

	s.upgrader = upgrader.NewUpgraderAPI(
		nil,
		s.hosted,
		s.authorizer,
		loggertesting.WrapCheckLog(c),
		s.watcherRegistry,
		nil,
		domainServices.Machine(),
		domainServices.Agent(),
		nil,
	)
}

func (s *upgraderSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.controllerConfigGetter = NewMockControllerConfigGetter(ctrl)
	s.agentService = NewMockModelAgentService(ctrl)
	s.isUpgrader = NewMockUpgrader(ctrl)
	s.isUpgrader.EXPECT().IsUpgrading().Return(false, nil).AnyTimes()
	s.watcherRegistry = facademocks.NewMockWatcherRegistry(ctrl)
	s.controllerNodeService = NewMockControllerNodeService(ctrl)
	s.machineService = NewMockMachineService(ctrl)
	s.unitService = NewMockUnitService(ctrl)
	return ctrl
}

func (s *upgraderSuite) makeMockedUpgraderAPI(c *gc.C) *upgrader.UpgraderAPI {
	return upgrader.NewUpgraderAPI(
		nil,
		nil,
		s.authorizer,
		loggertesting.WrapCheckLog(c),
		s.watcherRegistry,
		s.controllerNodeService,
		s.machineService,
		s.agentService,
		s.unitService,
	)
}

func (s *upgraderSuite) TearDownTest(c *gc.C) {
	if s.resources != nil {
		s.resources.StopAll()
	}
	s.ApiServerSuite.TearDownTest(c)
}

func (s *upgraderSuite) TestToolsNothing(c *gc.C) {
	c.Skip("tlm")
	defer s.setupMocks(c).Finish()

	// Not an error to watch nothing
	results, err := s.upgrader.Tools(context.Background(), params.Entities{})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(results.Results, gc.HasLen, 0)
}

func (s *upgraderSuite) TestToolsRefusesWrongAgent(c *gc.C) {
	c.Skip("(tlm) skipping till we can move this test to mocks")
	anAuthorizer := s.authorizer
	anAuthorizer.Tag = names.NewMachineTag("12354")

	domainServices := s.DefaultModelDomainServices(c)

	anUpgrader := upgrader.NewUpgraderAPI(
		nil,
		s.hosted,
		anAuthorizer,
		loggertesting.WrapCheckLog(c),
		s.watcherRegistry,
		s.controllerNodeService,
		s.machineService,
		domainServices.Agent(),
		s.unitService,
	)

	args := params.Entities{
		Entities: []params.Entity{{Tag: s.rawMachine.Tag().String()}},
	}
	results, err := anUpgrader.Tools(context.Background(), args)
	// It is not an error to make the request, but the specific item is rejected
	c.Assert(err, jc.ErrorIsNil)
	c.Check(results.Results, gc.HasLen, 1)
	toolResult := results.Results[0]
	c.Assert(toolResult.Error, gc.DeepEquals, apiservertesting.ErrUnauthorized)
}

func (s *upgraderSuite) TestToolsForAgent(c *gc.C) {
	c.Skip("(tlm) skipping till we can move this test to mocks")
	defer s.setupMocks(c).Finish()

	current := coretesting.CurrentVersion()
	agent := params.Entity{Tag: s.rawMachine.Tag().String()}

	// Seed the newer agent in storage.
	stor, err := s.ControllerModel(c).State().ToolsStorage(s.store)
	c.Assert(err, jc.ErrorIsNil)
	defer func() {
		_ = stor.Close()
	}()
	content := jujuversion.Current.String()
	hash := fmt.Sprintf("sha256(%s)", content)
	v := semversion.Binary{
		Number:  jujuversion.Current,
		Release: "ubuntu",
		Arch:    arch.HostArch(),
	}
	err = stor.Add(context.Background(), strings.NewReader(content), binarystorage.Metadata{
		Version: v.String(),
		Size:    int64(len(content)),
		SHA256:  hash,
	})
	c.Assert(err, jc.ErrorIsNil)

	// The machine must have its existing tools set before we query for the
	// next tools. This is so that we can grab Arch and OSType without
	// having to pass it in again
	err = s.rawMachine.SetAgentVersion(current)
	c.Assert(err, jc.ErrorIsNil)

	args := params.Entities{Entities: []params.Entity{agent}}
	results, err := s.upgrader.Tools(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	assertTools := func() {
		c.Check(results.Results, gc.HasLen, 1)
		c.Assert(results.Results[0].Error, gc.IsNil)
		agentTools := results.Results[0].ToolsList[0]

		url := &url.URL{}
		url.Host = s.ControllerModelApiInfo().Addrs[0]
		url.Scheme = "https"
		url.Path = path.Join("model", coretesting.ModelTag.Id(), "tools", current.String())
		c.Check(agentTools.URL, gc.Equals, url.String())
		c.Check(agentTools.Version, gc.DeepEquals, current)
	}
	assertTools()
}

// TestSetToolsNothing tests that SetTools does nothing and returns no errors
// when called.
func (s *upgraderSuite) TestSetToolsNothing(c *gc.C) {
	defer s.setupMocks(c).Finish()
	results, err := s.makeMockedUpgraderAPI(c).SetTools(context.Background(), params.EntitiesVersion{})
	c.Check(err, jc.ErrorIsNil)
	c.Check(results.Results, gc.HasLen, 0)
}

// TestSetToolsRefusesWrongAgent tests that SetTools refused to set the agent
// for a tag that isn't authorized. We test many tag types here to prove that
// a tag of one type cannot set the tools version of another tag type.
func (s *upgraderSuite) TestSetToolsRefusesWrongAgent(c *gc.C) {
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
		c.Logf("running TestSetToolsRefusesWrongAgent test #d", i)
		args := params.EntitiesVersion{
			AgentTools: []params.EntityVersion{{
				Tag: test.TagToSet.String(),
				Tools: &params.Version{
					Version: coretesting.CurrentVersion(),
				},
			}},
		}
		results, err := api.SetTools(context.Background(), args)
		c.Check(err, jc.ErrorIsNil)
		c.Assert(results.Results, gc.HasLen, 1)
		c.Check(results.Results[0].Error, gc.DeepEquals, apiservertesting.ErrUnauthorized)
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
func (s *upgraderSuite) TestSetToolsForUnknownTagEntity(c *gc.C) {
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
	result, err := s.makeMockedUpgraderAPI(c).SetTools(context.Background(), args)
	c.Check(err, jc.ErrorIsNil)
	c.Assert(len(result.Results), gc.Equals, 1)
	c.Check(result.Results[0].Error.Error(), gc.Matches, "entity \"application-foo\" does not support agent binaries")
}

// TestSetToolsMachine is testing the ability to set tools for a machine. This
// is a happy path test.
func (s *upgraderSuite) TestSetToolsMachine(c *gc.C) {
	machineTag := names.NewMachineTag("0")
	s.authorizer.Tag = machineTag
	defer s.setupMocks(c).Finish()

	s.machineService.EXPECT().SetReportedMachineAgentVersion(
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
	results, err := api.SetTools(context.Background(), args)
	c.Check(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)
}

// TestSetToolsMachineNotFound is testing that when we try and set the reported
// tools version for a machine that doesn't exist we get an a not found api
// error back.
func (s *upgraderSuite) TestSetToolsMachineNotFound(c *gc.C) {
	machineTag := names.NewMachineTag("0")
	s.authorizer.Tag = machineTag
	defer s.setupMocks(c).Finish()

	s.machineService.EXPECT().SetReportedMachineAgentVersion(
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
	results, err := api.SetTools(context.Background(), args)
	c.Check(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error.ErrorCode(), gc.Equals, "not found")
}

// TestSetToolsUnit is testing the ability to set tools for a unit. This
// is a happy path test.
func (s *upgraderSuite) TestSetToolsUnit(c *gc.C) {
	unitTag := names.NewUnitTag("foo/0")
	s.authorizer.Tag = unitTag
	defer s.setupMocks(c).Finish()

	unitName, err := coreunit.NewName("foo/0")
	c.Assert(err, jc.ErrorIsNil)

	s.unitService.EXPECT().SetReportedUnitAgentVersion(
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
	results, err := api.SetTools(context.Background(), args)
	c.Check(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)
}

// TestSetToolsUnitNotFound is testing that when we try and set the reported
// tools version for a unit that doesn't exist we get an a not found api
// error back.
func (s *upgraderSuite) TestSetToolsUnitNotFound(c *gc.C) {
	unitTag := names.NewUnitTag("foo/0")
	s.authorizer.Tag = unitTag
	defer s.setupMocks(c).Finish()

	unitName, err := coreunit.NewName("foo/0")
	c.Assert(err, jc.ErrorIsNil)

	s.unitService.EXPECT().SetReportedUnitAgentVersion(
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
	results, err := api.SetTools(context.Background(), args)
	c.Check(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error.ErrorCode(), gc.Equals, "not found")
}

// TestSetToolsControllerNode is testing the ability to set tools for a
// controller node. This is a happy path test.
func (s *upgraderSuite) TestSetToolsControllerNode(c *gc.C) {
	controllerTag := names.NewControllerAgentTag("1234")
	s.authorizer.Tag = controllerTag
	defer s.setupMocks(c).Finish()

	s.controllerNodeService.EXPECT().SetReportedControllerNodeAgentVersion(
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
	results, err := api.SetTools(context.Background(), args)
	c.Check(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)
}

// TestSetToolsControllerNotFound is testing that when we try and set the
// reported tools version for a unit that doesn't exist we get an a not found
// api error back.
func (s *upgraderSuite) TestSetToolsControllerNotFound(c *gc.C) {
	controllerTag := names.NewControllerAgentTag("1234")
	s.authorizer.Tag = controllerTag
	defer s.setupMocks(c).Finish()

	s.controllerNodeService.EXPECT().SetReportedControllerNodeAgentVersion(
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
	results, err := api.SetTools(context.Background(), args)
	c.Check(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error.ErrorCode(), gc.Equals, "not found")
}

// TestSetToolsUnsupportedArchitecture is checking that when an attempt is made
// to set the reported agent binary tools version for a given entity and we
// don't support the architecture that the error returned is the expected.
func (s *upgraderSuite) TestSetToolsUnsupportedArchitecture(c *gc.C) {
	defer s.setupMocks(c).Finish()

	tags := []names.Tag{
		names.NewMachineTag("0"),
		names.NewUnitTag("foo/0"),
		names.NewControllerAgentTag("123"),
	}

	ver := semversion.Number{Major: 4, Minor: 0, Patch: 0}
	s.controllerNodeService.EXPECT().SetReportedControllerNodeAgentVersion(
		gomock.Any(),
		gomock.Any(),
		coreagentbinary.Version{
			Number: ver,
			Arch:   "unknown",
		},
	).Return(coreerrors.NotSupported)
	s.machineService.EXPECT().SetReportedMachineAgentVersion(
		gomock.Any(),
		gomock.Any(),
		coreagentbinary.Version{
			Number: ver,
			Arch:   "unknown",
		},
	).Return(coreerrors.NotSupported)
	s.unitService.EXPECT().SetReportedUnitAgentVersion(
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
		result, err := api.SetTools(context.Background(), args)
		c.Check(err, jc.ErrorIsNil)
		c.Check(len(result.Results), gc.Equals, 1)
		c.Check(result.Results[0].Error.ErrorCode(), gc.Equals, "not supported")
	}
}

// TestSetToolsUnsupportedArchitecture is checking that when an attempt is made
// to set the reported agent binary tools version for a given entity and we
// don't support the architecture that the error returned is the expected.
func (s *upgraderSuite) TestSetToolsInvalidVersion(c *gc.C) {
	defer s.setupMocks(c).Finish()

	tags := []names.Tag{
		names.NewMachineTag("0"),
		names.NewUnitTag("foo/0"),
		names.NewControllerAgentTag("123"),
	}

	ver := semversion.Number{Major: 0, Minor: 0, Patch: 0}
	s.controllerNodeService.EXPECT().SetReportedControllerNodeAgentVersion(
		gomock.Any(),
		gomock.Any(),
		coreagentbinary.Version{
			Number: ver,
			Arch:   "arm64",
		},
	).Return(coreerrors.NotValid)
	s.machineService.EXPECT().SetReportedMachineAgentVersion(
		gomock.Any(),
		gomock.Any(),
		coreagentbinary.Version{
			Number: ver,
			Arch:   "arm64",
		},
	).Return(coreerrors.NotValid)
	s.unitService.EXPECT().SetReportedUnitAgentVersion(
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
		result, err := api.SetTools(context.Background(), args)
		c.Check(err, jc.ErrorIsNil)
		c.Check(len(result.Results), gc.Equals, 1)
		c.Check(result.Results[0].Error.ErrorCode(), gc.Equals, "not valid")
	}
}

func (s *upgraderSuite) TestDesiredVersionNothing(c *gc.C) {
	c.Skip("tlm")
	defer s.setupMocks(c).Finish()

	// Not an error to watch nothing
	results, err := s.upgrader.DesiredVersion(context.Background(), params.Entities{})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(results.Results, gc.HasLen, 0)
}

func (s *upgraderSuite) TestDesiredVersionRefusesWrongAgent(c *gc.C) {
	c.Skip("some reason")
	defer s.setupMocks(c).Finish()

	anAuthorizer := s.authorizer
	anAuthorizer.Tag = names.NewMachineTag("12354")
	domainServices := s.DefaultModelDomainServices(c)

	anUpgrader := upgrader.NewUpgraderAPI(
		nil,
		s.hosted,
		anAuthorizer,
		loggertesting.WrapCheckLog(c),
		s.watcherRegistry,
		s.controllerNodeService,
		s.machineService,
		domainServices.Agent(),
		s.unitService,
	)
	args := params.Entities{
		Entities: []params.Entity{{Tag: s.rawMachine.Tag().String()}},
	}
	results, err := anUpgrader.DesiredVersion(context.Background(), args)
	// It is not an error to make the request, but the specific item is rejected
	c.Assert(err, jc.ErrorIsNil)
	c.Check(results.Results, gc.HasLen, 1)
	toolResult := results.Results[0]
	c.Assert(toolResult.Error, gc.DeepEquals, apiservertesting.ErrUnauthorized)
}

func (s *upgraderSuite) TestDesiredVersionNoticesMixedAgents(c *gc.C) {
	c.Skip("some reason")
	defer s.setupMocks(c).Finish()

	args := params.Entities{Entities: []params.Entity{
		{Tag: s.rawMachine.Tag().String()},
		{Tag: "machine-12345"},
	}}
	results, err := s.upgrader.DesiredVersion(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(results.Results, gc.HasLen, 2)
	c.Assert(results.Results[0].Error, gc.IsNil)
	agentVersion := results.Results[0].Version
	c.Assert(agentVersion, gc.NotNil)
	c.Check(*agentVersion, gc.DeepEquals, jujuversion.Current)

	c.Assert(results.Results[1].Error, gc.DeepEquals, apiservertesting.ErrUnauthorized)
	c.Assert(results.Results[1].Version, gc.IsNil)

}

func (s *upgraderSuite) TestDesiredVersionForAgent(c *gc.C) {
	c.Skip("some reason")
	defer s.setupMocks(c).Finish()

	args := params.Entities{Entities: []params.Entity{{Tag: s.rawMachine.Tag().String()}}}
	results, err := s.upgrader.DesiredVersion(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)
	agentVersion := results.Results[0].Version
	c.Assert(agentVersion, gc.NotNil)
	c.Check(*agentVersion, gc.DeepEquals, jujuversion.Current)
}

func (s *upgraderSuite) TestDesiredVersionUnrestrictedForAPIAgents(c *gc.C) {
	c.Skip("some reason")
	defer s.setupMocks(c).Finish()

	newVersion := coretesting.CurrentVersion()
	newVersion.Patch++
	s.agentService.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(newVersion.Number, nil)

	// Grab a different Upgrader for the apiMachine
	authorizer := apiservertesting.FakeAuthorizer{
		Tag: s.apiMachine.Tag(),
	}

	upgraderAPI := upgrader.NewUpgraderAPI(
		nil,
		s.hosted,
		authorizer,
		loggertesting.WrapCheckLog(c),
		s.watcherRegistry,
		s.controllerNodeService,
		s.machineService,
		s.agentService,
		s.unitService,
	)
	args := params.Entities{Entities: []params.Entity{{Tag: s.apiMachine.Tag().String()}}}
	results, err := upgraderAPI.DesiredVersion(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)
	agentVersion := results.Results[0].Version
	c.Assert(agentVersion, gc.NotNil)
	c.Check(*agentVersion, gc.DeepEquals, newVersion.Number)
}

func (s *upgraderSuite) TestDesiredVersionRestrictedForNonAPIAgents(c *gc.C) {
	c.Skip("some reason")
	defer s.setupMocks(c).Finish()
	args := params.Entities{Entities: []params.Entity{{Tag: s.rawMachine.Tag().String()}}}
	results, err := s.upgrader.DesiredVersion(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)
	agentVersion := results.Results[0].Version
	c.Assert(agentVersion, gc.NotNil)
	c.Check(*agentVersion, gc.DeepEquals, jujuversion.Current)
}
