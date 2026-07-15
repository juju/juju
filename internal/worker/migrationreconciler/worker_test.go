// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationreconciler

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/clock/testclock"
	"github.com/juju/tc"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/workertest"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	coretesting "github.com/juju/juju/core/testing"
	"github.com/juju/juju/domain/modelmigration"
	modelmigrationerrors "github.com/juju/juju/domain/modelmigration/errors"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
)

func TestConfigSuite(t *testing.T) { tc.Run(t, &configSuite{}) }
func TestWorkerSuite(t *testing.T) {
	tc.Run(t, &workerSuite{})
}
func TestManifoldSuite(t *testing.T) { tc.Run(t, &manifoldSuite{}) }
func TestLogicSuite(t *testing.T)    { tc.Run(t, &logicSuite{}) }

type configSuite struct{}

func (s *configSuite) TestValidate(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	base := Config{
		Service: NewMockService(ctrl),
		Abort:   func(context.Context, coremodel.UUID) error { return nil },
		Clock:   testclock.NewClock(time.Now()),
		Logger:  loggertesting.WrapCheckLog(c),
	}
	c.Check(base.Validate(), tc.ErrorIsNil)

	for name, mut := range map[string]func(*Config){
		"Service": func(cfg *Config) { cfg.Service = nil },
		"Abort":   func(cfg *Config) { cfg.Abort = nil },
		"Clock":   func(cfg *Config) { cfg.Clock = nil },
		"Logger":  func(cfg *Config) { cfg.Logger = nil },
	} {
		cfg := base
		mut(&cfg)
		c.Check(cfg.Validate(), tc.ErrorIs, coreerrors.NotValid, tc.Commentf("nil %s", name))
	}
}

type workerSuite struct {
	service *MockService
	clock   *testclock.Clock

	aborted  chan coremodel.UUID
	abortErr error
}

func (s *workerSuite) setup(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.service = NewMockService(ctrl)
	s.clock = testclock.NewClock(time.Now())
	s.aborted = make(chan coremodel.UUID, 16)
	s.abortErr = nil
	return ctrl
}

func (s *workerSuite) newWorker(c *tc.C) worker.Worker {
	w, err := NewWorker(Config{
		Service: s.service,
		Abort: func(_ context.Context, modelUUID coremodel.UUID) error {
			s.aborted <- modelUUID
			return s.abortErr
		},
		Clock:  s.clock,
		Logger: loggertesting.WrapCheckLog(c),
	})
	c.Assert(err, tc.ErrorIsNil)
	return w
}

// tick advances the clock past one jittered reconcile interval, waiting for the
// worker's timer to be registered first.
func (s *workerSuite) tick(c *tc.C) {
	err := s.clock.WaitAdvance(defaultReconcileInterval*3/2, coretesting.LongWait, 1)
	c.Assert(err, tc.ErrorIsNil)
}

func abortingClaim(modelUUID coremodel.UUID, updatedAt time.Time) modelmigration.ImportClaimStatus {
	return modelmigration.ImportClaimStatus{
		ModelUUID:           modelUUID.String(),
		SourceMigrationUUID: "source-migration",
		Phase:               modelmigration.ImportPhaseAborting,
		UpdatedAt:           updatedAt,
	}
}

// TestFinalizesAbortingClaim verifies the reconciler spawns a per-model job
// worker that re-drives the abort (staging the model database for deletion)
// then finalizes the claim.
func (s *workerSuite) TestFinalizesAbortingClaim(c *tc.C) {
	defer s.setup(c).Finish()

	modelUUID := coremodel.UUID(uuid.MustNewUUID().String())
	done := make(chan struct{})

	s.service.EXPECT().GetAllImportClaims(gomock.Any()).
		Return([]modelmigration.ImportClaimStatus{abortingClaim(modelUUID, s.clock.Now())}, nil)
	s.service.EXPECT().FinalizeAbortedImport(gomock.Any(), modelUUID).DoAndReturn(
		func(context.Context, coremodel.UUID) error { close(done); return nil })
	// Any later scan (e.g. a timer re-fire before kill) sees nothing to do.
	s.service.EXPECT().GetAllImportClaims(gomock.Any()).Return(nil, nil).AnyTimes()

	w := s.newWorker(c)
	defer workertest.CleanKill(c, w)

	s.tick(c)

	select {
	case <-done:
	case <-time.After(coretesting.LongWait):
		c.Fatal("aborting claim was not finalized")
	}
	select {
	case got := <-s.aborted:
		c.Check(got, tc.Equals, modelUUID)
	default:
		c.Fatal("abort compensation was not re-driven")
	}
}

// TestIgnoresFreshNonAbortingClaims verifies importing/activating claims that
// are not stale are left entirely untouched.
func (s *workerSuite) TestIgnoresFreshNonAbortingClaims(c *tc.C) {
	defer s.setup(c).Finish()

	scanned := make(chan struct{}, 1)
	now := s.clock.Now()
	s.service.EXPECT().GetAllImportClaims(gomock.Any()).DoAndReturn(
		func(context.Context) ([]modelmigration.ImportClaimStatus, error) {
			select {
			case scanned <- struct{}{}:
			default:
			}
			return []modelmigration.ImportClaimStatus{
				{ModelUUID: uuid.MustNewUUID().String(), Phase: modelmigration.ImportPhaseImporting, UpdatedAt: now},
				{ModelUUID: uuid.MustNewUUID().String(), Phase: modelmigration.ImportPhaseActivating, UpdatedAt: now},
			}, nil
		}).AnyTimes()

	// No abort / finalize expectations: any such call fails the test.

	w := s.newWorker(c)
	defer workertest.CleanKill(c, w)

	s.tick(c)

	select {
	case <-scanned:
	case <-time.After(coretesting.LongWait):
		c.Fatal("reconcile did not run")
	}
	select {
	case <-s.aborted:
		c.Fatal("abort must not run for non-aborting claims")
	case <-time.After(coretesting.ShortWait):
	}
}

// TestFinalizeFailureKeepsWorkerAlive verifies a finalize error is handled
// gracefully: the abort is attempted and the worker survives to retry later.
func (s *workerSuite) TestFinalizeFailureKeepsWorkerAlive(c *tc.C) {
	defer s.setup(c).Finish()

	modelUUID := coremodel.UUID(uuid.MustNewUUID().String())

	s.service.EXPECT().GetAllImportClaims(gomock.Any()).
		Return([]modelmigration.ImportClaimStatus{abortingClaim(modelUUID, s.clock.Now())}, nil).AnyTimes()
	s.service.EXPECT().FinalizeAbortedImport(gomock.Any(), modelUUID).Return(
		errors.Errorf("cleanup incomplete: %w", modelmigrationerrors.ErrAbortNotFinalizable),
	).AnyTimes()

	w := s.newWorker(c)
	defer workertest.CleanKill(c, w)

	s.tick(c)

	// The worker must still be running after a finalize failure.
	workertest.CheckAlive(c, w)
}

// logicSuite white-box tests the reconciler's stale-claim warning logic
// directly, without the timer loop, for deterministic assertions.
type logicSuite struct{}

// recordingLogger returns a logger that appends every formatted message to
// *entries. Used to assert warnings synchronously.
func recordingLogger() (logger.Logger, *[]string) {
	var entries []string
	rec := loggertesting.RecordLog(func(s string, a ...any) {
		entries = append(entries, s)
	})
	return loggertesting.WrapCheckLog(rec), &entries
}

func countContaining(entries []string, sub string) int {
	n := 0
	for _, e := range entries {
		if strings.Contains(e, sub) {
			n++
		}
	}
	return n
}

func bareReconciler(cfg Config) *reconciler {
	return &reconciler{config: cfg, staleWarnings: make(map[string]time.Time)}
}

// TestWarnIfStale verifies a claim past the stale threshold warns once, is
// rate-limited within the warn interval, and warns again afterwards.
func (s *logicSuite) TestWarnIfStale(c *tc.C) {
	log, entries := recordingLogger()
	w := bareReconciler(Config{Logger: log})
	now := time.Now()
	claim := modelmigration.ImportClaimStatus{
		ModelUUID: uuid.MustNewUUID().String(),
		Phase:     modelmigration.ImportPhaseImporting,
		UpdatedAt: now.Add(-staleClaimThreshold - time.Hour),
	}

	w.warnIfStale(c.Context(), claim, now)
	c.Check(countContaining(*entries, "has been in the"), tc.Equals, 1)

	// Within the warn interval: rate-limited, no new warning.
	w.warnIfStale(c.Context(), claim, now.Add(time.Minute))
	c.Check(countContaining(*entries, "has been in the"), tc.Equals, 1)

	// Past the warn interval: warns again.
	w.warnIfStale(c.Context(), claim, now.Add(staleWarnInterval+time.Minute))
	c.Check(countContaining(*entries, "has been in the"), tc.Equals, 2)
}

// TestWarnIfStaleFreshClaimSilent verifies a claim younger than the stale
// threshold is never warned about.
func (s *logicSuite) TestWarnIfStaleFreshClaimSilent(c *tc.C) {
	log, entries := recordingLogger()
	w := bareReconciler(Config{Logger: log})
	now := time.Now()
	claim := modelmigration.ImportClaimStatus{
		ModelUUID: uuid.MustNewUUID().String(),
		Phase:     modelmigration.ImportPhaseImporting,
		UpdatedAt: now.Add(-time.Hour), // well under the 24h threshold
	}

	w.warnIfStale(c.Context(), claim, now)
	c.Check(*entries, tc.HasLen, 0)
	_, tracked := w.staleWarnings[claim.ModelUUID]
	c.Check(tracked, tc.IsFalse)
}
