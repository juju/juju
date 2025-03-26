// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrader_test

import (
	"context"
	"fmt"
	"net/url"
	"path"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	"github.com/juju/juju/apiserver/facades/agent/upgrader"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/objectstore"
	jujuversion "github.com/juju/juju/core/version"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/version"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/binarystorage"
)

type upgraderSuite struct {
	jujutesting.ApiServerSuite

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
	isUpgrader             *MockUpgrader
	watcherRegistry        *facademocks.MockWatcherRegistry
}

var _ = gc.Suite(&upgraderSuite{})

func (s *upgraderSuite) SetUpTest(c *gc.C) {
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
	systemState, err := s.StatePool().SystemState()
	c.Assert(err, jc.ErrorIsNil)

	domainServices := s.ControllerDomainServices(c)

	s.store = jujutesting.NewObjectStore(c, s.ControllerModelUUID())

	s.upgrader, err = upgrader.NewUpgraderAPI(
		s.controllerConfigGetter,
		systemState,
		s.hosted,
		s.authorizer,
		loggertesting.WrapCheckLog(c),
		domainServices.Cloud(),
		domainServices.Credential(),
		domainServices.Config(),
		domainServices.Agent(),
		s.store,
		s.watcherRegistry,
	)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *upgraderSuite) TearDownTest(c *gc.C) {
	if s.resources != nil {
		s.resources.StopAll()
	}
	s.ApiServerSuite.TearDownTest(c)
}

func (s *upgraderSuite) TestToolsNothing(c *gc.C) {
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
	systemState, err := s.StatePool().SystemState()
	c.Assert(err, jc.ErrorIsNil)

	domainServices := s.DefaultModelDomainServices(c)

	anUpgrader, err := upgrader.NewUpgraderAPI(
		s.controllerConfigGetter, systemState, s.hosted, anAuthorizer,
		loggertesting.WrapCheckLog(c),
		domainServices.Cloud(),
		domainServices.Credential(),
		domainServices.Config(),
		domainServices.Agent(),
		s.store,
		s.watcherRegistry,
	)
	c.Check(err, jc.ErrorIsNil)
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
	v := version.Binary{
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

func (s *upgraderSuite) TestSetToolsNothing(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Not an error to watch nothing
	results, err := s.upgrader.SetTools(context.Background(), params.EntitiesVersion{})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(results.Results, gc.HasLen, 0)
}

func (s *upgraderSuite) TestSetToolsRefusesWrongAgent(c *gc.C) {
	defer s.setupMocks(c).Finish()

	anAuthorizer := s.authorizer
	anAuthorizer.Tag = names.NewMachineTag("12354")
	systemState, err := s.StatePool().SystemState()
	c.Assert(err, jc.ErrorIsNil)

	domainServices := s.DefaultModelDomainServices(c)

	anUpgrader, err := upgrader.NewUpgraderAPI(
		s.controllerConfigGetter, systemState, s.hosted, anAuthorizer,
		loggertesting.WrapCheckLog(c),
		domainServices.Cloud(),
		domainServices.Credential(),
		domainServices.Config(),
		domainServices.Agent(),
		s.store,
		s.watcherRegistry,
	)
	c.Check(err, jc.ErrorIsNil)
	args := params.EntitiesVersion{
		AgentTools: []params.EntityVersion{{
			Tag: s.rawMachine.Tag().String(),
			Tools: &params.Version{
				Version: coretesting.CurrentVersion(),
			},
		}},
	}

	results, err := anUpgrader.SetTools(context.Background(), args)
	c.Check(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.DeepEquals, apiservertesting.ErrUnauthorized)
}

func (s *upgraderSuite) TestSetTools(c *gc.C) {
	defer s.setupMocks(c).Finish()

	current := coretesting.CurrentVersion()
	_, err := s.rawMachine.AgentTools()
	c.Assert(err, jc.ErrorIs, errors.NotFound)
	args := params.EntitiesVersion{
		AgentTools: []params.EntityVersion{{
			Tag: s.rawMachine.Tag().String(),
			Tools: &params.Version{
				Version: current,
			}},
		},
	}
	results, err := s.upgrader.SetTools(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)
	// Check that the new value actually got set, we must Refresh because
	// it was set on a different Machine object
	err = s.rawMachine.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	realTools, err := s.rawMachine.AgentTools()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(realTools.Version, gc.Equals, current)
	c.Check(realTools.URL, gc.Equals, "")
}

func (s *upgraderSuite) TestDesiredVersionNothing(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Not an error to watch nothing
	results, err := s.upgrader.DesiredVersion(context.Background(), params.Entities{})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(results.Results, gc.HasLen, 0)
}

func (s *upgraderSuite) TestDesiredVersionRefusesWrongAgent(c *gc.C) {
	defer s.setupMocks(c).Finish()

	anAuthorizer := s.authorizer
	anAuthorizer.Tag = names.NewMachineTag("12354")
	systemState, err := s.StatePool().SystemState()
	c.Assert(err, jc.ErrorIsNil)

	domainServices := s.DefaultModelDomainServices(c)

	anUpgrader, err := upgrader.NewUpgraderAPI(
		s.controllerConfigGetter, systemState, s.hosted, anAuthorizer,
		loggertesting.WrapCheckLog(c),
		domainServices.Cloud(),
		domainServices.Credential(),
		domainServices.Config(),
		domainServices.Agent(),
		s.store,
		s.watcherRegistry,
	)
	c.Check(err, jc.ErrorIsNil)
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
	defer s.setupMocks(c).Finish()

	newVersion := coretesting.CurrentVersion()
	newVersion.Patch++
	s.agentService.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(newVersion.Number, nil)

	// Grab a different Upgrader for the apiMachine
	authorizer := apiservertesting.FakeAuthorizer{
		Tag: s.apiMachine.Tag(),
	}
	systemState, err := s.StatePool().SystemState()
	c.Assert(err, jc.ErrorIsNil)

	domainServices := s.DefaultModelDomainServices(c)

	upgraderAPI, err := upgrader.NewUpgraderAPI(
		s.controllerConfigGetter, systemState, s.hosted, authorizer,
		loggertesting.WrapCheckLog(c),
		domainServices.Cloud(),
		domainServices.Credential(),
		domainServices.Config(),
		s.agentService, s.store, s.watcherRegistry,
	)
	c.Assert(err, jc.ErrorIsNil)
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

func (s *upgraderSuite) setupMocks(c *gc.C) *gomock.Controller {

	ctrl := gomock.NewController(c)

	s.controllerConfigGetter = NewMockControllerConfigGetter(ctrl)
	s.agentService = NewMockModelAgentService(ctrl)
	s.isUpgrader = NewMockUpgrader(ctrl)
	s.isUpgrader.EXPECT().IsUpgrading().Return(false, nil).AnyTimes()
	s.watcherRegistry = facademocks.NewMockWatcherRegistry(ctrl)

	return ctrl
}
