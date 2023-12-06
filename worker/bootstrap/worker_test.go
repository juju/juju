// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"
	time "time"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/workertest"
	gomock "go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	controller "github.com/juju/juju/controller"
	"github.com/juju/juju/core/flags"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
)

type workerSuite struct {
	baseSuite

	states chan string
}

var _ = gc.Suite(&workerSuite{})

func (s *workerSuite) TestKilled(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectGateUnlock()
	s.expectControllerConfig()
	s.expectAgentConfig(c)
	s.expectObjectStoreGetter()
	s.expectBootstrapFlagSet()

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)
	s.ensureFinished(c)

	workertest.CleanKill(c, w)
}

func (s *workerSuite) TestSeedAgentBinary(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Ensure that the ControllerModelUUID is used for the namespace for the
	// object store. If it's not the controller model uuid, then the agent
	// binary will not be found.

	uuid := utils.MustNewUUID().String()

	s.state.EXPECT().ControllerModelUUID().Return(uuid)
	s.objectStoreGetter.EXPECT().GetObjectStore(gomock.Any(), uuid).Return(s.objectStore, nil)

	var called bool
	w := &bootstrapWorker{
		internalStates: s.states,
		cfg: WorkerConfig{
			ObjectStoreGetter: s.objectStoreGetter,
			AgentBinaryUploader: func(context.Context, string, BinaryAgentStorageService, objectstore.ObjectStore, Logger) error {
				called = true
				return nil
			},
			State:  s.state,
			Logger: s.logger,
		},
	}
	err := w.seedAgentBinary(context.Background(), c.MkDir())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
}

func (s *workerSuite) newWorker(c *gc.C) worker.Worker {
	w, err := newWorker(WorkerConfig{
		Logger:            s.logger,
		Agent:             s.agent,
		ObjectStoreGetter: s.objectStoreGetter,
		BootstrapUnlocker: s.bootstrapUnlocker,
		AgentBinaryUploader: func(context.Context, string, BinaryAgentStorageService, objectstore.ObjectStore, Logger) error {
			return nil
		},
		State:                   &state.State{},
		ControllerConfigService: s.controllerConfigService,
		FlagService:             s.flagService,
	}, s.states)
	c.Assert(err, jc.ErrorIsNil)
	return w
}

func (s *workerSuite) setupMocks(c *gc.C) *gomock.Controller {
	// Ensure we buffer the channel, this is because we might miss the
	// event if we're too quick at starting up.
	s.states = make(chan string, 1)

	ctrl := s.baseSuite.setupMocks(c)

	return ctrl
}

func (s *workerSuite) ensureStartup(c *gc.C) {
	s.ensureState(c, stateStarted)
}

func (s *workerSuite) ensureFinished(c *gc.C) {
	s.ensureState(c, stateCompleted)
}

func (s *workerSuite) ensureState(c *gc.C, st string) {
	select {
	case state := <-s.states:
		c.Assert(state, gc.Equals, st)
	case <-time.After(testing.ShortWait * 10):
		c.Fatalf("timed out waiting for %s", st)
	}
}

func (s *workerSuite) expectControllerConfig() {
	s.controllerConfigService.EXPECT().ControllerConfig(gomock.Any()).Return(controller.Config{}, nil).AnyTimes()
}

func (s *workerSuite) expectObjectStoreGetter() {
	s.objectStoreGetter.EXPECT().GetObjectStore(gomock.Any(), gomock.Any()).Return(s.objectStore, nil)
}

func (s *workerSuite) expectBootstrapFlagSet() {
	s.flagService.EXPECT().SetFlag(gomock.Any(), flags.BootstrapFlag, true).Return(nil)
}
