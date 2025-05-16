// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package stateconverter_test

import (
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/watcher"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/worker/stateconverter"
	"github.com/juju/juju/internal/worker/stateconverter/mocks"
)

func TestConverterSuite(t *stdtesting.T) { tc.Run(t, &converterSuite{}) }

type converterSuite struct {
	machine  *mocks.MockMachine
	machiner *mocks.MockMachiner
}

func (s *converterSuite) TestSetUp(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.machiner.EXPECT().Machine(gomock.Any(), gomock.Any()).Return(s.machine, nil)
	s.machine.EXPECT().Watch(gomock.Any()).Return(nil, nil)

	conv := s.newConverter(c)
	_, err := conv.SetUp(c.Context())
	c.Assert(err, tc.ErrorIsNil)
}

func (s *converterSuite) TestSetupMachinerErr(c *tc.C) {
	defer s.setupMocks(c).Finish()
	expectedError := errors.NotValidf("machine tag")
	s.machiner.EXPECT().Machine(gomock.Any(), gomock.Any()).Return(nil, expectedError)

	conv := s.newConverter(c)
	w, err := conv.SetUp(c.Context())
	c.Assert(err, tc.ErrorIs, errors.NotValid)
	c.Assert(w, tc.IsNil)
}

func (s *converterSuite) TestSetupWatchErr(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.machiner.EXPECT().Machine(gomock.Any(), gomock.Any()).Return(s.machine, nil)
	expectedError := errors.NotValidf("machine tag")
	s.machine.EXPECT().Watch(gomock.Any()).Return(nil, expectedError)

	conv := s.newConverter(c)
	w, err := conv.SetUp(c.Context())
	c.Assert(err, tc.ErrorIs, errors.NotValid)
	c.Assert(w, tc.IsNil)
}

func (s *converterSuite) TestHandle(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.machiner.EXPECT().Machine(gomock.Any(), gomock.Any()).Return(s.machine, nil)
	s.machine.EXPECT().Watch(gomock.Any()).Return(nil, nil)
	s.machine.EXPECT().IsController(gomock.Any(), gomock.Any()).Return(true, nil)

	conv := s.newConverter(c)
	_, err := conv.SetUp(c.Context())
	c.Assert(err, tc.IsNil)
	err = conv.Handle(c.Context())
	// Since machine has model.JobManageModel, we expect an error
	// which will get machineTag to restart.
	c.Assert(err.Error(), tc.Equals, "bounce agent to pick up new jobs")
}

func (s *converterSuite) TestHandleNotController(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.machiner.EXPECT().Machine(gomock.Any(), gomock.Any()).Return(s.machine, nil)
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
	s.machiner.EXPECT().Machine(gomock.Any(), gomock.Any()).Return(s.machine, nil).AnyTimes()
	s.machine.EXPECT().Watch(gomock.Any()).Return(nil, nil).AnyTimes()
	s.machine.EXPECT().IsController(gomock.Any(), gomock.Any()).Return(true, nil)
	expectedError := errors.New("foo")
	s.machine.EXPECT().IsController(gomock.Any(), gomock.Any()).Return(false, expectedError)

	conv := s.newConverter(c)
	_, err := conv.SetUp(c.Context())
	c.Assert(err, tc.IsNil)
	err = conv.Handle(c.Context())
	// Since machine has model.JobManageModel, we expect an error
	// which will get machineTag to restart.
	c.Assert(err.Error(), tc.Equals, "bounce agent to pick up new jobs")
	_, err = conv.SetUp(c.Context())
	c.Assert(err, tc.IsNil)
	err = conv.Handle(c.Context())
	c.Assert(errors.Cause(err), tc.Equals, expectedError)
}

func (s *converterSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.machine = mocks.NewMockMachine(ctrl)
	s.machiner = mocks.NewMockMachiner(ctrl)
	return ctrl
}

func (s *converterSuite) newConverter(c *tc.C) watcher.NotifyHandler {
	return stateconverter.NewConverterForTest(s.machine, s.machiner, loggertesting.WrapCheckLog(c))
}
