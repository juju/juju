// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package stateconverter_test

import (
	"context"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/worker/stateconverter"
	"github.com/juju/juju/worker/stateconverter/mocks"
)

var _ = gc.Suite(&converterSuite{})

type converterSuite struct {
	machine  *mocks.MockMachine
	machiner *mocks.MockMachiner
}

func (s *converterSuite) TestSetUp(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.machiner.EXPECT().Machine(gomock.Any()).Return(s.machine, nil)
	s.machine.EXPECT().Watch().Return(nil, nil)

	conv := s.newConverter()
	_, err := conv.SetUp(context.Background())
	c.Assert(err, jc.ErrorIsNil)
}

func (s *converterSuite) TestSetupMachinerErr(c *gc.C) {
	defer s.setupMocks(c).Finish()
	expectedError := errors.NotValidf("machine tag")
	s.machiner.EXPECT().Machine(gomock.Any()).Return(nil, expectedError)

	conv := s.newConverter()
	w, err := conv.SetUp(context.Background())
	c.Assert(err, jc.ErrorIs, errors.NotValid)
	c.Assert(w, gc.IsNil)
}

func (s *converterSuite) TestSetupWatchErr(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.machiner.EXPECT().Machine(gomock.Any()).Return(s.machine, nil)
	expectedError := errors.NotValidf("machine tag")
	s.machine.EXPECT().Watch().Return(nil, expectedError)

	conv := s.newConverter()
	w, err := conv.SetUp(context.Background())
	c.Assert(err, jc.ErrorIs, errors.NotValid)
	c.Assert(w, gc.IsNil)
}

func (s *converterSuite) TestHandle(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.machiner.EXPECT().Machine(gomock.Any()).Return(s.machine, nil)
	s.machine.EXPECT().Watch().Return(nil, nil)
	jobs := params.JobsResult{Jobs: []model.MachineJob{model.JobHostUnits, model.JobManageModel}}
	s.machine.EXPECT().Jobs().Return(&jobs, nil)

	conv := s.newConverter()
	_, err := conv.SetUp(context.Background())
	c.Assert(err, gc.IsNil)
	err = conv.Handle(nil)
	// Since machine has model.JobManageModel, we expect an error
	// which will get machineTag to restart.
	c.Assert(err.Error(), gc.Equals, "bounce agent to pick up new jobs")
}

func (s *converterSuite) TestHandleNotController(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.machiner.EXPECT().Machine(gomock.Any()).Return(s.machine, nil)
	s.machine.EXPECT().Watch().Return(nil, nil)
	jobs := params.JobsResult{Jobs: []model.MachineJob{model.JobHostUnits}}
	s.machine.EXPECT().Jobs().Return(&jobs, nil)

	conv := s.newConverter()
	_, err := conv.SetUp(context.Background())
	c.Assert(err, gc.IsNil)
	err = conv.Handle(nil)
	c.Assert(err, gc.IsNil)
}

func (s *converterSuite) TestHandleJobsError(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.machiner.EXPECT().Machine(gomock.Any()).Return(s.machine, nil).AnyTimes()
	s.machine.EXPECT().Watch().Return(nil, nil).AnyTimes()
	jobs := params.JobsResult{Jobs: []model.MachineJob{model.JobHostUnits, model.JobManageModel}}
	s.machine.EXPECT().Jobs().Return(&jobs, nil)
	expectedError := errors.New("foo")
	s.machine.EXPECT().Jobs().Return(nil, expectedError)

	conv := s.newConverter()
	_, err := conv.SetUp(context.Background())
	c.Assert(err, gc.IsNil)
	err = conv.Handle(nil)
	// Since machine has model.JobManageModel, we expect an error
	// which will get machineTag to restart.
	c.Assert(err.Error(), gc.Equals, "bounce agent to pick up new jobs")
	_, err = conv.SetUp(context.Background())
	c.Assert(err, gc.IsNil)
	err = conv.Handle(nil)
	c.Assert(errors.Cause(err), gc.Equals, expectedError)
}

func (s *converterSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.machine = mocks.NewMockMachine(ctrl)
	s.machiner = mocks.NewMockMachiner(ctrl)
	return ctrl
}

func (s *converterSuite) newConverter() watcher.NotifyHandler {
	logger := fakeLogger{}
	return stateconverter.NewConverterForTest(s.machine, s.machiner, &logger)
}
