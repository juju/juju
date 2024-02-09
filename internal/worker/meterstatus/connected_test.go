// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package meterstatus_test

import (
	"context"
	"fmt"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/worker/common/charmrunner"
	"github.com/juju/juju/internal/worker/meterstatus"
	"github.com/juju/juju/internal/worker/meterstatus/mocks"
	coretesting "github.com/juju/juju/testing"
)

type ConnectedWorkerSuite struct {
	coretesting.BaseSuite

	stub *testing.Stub

	msClient *stubMeterStatusClient
	config   meterstatus.ConnectedConfig
}

var _ = gc.Suite(&ConnectedWorkerSuite{})

func (s *ConnectedWorkerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.stub = &testing.Stub{}

	s.msClient = newStubMeterStatusClient(s.stub)

	s.config = meterstatus.ConnectedConfig{
		Runner:          &stubRunner{stub: s.stub},
		Status:          s.msClient,
		StateReadWriter: struct{ meterstatus.StateReadWriter }{},
		Logger:          loggo.GetLogger("test"),
	}
}

func assertSignal(c *gc.C, signal <-chan struct{}) {
	select {
	case <-signal:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for signal")
	}
}

func (s *ConnectedWorkerSuite) TestConfigValid(c *gc.C) {
	c.Assert(s.config.Validate(), jc.ErrorIsNil)
}

func (s *ConnectedWorkerSuite) TestConfigMissingRunner(c *gc.C) {
	s.config.Runner = nil
	err := s.config.Validate()
	c.Assert(err, jc.ErrorIs, errors.NotValid)
	c.Assert(err.Error(), gc.Equals, "missing Runner not valid")
}

func (s *ConnectedWorkerSuite) TestConfigMissingStateReadWriter(c *gc.C) {
	s.config.StateReadWriter = nil
	err := s.config.Validate()
	c.Assert(err, jc.ErrorIs, errors.NotValid)
	c.Assert(err.Error(), gc.Equals, "missing StateReadWriter not valid")
}

func (s *ConnectedWorkerSuite) TestConfigMissingStatus(c *gc.C) {
	s.config.Status = nil
	err := s.config.Validate()
	c.Assert(err, jc.ErrorIs, errors.NotValid)
	c.Assert(err.Error(), gc.Equals, "missing Status not valid")
}

func (s *ConnectedWorkerSuite) TestConfigMissingLogger(c *gc.C) {
	s.config.Logger = nil
	err := s.config.Validate()
	c.Assert(err, jc.ErrorIs, errors.NotValid)
	c.Assert(err.Error(), gc.Equals, "missing Logger not valid")
}

// TestStatusHandlerDoesNotRerunNoChange ensures that the handler does not execute the hook if it
// detects no actual meter status change.
func (s *ConnectedWorkerSuite) TestStatusHandlerDoesNotRerunNoChange(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	stateReadWriter := mocks.NewMockStateReadWriter(ctrl)
	gomock.InOrder(
		stateReadWriter.EXPECT().Read().Return(nil, errors.NotFoundf("no state")),
		stateReadWriter.EXPECT().Write(&meterstatus.State{
			Code: "GREEN",
		}).Return(nil),
	)

	config := s.config
	config.StateReadWriter = stateReadWriter
	handler, err := meterstatus.NewConnectedStatusHandler(config)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(handler, gc.NotNil)
	_, err = handler.SetUp(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	err = handler.Handle(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	err = handler.Handle(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "WatchMeterStatus", "MeterStatus", "RunHook", "MeterStatus")
}

// TestStatusHandlerRunsHookOnChanges ensures that the handler runs the meter-status-changed hook
// if an actual meter status change is detected.
func (s *ConnectedWorkerSuite) TestStatusHandlerRunsHookOnChanges(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	stateReadWriter := mocks.NewMockStateReadWriter(ctrl)
	gomock.InOrder(
		stateReadWriter.EXPECT().Read().Return(nil, errors.NotFoundf("no state")),
		// First Handle() invocation
		stateReadWriter.EXPECT().Write(&meterstatus.State{
			Code: "GREEN",
		}).Return(nil),
		// Second Handle() invocation
		stateReadWriter.EXPECT().Write(&meterstatus.State{
			Code: "RED",
		}).Return(nil),
	)

	config := s.config
	config.StateReadWriter = stateReadWriter
	handler, err := meterstatus.NewConnectedStatusHandler(config)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(handler, gc.NotNil)
	_, err = handler.SetUp(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	handler.Handle(context.Background())
	s.msClient.SetStatus("RED")
	handler.Handle(context.Background())

	c.Assert(err, jc.ErrorIsNil)
	s.stub.CheckCallNames(c, "WatchMeterStatus", "MeterStatus", "RunHook", "MeterStatus", "RunHook")
}

// TestStatusHandlerHandlesHookMissingError tests that the handler does not report errors
// caused by a missing meter-status-changed hook.
func (s *ConnectedWorkerSuite) TestStatusHandlerHandlesHookMissingError(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	stateReadWriter := mocks.NewMockStateReadWriter(ctrl)
	gomock.InOrder(
		stateReadWriter.EXPECT().Read().Return(nil, errors.NotFoundf("no state")),
		stateReadWriter.EXPECT().Write(&meterstatus.State{
			Code: "GREEN",
		}).Return(nil),
	)

	s.stub.SetErrors(charmrunner.NewMissingHookError("meter-status-changed"))
	config := s.config
	config.StateReadWriter = stateReadWriter
	handler, err := meterstatus.NewConnectedStatusHandler(config)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(handler, gc.NotNil)
	_, err = handler.SetUp(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	err = handler.Handle(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	s.stub.CheckCallNames(c, "WatchMeterStatus", "MeterStatus", "RunHook")
}

// TestStatusHandlerHandlesRandomHookError tests that the meter status handler does not return
// errors encountered while executing the hook.
func (s *ConnectedWorkerSuite) TestStatusHandlerHandlesRandomHookError(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	stateReadWriter := mocks.NewMockStateReadWriter(ctrl)
	gomock.InOrder(
		stateReadWriter.EXPECT().Read().Return(nil, errors.NotFoundf("no state")),
		stateReadWriter.EXPECT().Write(&meterstatus.State{
			Code: "GREEN",
		}).Return(nil),
	)

	s.stub.SetErrors(fmt.Errorf("blah"))
	config := s.config
	config.StateReadWriter = stateReadWriter
	handler, err := meterstatus.NewConnectedStatusHandler(config)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(handler, gc.NotNil)
	_, err = handler.SetUp(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	err = handler.Handle(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "WatchMeterStatus", "MeterStatus", "RunHook")
}

// TestStatusHandlerDoesNotRerunAfterRestart tests that the status handler will not rerun a meter-status-changed
// hook if it is restarted, but no actual changes are recorded.
func (s *ConnectedWorkerSuite) TestStatusHandlerDoesNotRerunAfterRestart(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	stateReadWriter := mocks.NewMockStateReadWriter(ctrl)
	gomock.InOrder(
		// First run
		stateReadWriter.EXPECT().Read().Return(nil, errors.NotFoundf("no state")),
		stateReadWriter.EXPECT().Write(&meterstatus.State{
			Code: "GREEN",
		}).Return(nil),

		// Second run
		stateReadWriter.EXPECT().Read().Return(&meterstatus.State{
			Code: "GREEN",
		}, nil),
	)

	config := s.config
	config.StateReadWriter = stateReadWriter
	handler, err := meterstatus.NewConnectedStatusHandler(config)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(handler, gc.NotNil)
	_, err = handler.SetUp(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	err = handler.Handle(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "WatchMeterStatus", "MeterStatus", "RunHook")
	s.stub.ResetCalls()

	// Create a new handler (imitating worker restart).
	handler, err = meterstatus.NewConnectedStatusHandler(config)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(handler, gc.NotNil)
	_, err = handler.SetUp(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	err = handler.Handle(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "WatchMeterStatus", "MeterStatus")
}
