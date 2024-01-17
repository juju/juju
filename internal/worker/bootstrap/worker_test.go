// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"
	"os"
	"path/filepath"
	time "time"

	"github.com/juju/charm/v12"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/workertest"
	gomock "go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	controller "github.com/juju/juju/controller"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/flags"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/bootstrap"
	"github.com/juju/juju/internal/cloudconfig"
	"github.com/juju/juju/internal/cloudconfig/instancecfg"
	"github.com/juju/juju/testing"
)

type workerSuite struct {
	baseSuite

	states chan string
}

var _ = gc.Suite(&workerSuite{})

func (s *workerSuite) TestKilled(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.ensureBootstrapParams(c)

	s.expectGateUnlock()
	s.expectControllerConfig()
	s.expectAgentConfig(c)
	s.expectObjectStoreGetter(2)
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

	s.expectObjectStoreGetter(1)

	var called bool
	w := &bootstrapWorker{
		internalStates: s.states,
		cfg: WorkerConfig{
			ObjectStoreGetter: s.objectStoreGetter,
			AgentBinaryUploader: func(context.Context, string, BinaryAgentStorageService, objectstore.ObjectStore, Logger) (func(), error) {
				called = true
				return func() {}, nil
			},
			ControllerCharmDeployer: func(ControllerCharmDeployerConfig) (bootstrap.ControllerCharmDeployer, error) {
				return nil, nil
			},
			PopulateControllerCharm: func(context.Context, bootstrap.ControllerCharmDeployer) error {
				return nil
			},
			SystemState:   s.state,
			LoggerFactory: s.loggerFactory,
		},
	}
	cleanup, err := w.seedAgentBinary(context.Background(), c.MkDir())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
	c.Assert(cleanup, gc.NotNil)
}

func (s *workerSuite) newWorker(c *gc.C) worker.Worker {
	w, err := newWorker(WorkerConfig{
		LoggerFactory:           s.loggerFactory,
		Agent:                   s.agent,
		ObjectStoreGetter:       s.objectStoreGetter,
		BootstrapUnlocker:       s.bootstrapUnlocker,
		CharmhubHTTPClient:      s.httpClient,
		SystemState:             s.state,
		ControllerConfigService: s.controllerConfigService,
		FlagService:             s.flagService,
		PopulateControllerCharm: func(context.Context, bootstrap.ControllerCharmDeployer) error {
			return nil
		},
		AgentBinaryUploader: func(context.Context, string, BinaryAgentStorageService, objectstore.ObjectStore, Logger) (func(), error) {
			return func() {}, nil
		},
		ControllerCharmDeployer: func(ControllerCharmDeployerConfig) (bootstrap.ControllerCharmDeployer, error) {
			return nil, nil
		},
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
	s.controllerConfigService.EXPECT().ControllerConfig(gomock.Any()).Return(controller.Config{}, nil)
}

func (s *workerSuite) expectObjectStoreGetter(num int) {
	s.state.EXPECT().ControllerModelUUID().Return(utils.MustNewUUID().String()).Times(num)
	s.objectStoreGetter.EXPECT().GetObjectStore(gomock.Any(), gomock.Any()).Return(s.objectStore, nil).Times(num)
}

func (s *workerSuite) expectBootstrapFlagSet() {
	s.flagService.EXPECT().SetFlag(gomock.Any(), flags.BootstrapFlag, true, flags.BootstrapFlagDescription).Return(nil)
}

func (s *workerSuite) ensureBootstrapParams(c *gc.C) {
	cfg, err := config.New(config.NoDefaults, testing.FakeConfig())
	c.Assert(err, jc.ErrorIsNil)

	args := instancecfg.StateInitializationParams{
		ControllerModelConfig:       cfg,
		BootstrapMachineConstraints: constraints.MustParse("mem=1G"),
		ControllerCharmPath:         "obscura",
		ControllerCharmChannel:      charm.MakePermissiveChannel("", "stable", ""),
	}
	bytes, err := args.Marshal()
	c.Assert(err, jc.ErrorIsNil)

	err = os.WriteFile(filepath.Join(s.dataDir, cloudconfig.FileNameBootstrapParams), bytes, 0644)
	c.Assert(err, jc.ErrorIsNil)
}
