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
	"github.com/juju/juju/core/watcher/watchertest"
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
		Service:  NewMockService(ctrl),
		Abort:    func(context.Context, coremodel.UUID) error { return nil },
		Activate: func(context.Context, coremodel.UUID) error { return nil },
		Clock:    testclock.NewClock(time.Now()),
		Logger:   loggertesting.WrapCheckLog(c),
	}
	c.Check(base.Validate(), tc.ErrorIsNil)

	for name, mut := range map[string]func(*Config){
		"Service":  func(cfg *Config) { cfg.Service = nil },
		"Abort":    func(cfg *Config) { cfg.Abort = nil },
		"Activate": func(cfg *Config) { cfg.Activate = nil },
		"Clock":    func(cfg *Config) { cfg.Clock = nil },
		"Logger":   func(cfg *Config) { cfg.Logger = nil },
	} {
		cfg := base
		mut(&cfg)
		c.Check(cfg.Validate(), tc.ErrorIs, coreerrors.NotValid, tc.Commentf("nil %s", name))
	}
}

type workerSuite struct {
	service *MockService
	clock   *testclock.Clock

	aborted     chan coremodel.UUID
	abortErr    error
	activatedCh chan coremodel.UUID
	activateErr error
	changes     chan []string
}

func (s *workerSuite) setup(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.service = NewMockService(ctrl)
	s.clock = testclock.NewClock(time.Now())
	s.aborted = make(chan coremodel.UUID, 16)
	s.abortErr = nil
	s.activatedCh = make(chan coremodel.UUID, 16)
	s.activateErr = nil
	s.changes = make(chan []string, 16)
	s.service.EXPECT().WatchImportClaims(gomock.Any()).Return(
		watchertest.NewMockStringsWatcher(s.changes), nil,
	)
	return ctrl
}

func (s *workerSuite) newWorker(c *tc.C) worker.Worker {
	w, err := NewWorker(Config{
		Service: s.service,
		Abort: func(_ context.Context, modelUUID coremodel.UUID) error {
			s.aborted <- modelUUID
			return s.abortErr
		},
		Activate: func(_ context.Context, modelUUID coremodel.UUID) error {
			s.activatedCh <- modelUUID
			return s.activateErr
		},
		Clock:  s.clock,
		Logger: loggertesting.WrapCheckLog(c),
	})
	c.Assert(err, tc.ErrorIsNil)
	return w
}

func abortingClaim(modelUUID coremodel.UUID, updatedAt time.Time) modelmigration.ImportClaimStatus {
	return modelmigration.ImportClaimStatus{
		ModelUUID:           modelUUID.String(),
		SourceMigrationUUID: "source-migration",
		Phase:               modelmigration.ImportPhaseAborting,
		UpdatedAt:           updatedAt,
	}
}

func activatingClaim(modelUUID coremodel.UUID, updatedAt time.Time) modelmigration.ImportClaimStatus {
	return modelmigration.ImportClaimStatus{
		ModelUUID:           modelUUID.String(),
		SourceMigrationUUID: "source-migration",
		Phase:               modelmigration.ImportPhaseActivating,
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

	s.changes <- []string{modelUUID.String()}

	select {
	case <-done:
	case <-c.Context().Done():
		c.Fatal("aborting claim was not finalized")
	}
	select {
	case got := <-s.aborted:
		c.Check(got, tc.Equals, modelUUID)
	default:
		c.Fatal("abort compensation was not re-driven")
	}
}

// TestIgnoresFreshImportingClaims verifies importing claims that are not stale
// are left entirely untouched (no abort, no activation, no finalize).
func (s *workerSuite) TestIgnoresFreshImportingClaims(c *tc.C) {
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
			}, nil
		}).AnyTimes()

	// No abort / activate / finalize expectations: any such call fails the test.

	w := s.newWorker(c)
	defer workertest.CleanKill(c, w)

	s.changes <- []string{"changed-model"}

	select {
	case <-scanned:
	case <-c.Context().Done():
		c.Fatal("reconcile did not run")
	}
	select {
	case <-s.aborted:
		c.Fatal("abort must not run for importing claims")
	case <-s.activatedCh:
		c.Fatal("activation must not run for importing claims")
	default:
	}
}

// TestCompletesActivatingClaim verifies the reconciler drives an activating
// claim to completion by re-driving the idempotent activation finalization -
// the self-healing backstop for an activation the source worker did not finish.
func (s *workerSuite) TestCompletesActivatingClaim(c *tc.C) {
	defer s.setup(c).Finish()

	modelUUID := coremodel.UUID(uuid.MustNewUUID().String())

	s.service.EXPECT().GetAllImportClaims(gomock.Any()).
		Return([]modelmigration.ImportClaimStatus{activatingClaim(modelUUID, s.clock.Now())}, nil)
	// Any later scan (e.g. a timer re-fire before kill) sees nothing to do.
	s.service.EXPECT().GetAllImportClaims(gomock.Any()).Return(nil, nil).AnyTimes()

	w := s.newWorker(c)
	defer workertest.CleanKill(c, w)

	s.changes <- []string{modelUUID.String()}

	select {
	case got := <-s.activatedCh:
		c.Check(got, tc.Equals, modelUUID)
	case <-c.Context().Done():
		c.Fatal("activating claim was not completed")
	}
	// Abort must never run for an activating claim.
	select {
	case <-s.aborted:
		c.Fatal("abort must not run for an activating claim")
	default:
	}
}

// TestActivateFailureKeepsWorkerAlive verifies an activation-completion failure
// is handled gracefully: the worker survives to retry later (finalization is
// convergent).
func (s *workerSuite) TestActivateFailureKeepsWorkerAlive(c *tc.C) {
	defer s.setup(c).Finish()

	s.activateErr = errors.New("clearing import gate: boom")
	modelUUID := coremodel.UUID(uuid.MustNewUUID().String())

	s.service.EXPECT().GetAllImportClaims(gomock.Any()).
		Return([]modelmigration.ImportClaimStatus{activatingClaim(modelUUID, s.clock.Now())}, nil).AnyTimes()

	w := s.newWorker(c)
	defer workertest.CleanKill(c, w)

	s.changes <- []string{modelUUID.String()}

	select {
	case <-s.activatedCh:
	case <-c.Context().Done():
		c.Fatal("activation completion was not attempted")
	}
	// The worker must still be running after an activation-completion failure.
	workertest.CheckAlive(c, w)
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

	s.changes <- []string{modelUUID.String()}

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

// TestReconcileReturnsListError verifies a failed claim listing is reported to
// the caller rather than swallowed, so the loop decides how to handle it.
func (s *logicSuite) TestReconcileReturnsListError(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	boom := errors.New("boom")
	service := NewMockService(ctrl)
	service.EXPECT().GetAllImportClaims(gomock.Any()).Return(nil, boom)

	w := bareReconciler(Config{
		Service: service,
		Clock:   testclock.NewClock(time.Now()),
		Logger:  loggertesting.WrapCheckLog(c),
	})

	err := w.reconcile(c.Context())
	c.Assert(err, tc.ErrorIs, boom)
	c.Check(err, tc.ErrorMatches, ".*listing migration import claims.*")
}

// TestReconcilePrunesStaleWarnings verifies a successful scan forgets warning
// state for claims that no longer exist, so the map cannot grow unbounded.
func (s *logicSuite) TestReconcilePrunesStaleWarnings(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	gone := uuid.MustNewUUID().String()
	service := NewMockService(ctrl)
	service.EXPECT().GetAllImportClaims(gomock.Any()).Return(nil, nil)

	w := bareReconciler(Config{
		Service: service,
		Clock:   testclock.NewClock(time.Now()),
		Logger:  loggertesting.WrapCheckLog(c),
	})
	w.staleWarnings[gone] = time.Now()

	c.Assert(w.reconcile(c.Context()), tc.ErrorIsNil)
	c.Check(w.staleWarnings, tc.HasLen, 0)
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
