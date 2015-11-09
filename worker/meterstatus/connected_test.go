// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package meterstatus_test

import (
	"fmt"
	"path"
	"time"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/fslock"
	gc "gopkg.in/check.v1"

	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/meterstatus"
	"github.com/juju/juju/worker/uniter/runner/context"
)

type ConnectedWorkerSuite struct {
	coretesting.BaseSuite

	stub *testing.Stub

	dataDir  string
	lock     *fslock.Lock
	msClient *stubMeterStatusClient
}

var _ = gc.Suite(&ConnectedWorkerSuite{})

func (s *ConnectedWorkerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.stub = &testing.Stub{}

	s.dataDir = c.MkDir()

	s.msClient = newStubMeterStatusClient(s.stub)
}

func assertSignal(c *gc.C, signal <-chan struct{}) {
	select {
	case <-signal:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for signal")
	}
}

func assertNoSignal(c *gc.C, signal <-chan struct{}) {
	select {
	case <-signal:
		c.Fatal("unexpected signal")
	case <-time.After(coretesting.ShortWait):
	}
}

func (s *ConnectedWorkerSuite) TestConfigValidation(c *gc.C) {
	tests := []struct {
		cfg      meterstatus.ConnectedConfig
		expected string
	}{{
		cfg: meterstatus.ConnectedConfig{
			Status:    s.msClient,
			StateFile: meterstatus.NewStateFile(path.Join(s.dataDir, "meter-status.yaml")),
		},
		expected: "hook runner not provided",
	}, {
		cfg: meterstatus.ConnectedConfig{
			StateFile: meterstatus.NewStateFile(path.Join(s.dataDir, "meter-status.yaml")),
			Runner:    &stubRunner{stub: s.stub},
		},
		expected: "meter status API client not provided",
	}, {
		cfg: meterstatus.ConnectedConfig{
			Status: s.msClient,
			Runner: &stubRunner{stub: s.stub},
		},
		expected: "state file not provided",
	}}
	for i, test := range tests {
		c.Logf("running test %d", i)
		err := test.cfg.Validate()
		c.Assert(err, gc.ErrorMatches, test.expected)
	}
}

// TestStatusHandlerDoesNotRerunNoChange ensures that the handler does not execute the hook if it
// detects no actual meter status change.
func (s *ConnectedWorkerSuite) TestStatusHandlerDoesNotRerunNoChange(c *gc.C) {
	handler, err := meterstatus.NewConnectedStatusHandler(
		meterstatus.ConnectedConfig{
			Runner:    &stubRunner{stub: s.stub},
			StateFile: meterstatus.NewStateFile(path.Join(s.dataDir, "meter-status.yaml")),
			Status:    s.msClient,
		})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(handler, gc.NotNil)
	_, err = handler.SetUp()
	c.Assert(err, jc.ErrorIsNil)

	err = handler.Handle(nil)
	c.Assert(err, jc.ErrorIsNil)
	err = handler.Handle(nil)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "WatchMeterStatus", "MeterStatus", "RunHook", "MeterStatus")
}

// TestStatusHandlerRunsHookOnChanges ensures that the handler runs the meter-status-changed hook
// if an actual meter status change is detected.
func (s *ConnectedWorkerSuite) TestStatusHandlerRunsHookOnChanges(c *gc.C) {
	handler, err := meterstatus.NewConnectedStatusHandler(
		meterstatus.ConnectedConfig{
			Runner:    &stubRunner{stub: s.stub},
			StateFile: meterstatus.NewStateFile(path.Join(s.dataDir, "meter-status.yaml")),
			Status:    s.msClient,
		})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(handler, gc.NotNil)
	_, err = handler.SetUp()
	c.Assert(err, jc.ErrorIsNil)

	handler.Handle(nil)
	s.msClient.SetStatus("RED")
	handler.Handle(nil)

	c.Assert(err, jc.ErrorIsNil)
	s.stub.CheckCallNames(c, "WatchMeterStatus", "MeterStatus", "RunHook", "MeterStatus", "RunHook")
}

// TestStatusHandlerHandlesHookMissingError tests that the handler does not report errors
// caused by a missing meter-status-changed hook.
func (s *ConnectedWorkerSuite) TestStatusHandlerHandlesHookMissingError(c *gc.C) {
	s.stub.SetErrors(context.NewMissingHookError("meter-status-changed"))
	handler, err := meterstatus.NewConnectedStatusHandler(
		meterstatus.ConnectedConfig{
			Runner:    &stubRunner{stub: s.stub},
			StateFile: meterstatus.NewStateFile(path.Join(s.dataDir, "meter-status.yaml")),
			Status:    s.msClient,
		})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(handler, gc.NotNil)
	_, err = handler.SetUp()
	c.Assert(err, jc.ErrorIsNil)

	err = handler.Handle(nil)
	c.Assert(err, jc.ErrorIsNil)
	s.stub.CheckCallNames(c, "WatchMeterStatus", "MeterStatus", "RunHook")
}

// TestStatusHandlerHandlesRandomHookError tests that the meter status handler does not return
// errors encountered while executing the hook.
func (s *ConnectedWorkerSuite) TestStatusHandlerHandlesRandomHookError(c *gc.C) {
	s.stub.SetErrors(fmt.Errorf("blah"))
	handler, err := meterstatus.NewConnectedStatusHandler(
		meterstatus.ConnectedConfig{
			Runner:    &stubRunner{stub: s.stub},
			StateFile: meterstatus.NewStateFile(path.Join(s.dataDir, "meter-status.yaml")),
			Status:    s.msClient,
		})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(handler, gc.NotNil)
	_, err = handler.SetUp()
	c.Assert(err, jc.ErrorIsNil)

	err = handler.Handle(nil)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "WatchMeterStatus", "MeterStatus", "RunHook")
}

// TestStatusHandlerDoesNotRerunAfterRestart tests that the status handler will not rerun a meter-status-changed
// hook if it is restarted, but no actual changes are recorded.
func (s *ConnectedWorkerSuite) TestStatusHandlerDoesNotRerunAfterRestart(c *gc.C) {
	handler, err := meterstatus.NewConnectedStatusHandler(
		meterstatus.ConnectedConfig{
			Runner:    &stubRunner{stub: s.stub},
			StateFile: meterstatus.NewStateFile(path.Join(s.dataDir, "meter-status.yaml")),
			Status:    s.msClient,
		})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(handler, gc.NotNil)
	_, err = handler.SetUp()
	c.Assert(err, jc.ErrorIsNil)

	err = handler.Handle(nil)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "WatchMeterStatus", "MeterStatus", "RunHook")
	s.stub.ResetCalls()

	// Create a new handler (imitating worker restart).
	handler, err = meterstatus.NewConnectedStatusHandler(
		meterstatus.ConnectedConfig{
			Runner:    &stubRunner{stub: s.stub},
			StateFile: meterstatus.NewStateFile(path.Join(s.dataDir, "meter-status.yaml")),
			Status:    s.msClient})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(handler, gc.NotNil)
	_, err = handler.SetUp()
	c.Assert(err, jc.ErrorIsNil)

	err = handler.Handle(nil)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "WatchMeterStatus", "MeterStatus")
}
