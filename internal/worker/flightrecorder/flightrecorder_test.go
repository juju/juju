// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package flightrecorder

import (
	"testing"
	"time"

	"github.com/juju/tc"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/goleak"
	gomock "go.uber.org/mock/gomock"

	"github.com/juju/juju/core/flightrecorder"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

func TestFlightRecorderWorker(t *testing.T) {
	defer goleak.VerifyNone(t)
	tc.Run(t, &flightRecorderSuite{})
}

type flightRecorderSuite struct {
	recorder *MockFileRecorder
}

func (s *flightRecorderSuite) TestWorkerStart(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.recorder.EXPECT().Stop().Return(nil)

	recorder := New(s.recorder, "/tmp", loggertesting.WrapCheckLog(c))
	defer workertest.DirtyKill(c, recorder)

	workertest.CleanKill(c, recorder)
}

func (s *flightRecorderSuite) TestStart(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.recorder.EXPECT().Start(time.Millisecond).Return(nil)
	s.recorder.EXPECT().Stop().Return(nil)

	recorder := New(s.recorder, "/tmp", loggertesting.WrapCheckLog(c))
	defer workertest.DirtyKill(c, recorder)

	err := recorder.Start(flightrecorder.KindAll, time.Millisecond)
	c.Assert(err, tc.ErrorIsNil)

	workertest.CleanKill(c, recorder)
}

func (s *flightRecorderSuite) TestStartStop(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.recorder.EXPECT().Start(time.Millisecond).Return(nil)
	s.recorder.EXPECT().Stop().Return(nil).Times(2)

	recorder := New(s.recorder, "/tmp", loggertesting.WrapCheckLog(c))
	defer workertest.DirtyKill(c, recorder)

	err := recorder.Start(flightrecorder.KindAll, time.Millisecond)
	c.Assert(err, tc.ErrorIsNil)

	err = recorder.Stop()
	c.Assert(err, tc.ErrorIsNil)

	workertest.CleanKill(c, recorder)
}

func (s *flightRecorderSuite) TestStartCapture(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.recorder.EXPECT().Start(time.Millisecond).Return(nil)
	s.recorder.EXPECT().Capture("/mytmp").Return("path/to/recording", nil)
	s.recorder.EXPECT().Stop().Return(nil)

	recorder := New(s.recorder, "/mytmp", loggertesting.WrapCheckLog(c))
	defer workertest.DirtyKill(c, recorder)

	err := recorder.Start(flightrecorder.KindAll, time.Millisecond)
	c.Assert(err, tc.ErrorIsNil)

	err = recorder.Capture(flightrecorder.KindAll)
	c.Assert(err, tc.ErrorIsNil)

	workertest.CleanKill(c, recorder)
}

func (s *flightRecorderSuite) TestStartCaptureDefaultPath(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.recorder.EXPECT().Start(time.Millisecond).Return(nil)
	s.recorder.EXPECT().Capture("/tmp").Return("path/to/recording", nil)
	s.recorder.EXPECT().Stop().Return(nil)

	recorder := New(s.recorder, "", loggertesting.WrapCheckLog(c))
	defer workertest.DirtyKill(c, recorder)

	err := recorder.Start(flightrecorder.KindAll, time.Millisecond)
	c.Assert(err, tc.ErrorIsNil)

	err = recorder.Capture(flightrecorder.KindAll)
	c.Assert(err, tc.ErrorIsNil)

	workertest.CleanKill(c, recorder)
}

func (s *flightRecorderSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.recorder = NewMockFileRecorder(ctrl)

	return ctrl
}
