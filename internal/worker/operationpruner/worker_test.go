// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operationpruner

import (
	"context"
	"testing"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"

	coretesting "github.com/juju/juju/core/testing"
	corewatcher "github.com/juju/juju/core/watcher"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
)

func TestConfigSuite(t *testing.T) { tc.Run(t, &configSuite{}) }
func TestWorkerSuite(t *testing.T) { tc.Run(t, &workerSuite{}) }

type configSuite struct{}

// TestConfigValidation tests that the config is validated correctly.
func (s *configSuite) TestConfigValidation(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	// Base valid config
	origCfg := Config{
		Clock:            testclock.NewClock(time.Now()),
		ModelConfig:      NewMockModelConfigService(ctrl),
		OperationService: NewMockOperationService(ctrl),
		Logger:           loggertesting.WrapCheckLog(c),
		PruneInterval:    time.Second,
	}

	c.Check(origCfg.Validate(), tc.ErrorIsNil)

	testCfg := origCfg
	testCfg.Clock = nil
	c.Check(testCfg.Validate(), tc.ErrorMatches, "nil clock.Clock.*")

	testCfg = origCfg
	testCfg.ModelConfig = nil
	c.Check(testCfg.Validate(), tc.ErrorMatches, "nil ModelConfig.*")

	testCfg = origCfg
	testCfg.OperationService = nil
	c.Check(testCfg.Validate(), tc.ErrorMatches, "nil OperationService.*")

	testCfg = origCfg
	testCfg.Logger = nil
	c.Check(testCfg.Validate(), tc.ErrorMatches, "nil Logger.*")

	testCfg = origCfg
	testCfg.PruneInterval = 0
	c.Check(testCfg.Validate(), tc.ErrorMatches, "prune interval must be positive.*")
}

type workerSuite struct{}

// TestPrunesAfterBothConfigValues tests that the worker prunes operations
// when both the max action results age and size config values are set.
func (s *workerSuite) TestPrunesAfterBothConfigValues(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	w, mocked := s.startWorker(c, ctrl)
	defer workertest.CleanKill(c, w)

	mocked.expectModelConfig(c, "1h", "20M").Times(1)

	// Expect one prune call after timer fires with the values above.
	wait := mocked.expectPruneOperation(c, time.Hour, 20)
	defer wait()

	// Emit changes
	mocked.pushConfigChanges(c, config.MaxActionResultsAge, config.MaxActionResultsSize)

	// Advance enough to trigger the prune
	mocked.advancePruneInterval(c)
}

func (w *workerMocks) expectPruneOperation(c *tc.C, duration time.Duration, sizeMB int) (waitForMe func()) {
	waitForIt := make(chan struct{})
	w.operationService.EXPECT().PruneOperations(gomock.Any(), duration, sizeMB).DoAndReturn(
		func(ctx context.Context, duration time.Duration, sizeMB int) error {
			close(waitForIt)
			return nil
		}).Times(1)
	return func() {
		select {
		case <-waitForIt:
		case <-time.After(coretesting.ShortWait):
			c.Fatalf("Prune operation should have been called")
		}
	}
}

// TestPrunesAfterBothConfigValuesSequentially tests that the worker prunes operations
// when both the max action results age and size config values are set.
func (s *workerSuite) TestPrunesAfterBothConfigValuesSequentially(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	w, mocked := s.startWorker(c, ctrl)
	defer workertest.CleanKill(c, w)

	mocked.expectModelConfig(c, "1h", "20M").Times(2) // called twice

	// Expect one prune call after timer fires with the values above.
	wait := mocked.expectPruneOperation(c, time.Hour, 20)
	defer wait()

	// Emit changes one by one
	mocked.pushConfigChanges(c, config.MaxActionResultsAge)
	mocked.pushConfigChanges(c, config.MaxActionResultsSize)

	// Advance enough to trigger the prune
	mocked.advancePruneInterval(c)
}

// TestModelConfigErrorOnGetModelConfig tests that the worker does not prune
// operations when the model config fails to be retrieved.
func (s *workerSuite) TestModelConfigErrorOnGetModelConfig(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	expectedError := errors.New("bang")

	w, mocked := s.startWorker(c, ctrl)
	defer func() {
		err := workertest.CheckKill(c, w)
		c.Assert(err, tc.ErrorIs, expectedError)
	}()

	// Failure while getting model config.
	mocked.modelConfigService.EXPECT().ModelConfig(gomock.Any()).Return(nil, expectedError)

	// Expect no call because getting model config failed.
	mocked.operationService.EXPECT().PruneOperations(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(0)

	// Emit a change to trigger the failure
	mocked.pushConfigChanges(c, config.MaxActionResultsAge)
}

// TestPruneError verifies that the worker correctly handles errors returned
// during the prune operation.
func (s *workerSuite) TestPruneError(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	expectedError := errors.New("bang")

	w, mocked := s.startWorker(c, ctrl)
	defer func() {
		err := workertest.CheckKill(c, w)
		c.Assert(err, tc.ErrorIs, expectedError)
	}()

	mocked.expectModelConfig(c, "1h", "10M").AnyTimes()

	// Expect one prune call after timer fires with the values above.
	mocked.operationService.EXPECT().PruneOperations(gomock.Any(), gomock.Any(), gomock.Any()).Return(expectedError)

	// Emit model changes
	mocked.pushConfigChanges(c, config.MaxActionResultsAge, config.MaxActionResultsSize)

	// Advance time to trigger the prune which will fail.
	mocked.advancePruneInterval(c)
	mocked.shouldDie(c)
}

type workerMocks struct {
	clock              *testclock.Clock
	modelConfigService *MockModelConfigService
	operationService   *MockOperationService
	pruneInterval      time.Duration
	worker             *prunerWorker
	modelConfigChanges chan []string
}

// helper to build a minimal *config.Config with our keys
func buildModelConfig(c *tc.C, age, size string) *config.Config {
	attrs := map[string]any{
		"name":                      "test-model",
		"type":                      "test-type",
		"uuid":                      uuid.MustNewUUID().String(),
		config.MaxActionResultsAge:  age,
		config.MaxActionResultsSize: size,
	}
	cfg, err := config.New(config.UseDefaults, attrs)
	c.Assert(err, tc.ErrorIsNil)
	return cfg
}

// startWorker starts a worker and returns it and the mocks it uses.
func (s *workerSuite) startWorker(c *tc.C, ctrl *gomock.Controller) (worker.Worker, workerMocks) {
	mocked := workerMocks{
		clock:              testclock.NewClock(time.Now()),
		modelConfigService: NewMockModelConfigService(ctrl),
		operationService:   NewMockOperationService(ctrl),
		pruneInterval:      time.Second,
		modelConfigChanges: make(chan []string),
	}
	c.Cleanup(func() {
		close(mocked.modelConfigChanges)
	})

	workerMainLoopEnteredCh := make(chan struct{}, 1)
	watcher := NewMockStringsWatcher(ctrl)
	mocked.modelConfigService.EXPECT().Watch(gomock.Any()).Return(watcher, nil)
	mocked.expectModelConfig(c, "42h", "42M").Times(1) // Upfront call to get initial config.
	watcher.EXPECT().Changes().DoAndReturn(func() corewatcher.StringsChannel {
		select {
		case workerMainLoopEnteredCh <- struct{}{}:
		default:
		}
		return mocked.modelConfigChanges
	}).AnyTimes()
	watcher.EXPECT().Kill().AnyTimes()
	watcher.EXPECT().Wait().AnyTimes()

	w, err := NewWorker(Config{
		Clock:            mocked.clock,
		ModelConfig:      mocked.modelConfigService,
		OperationService: mocked.operationService,
		Logger:           loggertesting.WrapCheckLog(c),
		PruneInterval:    mocked.pruneInterval,
	})
	c.Assert(err, tc.ErrorIsNil)

	// Wait for worker to reach main loop before we allow tests to
	// manipulate the clock.
	select {
	case <-workerMainLoopEnteredCh:
	case <-time.After(coretesting.ShortWait):
		c.Fatal("timed out waiting for worker to enter main loop")
	}

	mocked.worker = w.(*prunerWorker)
	return w, mocked
}

// expectModelConfig expects a call to ModelConfig with the given age and size.
func (w *workerMocks) expectModelConfig(c *tc.C, age string, size string) *gomock.Call {
	return w.modelConfigService.EXPECT().ModelConfig(gomock.Any()).DoAndReturn(func(ctx context.Context) (
		*config.Config, error) {
		return buildModelConfig(c, age, size), nil
	}).Call
}

// advancePruneInterval advances the clock at least by the prune interval
func (w *workerMocks) advancePruneInterval(c *tc.C) {
	w.clock.Advance(w.pruneInterval * 3 / 2) // jitter can be up to 1/2 prune interval
}

// pushConfigChanges emits the given changes to the model config watcher and
// asserts a loop completes (and so the changes are applied).
func (w *workerMocks) pushConfigChanges(c *tc.C, changes ...string) {
	w.modelConfigChanges <- changes
}

// shouldDie verifies if the worker has successfully terminated within a short
// timeout, failing the test if it hasn't.
func (w *workerMocks) shouldDie(c *tc.C) {
	select {
	case <-w.worker.catacomb.Dead():
	case <-time.After(coretesting.ShortWait):
		c.Fatalf("Undead worker")
	}
}
