// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package stateconverter

import (
	"testing"

	"github.com/juju/errors"
	names "github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	agent "github.com/juju/juju/agent"
	agenterrors "github.com/juju/juju/agent/errors"
	controller "github.com/juju/juju/controller"
	"github.com/juju/juju/core/watcher"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

func TestConverterSuite(t *testing.T) {
	tc.Run(t, &converterSuite{})
}

type converterSuite struct {
	agent             *MockAgent
	agentConfig       *MockConfig
	agentConfigSetter *MockConfigSetter
	machine           *MockMachine
	machineClient     *MockMachineClient
	agentClient       *MockAgentClient
}

func (s *converterSuite) TestSetUp(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.machineClient.EXPECT().Machine(gomock.Any(), gomock.Any()).Return(s.machine, nil)
	s.machine.EXPECT().Watch(gomock.Any()).Return(nil, nil)

	conv := s.newConverter(c)
	_, err := conv.SetUp(c.Context())
	c.Assert(err, tc.ErrorIsNil)
}

func (s *converterSuite) TestSetupMachineClientErr(c *tc.C) {
	defer s.setupMocks(c).Finish()
	expectedError := errors.NotValidf("machine tag")
	s.machineClient.EXPECT().Machine(gomock.Any(), gomock.Any()).Return(nil, expectedError)

	conv := s.newConverter(c)
	w, err := conv.SetUp(c.Context())
	c.Assert(err, tc.ErrorIs, errors.NotValid)
	c.Assert(w, tc.IsNil)
}

func (s *converterSuite) TestSetupWatchErr(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.machineClient.EXPECT().Machine(gomock.Any(), gomock.Any()).Return(s.machine, nil)
	expectedError := errors.NotValidf("machine tag")
	s.machine.EXPECT().Watch(gomock.Any()).Return(nil, expectedError)

	conv := s.newConverter(c)
	w, err := conv.SetUp(c.Context())
	c.Assert(err, tc.ErrorIs, errors.NotValid)
	c.Assert(w, tc.IsNil)
}

func (s *converterSuite) TestHandle(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.machineClient.EXPECT().Machine(gomock.Any(), gomock.Any()).Return(s.machine, nil)
	s.machine.EXPECT().Watch(gomock.Any()).Return(nil, nil)
	s.machine.EXPECT().IsController(gomock.Any(), gomock.Any()).Return(true, nil)

	// Notice that we ignore what was already set and just replace what
	// the agent has set.
	s.agentClient.EXPECT().StateServingInfo(gomock.Any()).Return(controller.StateServingInfo{
		APIPort: 1234,
	}, nil)
	s.agentConfigSetter.EXPECT().StateServingInfo().Return(controller.StateServingInfo{
		APIPort: 4321,
	}, false)
	s.agentConfigSetter.EXPECT().SetStateServingInfo(controller.StateServingInfo{
		APIPort: 1234,
	})

	s.agent.EXPECT().ChangeConfig(gomock.Any()).DoAndReturn(func(f agent.ConfigMutator) error {
		return f(s.agentConfigSetter)
	})

	conv := s.newConverter(c)
	_, err := conv.SetUp(c.Context())
	c.Assert(err, tc.IsNil)
	err = conv.Handle(c.Context())
	// Since machine has model.JobManageModel, we expect an error
	// which will get machineTag to restart.
	c.Assert(err, tc.ErrorIs, agenterrors.FatalError)
}

func (s *converterSuite) TestHandleAlreadyHasInfo(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.machineClient.EXPECT().Machine(gomock.Any(), gomock.Any()).Return(s.machine, nil)
	s.machine.EXPECT().Watch(gomock.Any()).Return(nil, nil)
	s.machine.EXPECT().IsController(gomock.Any(), gomock.Any()).Return(true, nil)

	// If the information is already set, prevent the update from happening.
	s.agentClient.EXPECT().StateServingInfo(gomock.Any()).Return(controller.StateServingInfo{
		APIPort: 1234,
	}, nil)
	s.agentConfigSetter.EXPECT().StateServingInfo().Return(controller.StateServingInfo{
		APIPort: 4321,
	}, true)

	s.agent.EXPECT().ChangeConfig(gomock.Any()).DoAndReturn(func(f agent.ConfigMutator) error {
		return f(s.agentConfigSetter)
	})

	conv := s.newConverter(c)
	_, err := conv.SetUp(c.Context())
	c.Assert(err, tc.IsNil)
	err = conv.Handle(c.Context())
	// Since machine has model.JobManageModel, we expect an error
	// which will get machineTag to restart.
	c.Assert(err, tc.ErrorIs, agenterrors.FatalError)
}

func (s *converterSuite) TestHandleNotController(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.machineClient.EXPECT().Machine(gomock.Any(), gomock.Any()).Return(s.machine, nil)
	s.machine.EXPECT().Watch(gomock.Any()).Return(nil, nil)
	s.machine.EXPECT().IsController(gomock.Any(), gomock.Any()).Return(false, nil)

	conv := s.newConverter(c)
	_, err := conv.SetUp(c.Context())
	c.Assert(err, tc.IsNil)
	err = conv.Handle(c.Context())
	c.Assert(err, tc.IsNil)
}

func (s *converterSuite) TestHandleJobsError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.machineClient.EXPECT().Machine(gomock.Any(), gomock.Any()).Return(s.machine, nil).AnyTimes()
	s.machine.EXPECT().Watch(gomock.Any()).Return(nil, nil).AnyTimes()
	s.machine.EXPECT().IsController(gomock.Any(), gomock.Any()).Return(true, errors.New("foo"))

	conv := s.newConverter(c)
	_, err := conv.SetUp(c.Context())
	c.Assert(err, tc.IsNil)

	err = conv.Handle(c.Context())
	c.Assert(err, tc.ErrorMatches, "foo")
}

func (s *converterSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.agentConfig = NewMockConfig(ctrl)
	s.agentConfigSetter = NewMockConfigSetter(ctrl)

	s.agent = NewMockAgent(ctrl)
	s.agent.EXPECT().CurrentConfig().Return(s.agentConfig).AnyTimes()

	s.machine = NewMockMachine(ctrl)
	s.machineClient = NewMockMachineClient(ctrl)
	s.agentClient = NewMockAgentClient(ctrl)

	return ctrl
}

func (s *converterSuite) newConverter(c *tc.C) watcher.NotifyHandler {
	handler, err := NewConverter(Config{
		agent:         s.agent,
		machineTag:    names.NewMachineTag("3"),
		machineClient: s.machineClient,
		agentClient:   s.agentClient,
		logger:        loggertesting.WrapCheckLog(c),
	})
	c.Assert(err, tc.ErrorIsNil)
	return handler
}
