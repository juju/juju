// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backupcreation

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/juju/clock"
	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v5/workertest"

	corebackups "github.com/juju/juju/core/backups"
	"github.com/juju/juju/core/lease"
	leaseservice "github.com/juju/juju/domain/lease/service"
	leasestate "github.com/juju/juju/domain/lease/state"
	schematesting "github.com/juju/juju/domain/schema/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
)

type workerSuite struct{}

func TestWorkerSuite(t *testing.T) { tc.Run(t, &workerSuite{}) }

func workerConfig(c *tc.C) Config {
	return Config{
		LeaseService:        leaseStub{},
		ControllerUUID:      uuid.MustNewUUID().String(),
		ControllerModelUUID: uuid.MustNewUUID().String(),
		CreateArchive: func(context.Context, string) (*corebackups.Metadata, string, func(), error) {
			return &corebackups.Metadata{}, "archive", func() {}, nil
		},
		Clock:  clock.WallClock,
		Logger: loggertesting.WrapCheckLog(c),
	}
}

func startWorker(c *tc.C, cfg Config) *Worker {
	w, err := NewWorker(cfg)
	c.Assert(err, tc.ErrorIsNil)
	c.Cleanup(func() { w.Kill(); c.Check(w.Wait(), tc.ErrorIsNil) })
	return w.(*Worker)
}

func createAsync(ctx context.Context, w *Worker) <-chan result {
	ch := make(chan result, 1)
	go func() {
		meta, archive, cleanup, err := w.Create(ctx, "notes")
		ch <- result{meta: meta, archive: archive, cleanup: cleanup, err: err}
	}()
	return ch
}

func (s *workerSuite) TestValidate(c *tc.C) {
	tests := []struct {
		change  func(*Config)
		message string
	}{
		{func(c *Config) { c.LeaseService = nil }, "nil LeaseService not valid"},
		{func(c *Config) { c.CreateArchive = nil }, "nil CreateArchive not valid"},
		{func(c *Config) { c.Clock = nil }, "nil Clock not valid"},
		{func(c *Config) { c.Logger = nil }, "nil Logger not valid"},
		{func(c *Config) { c.ControllerUUID = "bad" }, "backup controller UUID: .*"},
		{func(c *Config) { c.ControllerModelUUID = "bad" }, "backup controller model UUID: .*"},
	}
	for _, test := range tests {
		cfg := workerConfig(c)
		test.change(&cfg)
		_, err := NewWorker(cfg)
		c.Check(err, tc.ErrorMatches, test.message)
	}
}

func (s *workerSuite) TestSuccessReleasesBeforeReturningArchive(c *tc.C) {
	cfg := workerConfig(c)
	var claimed lease.Key
	var holder string
	var released, cleaned atomic.Bool
	cfg.LeaseService = leaseStub{
		claim: func(ctx context.Context, key lease.Key, req lease.Request) error {
			claimed, holder = key, req.Holder
			c.Check(req.Duration, tc.Equals, leaseDuration)
			return nil
		},
		revoke: func(ctx context.Context, key lease.Key, owner string) error {
			c.Check(key, tc.DeepEquals, claimed)
			c.Check(owner, tc.Equals, holder)
			c.Check(ctx.Err(), tc.ErrorIsNil)
			released.Store(true)
			return nil
		},
	}
	cfg.CreateArchive = func(ctx context.Context, notes string) (*corebackups.Metadata, string, func(), error) {
		c.Check(ctx.Err(), tc.ErrorIsNil)
		c.Check(notes, tc.Equals, "notes")
		c.Check(holder, tc.Not(tc.Equals), "")
		return &corebackups.Metadata{Notes: notes}, "archive", func() { cleaned.Store(true) }, nil
	}
	w := startWorker(c, cfg)
	meta, path, cleanup, err := w.Create(c.Context(), "notes")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(meta.Notes, tc.Equals, "notes")
	c.Check(path, tc.Equals, "archive")
	c.Check(claimed, tc.DeepEquals, lease.Key{Namespace: lease.BackupCreationNamespace, ModelUUID: cfg.ControllerModelUUID, Lease: cfg.ControllerUUID})
	c.Check(released.Load(), tc.IsTrue)
	c.Check(cleaned.Load(), tc.IsFalse)
	// A second creation can run while the first archive is still being consumed.
	_, _, cleanup2, err := w.Create(c.Context(), "notes")
	c.Assert(err, tc.ErrorIsNil)
	cleanup2()
	cleanup()
	c.Check(cleaned.Load(), tc.IsTrue)
}

func (s *workerSuite) TestAcquisitionErrorsDoNotGenerate(c *tc.C) {
	for _, failure := range []error{lease.ErrHeld, errors.New("database failed")} {
		cfg := workerConfig(c)
		var generated, revoked atomic.Bool
		cfg.LeaseService = leaseStub{
			claim:  func(context.Context, lease.Key, lease.Request) error { return failure },
			revoke: func(context.Context, lease.Key, string) error { revoked.Store(true); return nil },
		}
		cfg.CreateArchive = func(context.Context, string) (*corebackups.Metadata, string, func(), error) {
			generated.Store(true)
			return nil, "", nil, nil
		}
		w := startWorker(c, cfg)
		_, _, _, err := w.Create(c.Context(), "")
		if failure == lease.ErrHeld {
			c.Check(err, tc.ErrorIs, errors.NotYetAvailable)
			c.Check(revoked.Load(), tc.IsFalse)
		} else {
			c.Check(err, tc.ErrorIs, failure)
			c.Check(revoked.Load(), tc.IsTrue)
		}
		c.Check(generated.Load(), tc.IsFalse)
		workertest.CheckAlive(c, w)
	}
}

func (s *workerSuite) TestCancelledBeforeSubmission(c *tc.C) {
	cfg := workerConfig(c)
	cfg.LeaseService = leaseStub{claim: func(context.Context, lease.Key, lease.Request) error {
		c.Error("unexpected claim")
		return nil
	}}
	w := startWorker(c, cfg)
	ctx, cancel := context.WithCancel(c.Context())
	cancel()
	_, _, _, err := w.Create(ctx, "")
	c.Check(err, tc.ErrorIs, context.Canceled)
}

func (s *workerSuite) TestCancellationDuringAcquisition(c *tc.C) {
	s.cancelAcquisition(c, false)
}

func (s *workerSuite) TestShutdownDuringAcquisition(c *tc.C) {
	s.cancelAcquisition(c, true)
}

func (s *workerSuite) cancelAcquisition(c *tc.C, shutdown bool) {
	cfg := workerConfig(c)
	claiming := make(chan struct{})
	var revoked atomic.Bool
	cfg.LeaseService = leaseStub{
		claim: func(ctx context.Context, _ lease.Key, _ lease.Request) error {
			close(claiming)
			<-ctx.Done()
			return ctx.Err()
		},
		revoke: func(ctx context.Context, _ lease.Key, _ string) error {
			c.Check(ctx.Err(), tc.ErrorIsNil)
			_, bounded := ctx.Deadline()
			c.Check(bounded, tc.IsTrue)
			revoked.Store(true)
			return nil
		},
	}
	cfg.CreateArchive = func(context.Context, string) (*corebackups.Metadata, string, func(), error) {
		c.Error("generation started after cancelled acquisition")
		return nil, "", nil, nil
	}
	w := startWorker(c, cfg)
	ctx, cancel := context.WithCancel(c.Context())
	defer cancel()
	done := createAsync(ctx, w)
	<-claiming
	if shutdown {
		w.Kill()
	} else {
		cancel()
	}
	res := <-done
	c.Check(res.err, tc.ErrorIs, context.Canceled)
	c.Check(revoked.Load(), tc.IsTrue)
	if shutdown {
		c.Assert(w.Wait(), tc.ErrorIsNil)
	} else {
		workertest.CheckAlive(c, w)
	}
}

func (s *workerSuite) TestBusyWhileAcquiring(c *tc.C) {
	cfg := workerConfig(c)
	entered := make(chan struct{})
	cfg.LeaseService = leaseStub{claim: func(ctx context.Context, _ lease.Key, _ lease.Request) error {
		close(entered)
		<-ctx.Done()
		return ctx.Err()
	}}
	w := startWorker(c, cfg)
	ctx, cancel := context.WithCancel(c.Context())
	defer cancel()
	done := createAsync(ctx, w)
	<-entered
	_, _, _, err := w.Create(c.Context(), "")
	c.Check(err, tc.ErrorIs, errors.NotYetAvailable)
	cancel()
	c.Check((<-done).err, tc.ErrorIs, context.Canceled)
}

func (s *workerSuite) TestGenerationFailureReleasesLease(c *tc.C) {
	cfg := workerConfig(c)
	failure := errors.New("export failed")
	var revoked atomic.Bool
	cfg.LeaseService = leaseStub{revoke: func(context.Context, lease.Key, string) error { revoked.Store(true); return nil }}
	cfg.CreateArchive = func(context.Context, string) (*corebackups.Metadata, string, func(), error) {
		return nil, "", nil, failure
	}
	w := startWorker(c, cfg)
	_, _, _, err := w.Create(c.Context(), "")
	c.Check(err, tc.ErrorIs, failure)
	c.Check(revoked.Load(), tc.IsTrue)
	workertest.CheckAlive(c, w)
}

func (s *workerSuite) TestCancellationCleansCompletedArchive(c *tc.C) {
	cfg := workerConfig(c)
	ctx, cancel := context.WithCancel(c.Context())
	defer cancel()
	var cleaned atomic.Bool
	cfg.CreateArchive = func(context.Context, string) (*corebackups.Metadata, string, func(), error) {
		cancel()
		return &corebackups.Metadata{}, "archive", func() { cleaned.Store(true) }, nil
	}
	w := startWorker(c, cfg)
	meta, path, cleanup, err := w.Create(ctx, "")
	c.Check(err, tc.ErrorIs, context.Canceled)
	c.Check(meta, tc.IsNil)
	c.Check(path, tc.Equals, "")
	c.Check(cleanup, tc.IsNil)
	c.Check(cleaned.Load(), tc.IsTrue)
}

func (s *workerSuite) TestShutdownWaitsForGenerationBeforeRevoking(c *tc.C) {
	cfg := workerConfig(c)
	entered, cancelled, finish := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var stopped, revoked atomic.Bool
	cfg.CreateArchive = func(ctx context.Context, _ string) (*corebackups.Metadata, string, func(), error) {
		close(entered)
		<-ctx.Done()
		close(cancelled)
		<-finish
		stopped.Store(true)
		return nil, "", nil, ctx.Err()
	}
	cfg.LeaseService = leaseStub{revoke: func(context.Context, lease.Key, string) error {
		c.Check(stopped.Load(), tc.IsTrue)
		revoked.Store(true)
		return nil
	}}
	w := startWorker(c, cfg)
	done := createAsync(c.Context(), w)
	<-entered
	w.Kill()
	<-cancelled
	c.Check(revoked.Load(), tc.IsFalse)
	close(finish)
	c.Check((<-done).err, tc.ErrorIs, context.Canceled)
	c.Assert(w.Wait(), tc.ErrorIsNil)
	c.Check(revoked.Load(), tc.IsTrue)
}

func (s *workerSuite) TestRenewsDuringGeneration(c *tc.C)          { s.renewal(c, "success") }
func (s *workerSuite) TestRenewalFailureCancelsGeneration(c *tc.C) { s.renewal(c, "failure") }
func (s *workerSuite) TestExpiryCancelsStalledRenewal(c *tc.C)     { s.renewal(c, "expiry") }
func (s *workerSuite) TestGenerationCompletionDoesNotHideRenewalFailure(c *tc.C) {
	s.renewal(c, "late-failure")
}

func (s *workerSuite) TestLateRenewalCannotReviveExpiredGeneration(c *tc.C) {
	s.renewal(c, "late-success")
}

func (s *workerSuite) TestRequestCancellationInterruptsRenewal(c *tc.C) { s.renewal(c, "cancel") }
func (s *workerSuite) TestShutdownInterruptsRenewal(c *tc.C)            { s.renewal(c, "shutdown") }

func (s *workerSuite) renewal(c *tc.C, scenario string) {
	cfg := workerConfig(c)
	clk := &observedClock{Clock: testclock.NewClock(time.Now()), timers: make(chan struct{}, 4)}
	cfg.Clock = clk
	renewed, entered, finish := make(chan struct{}), make(chan struct{}), make(chan struct{})
	failure := errors.New("lease lost")
	var renewalStopped, cleaned atomic.Bool
	cfg.LeaseService = leaseStub{extend: func(ctx context.Context, _ lease.Key, _ lease.Request) error {
		defer renewalStopped.Store(true)
		close(renewed)
		switch scenario {
		case "success":
			return nil
		case "failure":
			return failure
		case "late-failure":
			<-ctx.Done()
			return lease.ErrInvalid
		case "late-success":
			<-ctx.Done()
			return nil
		default:
			<-ctx.Done()
			return ctx.Err()
		}
	}, revoke: func(context.Context, lease.Key, string) error {
		c.Check(renewalStopped.Load(), tc.IsTrue)
		return nil
	}}
	cfg.CreateArchive = func(ctx context.Context, _ string) (*corebackups.Metadata, string, func(), error) {
		close(entered)
		select {
		case <-ctx.Done():
			return nil, "", nil, ctx.Err()
		case <-finish:
			return &corebackups.Metadata{}, "archive", func() { cleaned.Store(true) }, nil
		}
	}
	w := startWorker(c, cfg)
	ctx, cancel := context.WithCancel(c.Context())
	defer cancel()
	done := createAsync(ctx, w)
	<-entered
	<-clk.timers
	clk.Advance(renewInterval)
	<-renewed
	switch scenario {
	case "success", "late-failure":
		close(finish)
	case "expiry", "late-success":
		clk.Advance(leaseDuration - leaseSafetyMargin - renewInterval)
	case "cancel":
		cancel()
	case "shutdown":
		w.Kill()
	}
	res := <-done
	switch scenario {
	case "success":
		c.Assert(res.err, tc.ErrorIsNil)
		res.cleanup()
	case "late-failure":
		c.Check(res.err, tc.ErrorIs, lease.ErrNotHeld)
		c.Check(cleaned.Load(), tc.IsTrue)
	case "failure":
		c.Check(res.err, tc.ErrorIs, failure)
	case "expiry", "late-success":
		c.Check(res.err, tc.ErrorMatches, "backup creation lease expired")
	default:
		c.Check(res.err, tc.ErrorIs, context.Canceled)
	}
}

func (s *workerSuite) TestLateAcquisitionDoesNotStartGeneration(c *tc.C) {
	cfg := workerConfig(c)
	clk := testclock.NewClock(time.Now())
	cfg.Clock = clk
	cfg.LeaseService = leaseStub{claim: func(context.Context, lease.Key, lease.Request) error {
		clk.Advance(leaseDuration)
		return nil
	}}
	cfg.CreateArchive = func(context.Context, string) (*corebackups.Metadata, string, func(), error) {
		c.Error("generation started after lease deadline")
		return nil, "", nil, nil
	}
	w := startWorker(c, cfg)
	_, _, _, err := w.Create(c.Context(), "")
	c.Check(err, tc.ErrorMatches, "backup creation lease expired")
}

func (s *workerSuite) TestReleaseFailureDoesNotDiscardSuccessfulArchive(c *tc.C) {
	cfg := workerConfig(c)
	cfg.LeaseService = leaseStub{revoke: func(context.Context, lease.Key, string) error { return errors.New("release failed") }}
	w := startWorker(c, cfg)
	_, _, cleanup, err := w.Create(c.Context(), "")
	c.Assert(err, tc.ErrorIsNil)
	cleanup()
}

type leaseStub struct {
	claim  func(context.Context, lease.Key, lease.Request) error
	extend func(context.Context, lease.Key, lease.Request) error
	revoke func(context.Context, lease.Key, string) error
}

func (s leaseStub) ClaimLease(ctx context.Context, key lease.Key, req lease.Request) error {
	if s.claim != nil {
		return s.claim(ctx, key, req)
	}
	return nil
}
func (s leaseStub) ExtendLease(ctx context.Context, key lease.Key, req lease.Request) error {
	if s.extend != nil {
		return s.extend(ctx, key, req)
	}
	return nil
}
func (s leaseStub) RevokeLease(ctx context.Context, key lease.Key, holder string) error {
	if s.revoke != nil {
		return s.revoke(ctx, key, holder)
	}
	return nil
}

type observedClock struct {
	*testclock.Clock
	timers chan struct{}
}

func (c *observedClock) NewTimer(d time.Duration) clock.Timer {
	timer := c.Clock.NewTimer(d)
	c.timers <- struct{}{}
	return timer
}

type databaseSuite struct{ schematesting.ControllerSuite }

func TestDatabaseSuite(t *testing.T) { tc.Run(t, &databaseSuite{}) }

func (s *databaseSuite) TestCrossControllerContentionAndRelease(c *tc.C) {
	cfg := workerConfig(c)
	service := leaseservice.NewService(leasestate.NewState(s.TxnRunnerFactory()))
	clk := &observedClock{Clock: testclock.NewClock(time.Now()), timers: make(chan struct{}, 4)}
	cfg.Clock = clk
	renewed := make(chan struct{}, 1)
	cfg.LeaseService = leaseStub{
		claim:  service.ClaimLease,
		revoke: service.RevokeLease,
		extend: func(ctx context.Context, key lease.Key, req lease.Request) error {
			err := service.ExtendLease(ctx, key, req)
			renewed <- struct{}{}
			return err
		},
	}
	started, finish := make(chan struct{}, 2), make(chan struct{})
	cfg.CreateArchive = func(ctx context.Context, _ string) (*corebackups.Metadata, string, func(), error) {
		started <- struct{}{}
		select {
		case <-ctx.Done():
			return nil, "", nil, ctx.Err()
		case <-finish:
			return &corebackups.Metadata{}, "archive", func() {}, nil
		}
	}
	w1 := startWorker(c, cfg)
	cfg.LeaseService = leaseservice.NewService(leasestate.NewState(s.TxnRunnerFactory()))
	w2 := startWorker(c, cfg)
	first := createAsync(c.Context(), w1)
	<-started
	leases, err := service.Leases(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(leases, tc.HasLen, 1)
	key := lease.Key{Namespace: lease.BackupCreationNamespace, ModelUUID: cfg.ControllerModelUUID, Lease: cfg.ControllerUUID}
	oldHolder := leases[key].Holder
	c.Check(oldHolder, tc.Not(tc.Equals), "")
	// Shorten the stored lease to distinguish renewal from the initial claim
	// without sleeping for the database clock to advance.
	err = service.ExtendLease(c.Context(), key, lease.Request{Holder: oldHolder, Duration: 10 * time.Second})
	c.Assert(err, tc.ErrorIsNil)
	<-clk.timers
	clk.Advance(renewInterval)
	<-renewed
	leases, err = service.Leases(c.Context(), key)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(leases[key].Expiry.After(time.Now().Add(30*time.Second)), tc.IsTrue)
	_, _, _, err = w2.Create(c.Context(), "")
	c.Check(err, tc.ErrorIs, errors.NotYetAvailable)
	close(finish)
	res := <-first
	c.Assert(res.err, tc.ErrorIsNil)
	res.cleanup()
	leases, err = service.Leases(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(leases, tc.HasLen, 0)
	// Stale releases cannot remove a later invocation's lease.
	err = service.ClaimLease(c.Context(), key, lease.Request{Holder: "successor", Duration: leaseDuration})
	c.Assert(err, tc.ErrorIsNil)
	err = service.ExtendLease(c.Context(), key, lease.Request{Holder: "successor", Duration: 2 * leaseDuration})
	c.Assert(err, tc.ErrorIsNil)
	err = service.RevokeLease(c.Context(), key, oldHolder)
	c.Check(err, tc.ErrorIs, lease.ErrInvalid)
	leases, err = service.Leases(c.Context(), key)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(leases[key].Holder, tc.Equals, "successor")
	c.Check(leases[key].Expiry.After(time.Now().Add(leaseDuration)), tc.IsTrue)
}

func (s *databaseSuite) TestConcurrentAcquisition(c *tc.C) {
	cfg := workerConfig(c)
	entered := make(chan struct{}, 2)
	ctx, cancel := context.WithCancel(c.Context())
	defer cancel()
	cfg.CreateArchive = func(ctx context.Context, _ string) (*corebackups.Metadata, string, func(), error) {
		entered <- struct{}{}
		<-ctx.Done()
		return nil, "", nil, ctx.Err()
	}
	cfg.LeaseService = leaseservice.NewService(leasestate.NewState(s.TxnRunnerFactory()))
	w1 := startWorker(c, cfg)
	cfg.LeaseService = leaseservice.NewService(leasestate.NewState(s.TxnRunnerFactory()))
	w2 := startWorker(c, cfg)
	first, second := createAsync(ctx, w1), createAsync(ctx, w2)
	<-entered
	select {
	case res := <-first:
		c.Check(res.err, tc.ErrorIs, errors.NotYetAvailable)
		cancel()
		c.Check((<-second).err, tc.ErrorIs, context.Canceled)
	case res := <-second:
		c.Check(res.err, tc.ErrorIs, errors.NotYetAvailable)
		cancel()
		c.Check((<-first).err, tc.ErrorIs, context.Canceled)
	case <-c.Context().Done():
		c.Fatal("neither competing request completed")
	}
}
