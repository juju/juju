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

	corebackups "github.com/juju/juju/core/backups"
	"github.com/juju/juju/core/lease"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
)

type workerSuite struct{}

func TestWorkerSuite(t *testing.T) { tc.Run(t, &workerSuite{}) }

func workerConfig(c *tc.C) Config {
	return Config{
		LeaseManager:        leaseManagerStub{},
		ControllerUUID:      uuid.MustNewUUID().String(),
		ControllerModelUUID: uuid.MustNewUUID().String(),
		ControllerNodeID:    "0",
		CreateArchive: backupCreatorFunc(func(context.Context, string) (*corebackups.Metadata, string, func(), error) {
			return &corebackups.Metadata{}, "archive", func() {}, nil
		}),
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

func (s *workerSuite) TestValidate(c *tc.C) {
	tests := []struct {
		change  func(*Config)
		message string
	}{
		{func(c *Config) { c.LeaseManager = nil }, "nil LeaseManager not valid"},
		{func(c *Config) { c.CreateArchive = nil }, "nil CreateArchive not valid"},
		{func(c *Config) { c.Clock = nil }, "nil Clock not valid"},
		{func(c *Config) { c.Logger = nil }, "nil Logger not valid"},
		{func(c *Config) { c.ControllerUUID = "bad" }, "backup controller UUID: .*"},
		{func(c *Config) { c.ControllerModelUUID = "bad" }, "backup controller model UUID: .*"},
		{func(c *Config) { c.ControllerNodeID = "" }, "empty ControllerNodeID not valid"},
	}
	for _, test := range tests {
		cfg := workerConfig(c)
		test.change(&cfg)
		_, err := NewWorker(cfg)
		c.Check(err, tc.ErrorMatches, test.message)
	}
}

func (s *workerSuite) TestNewWorkerFailsWhenBindingLeaseOperations(c *tc.C) {
	for _, test := range []struct {
		manager leaseManagerStub
		message string
	}{
		{leaseManagerStub{claimerErr: errors.New("claimer failed")}, "getting backup creation lease claimer: claimer failed"},
		{leaseManagerStub{checkerErr: errors.New("checker failed")}, "getting backup creation lease checker: checker failed"},
		{leaseManagerStub{revokerErr: errors.New("revoker failed")}, "getting backup creation lease revoker: revoker failed"},
	} {
		cfg := workerConfig(c)
		cfg.LeaseManager = test.manager
		_, err := NewWorker(cfg)
		c.Check(err, tc.ErrorMatches, test.message)
	}
}

func (s *workerSuite) TestCreateClaimsChecksAndRevokesControllerNodeLease(c *tc.C) {
	cfg := workerConfig(c)
	var claimed, checked, revoked atomic.Bool
	cfg.LeaseManager = leaseManagerStub{operations: leaseOperations{
		claim: func(name, holder string, duration time.Duration) error {
			c.Check(name, tc.Equals, cfg.ControllerUUID)
			c.Check(holder, tc.Equals, cfg.ControllerNodeID)
			c.Check(duration, tc.Equals, leaseDuration)
			claimed.Store(true)
			return nil
		},
		check: func(name, holder string) error {
			c.Check(claimed.Load(), tc.IsTrue)
			c.Check(name, tc.Equals, cfg.ControllerUUID)
			c.Check(holder, tc.Equals, cfg.ControllerNodeID)
			checked.Store(true)
			return nil
		},
		revoke: func(name, holder string) error {
			c.Check(checked.Load(), tc.IsTrue)
			c.Check(name, tc.Equals, cfg.ControllerUUID)
			c.Check(holder, tc.Equals, cfg.ControllerNodeID)
			revoked.Store(true)
			return nil
		},
	}}
	w := startWorker(c, cfg)
	_, _, cleanup, err := w.Create(c.Context(), "notes")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(checked.Load(), tc.IsTrue)
	c.Check(revoked.Load(), tc.IsTrue)
	cleanup()
}

func (s *workerSuite) TestClaimContentionDoesNotGenerateOrRevoke(c *tc.C) {
	for _, claimErr := range []error{lease.ErrClaimDenied, lease.ErrHeld} {
		cfg := workerConfig(c)
		var generated, revoked atomic.Bool
		cfg.LeaseManager = leaseManagerStub{operations: leaseOperations{
			claim:  func(string, string, time.Duration) error { return claimErr },
			revoke: func(string, string) error { revoked.Store(true); return nil },
		}}
		cfg.CreateArchive = backupCreatorFunc(func(context.Context, string) (*corebackups.Metadata, string, func(), error) {
			generated.Store(true)
			return nil, "", nil, nil
		})
		w := startWorker(c, cfg)
		_, _, _, err := w.Create(c.Context(), "")
		c.Check(err, tc.ErrorIs, errors.NotYetAvailable)
		c.Check(generated.Load(), tc.IsFalse)
		c.Check(revoked.Load(), tc.IsFalse)
	}
}

func (s *workerSuite) TestTokenFailureDoesNotGenerateAndRevokes(c *tc.C) {
	cfg := workerConfig(c)
	var generated, revoked atomic.Bool
	cfg.LeaseManager = leaseManagerStub{operations: leaseOperations{
		check:  func(string, string) error { return lease.ErrNotHeld },
		revoke: func(string, string) error { revoked.Store(true); return nil },
	}}
	cfg.CreateArchive = backupCreatorFunc(func(context.Context, string) (*corebackups.Metadata, string, func(), error) {
		generated.Store(true)
		return nil, "", nil, nil
	})
	w := startWorker(c, cfg)
	_, _, _, err := w.Create(c.Context(), "")
	c.Check(err, tc.ErrorIs, lease.ErrNotHeld)
	c.Check(generated.Load(), tc.IsFalse)
	c.Check(revoked.Load(), tc.IsTrue)
}

func (s *workerSuite) TestLeaseExpiryCancelsGenerationAndCleansArchive(c *tc.C) {
	cfg := workerConfig(c)
	var cleaned atomic.Bool
	cfg.LeaseManager = leaseManagerStub{operations: leaseOperations{
		wait: func(context.Context, string, chan<- struct{}) error { return nil },
	}}
	cfg.CreateArchive = backupCreatorFunc(func(context.Context, string) (*corebackups.Metadata, string, func(), error) {
		c.Error("generation started after lease expiry")
		return &corebackups.Metadata{}, "archive", func() { cleaned.Store(true) }, nil
	})
	w := startWorker(c, cfg)
	_, _, _, err := w.Create(c.Context(), "")
	c.Check(err, tc.ErrorMatches, "backup creation lease expired.*")
	c.Check(cleaned.Load(), tc.IsFalse)
}

func (s *workerSuite) TestRenewalFailureCancelsGenerationAndCleansArchive(c *tc.C) {
	cfg := workerConfig(c)
	clk := &observedClock{Clock: testclock.NewClock(time.Now()), timers: make(chan struct{}, 1)}
	cfg.Clock = clk
	failure := errors.New("renewal failed")
	var claims atomic.Int32
	var cleaned atomic.Bool
	cfg.LeaseManager = leaseManagerStub{operations: leaseOperations{
		claim: func(string, string, time.Duration) error {
			if claims.Add(1) == 2 {
				return failure
			}
			return nil
		},
	}}
	cfg.CreateArchive = backupCreatorFunc(func(ctx context.Context, _ string) (*corebackups.Metadata, string, func(), error) {
		<-ctx.Done()
		return &corebackups.Metadata{}, "archive", func() { cleaned.Store(true) }, ctx.Err()
	})
	w := startWorker(c, cfg)
	done := make(chan error, 1)
	go func() { _, _, _, err := w.Create(c.Context(), ""); done <- err }()
	<-clk.timers
	clk.Advance(renewInterval)
	c.Check(<-done, tc.ErrorIs, context.Canceled)
	c.Check(cleaned.Load(), tc.IsTrue)
}

func (s *workerSuite) TestReleaseFailureDoesNotDiscardArchive(c *tc.C) {
	cfg := workerConfig(c)
	cfg.LeaseManager = leaseManagerStub{operations: leaseOperations{
		revoke: func(string, string) error { return errors.New("release failed") },
	}}
	w := startWorker(c, cfg)
	_, _, cleanup, err := w.Create(c.Context(), "")
	c.Assert(err, tc.ErrorIsNil)
	cleanup()
}

func (s *workerSuite) TestCreateReturnsWorkerDyingAfterShutdown(c *tc.C) {
	w := startWorker(c, workerConfig(c))
	w.Kill()
	_, _, _, err := w.Create(c.Context(), "")
	c.Check(err, tc.ErrorIs, corebackups.ErrBackupCreationWorkerDying)
}

func (s *workerSuite) TestAcceptedRequestReturnsWorkerDyingDuringShutdown(c *tc.C) {
	cfg := workerConfig(c)
	started := make(chan struct{})
	cfg.CreateArchive = backupCreatorFunc(func(ctx context.Context, _ string) (*corebackups.Metadata, string, func(), error) {
		close(started)
		<-ctx.Done()
		return nil, "", nil, ctx.Err()
	})
	w := startWorker(c, cfg)
	done := make(chan error, 1)
	go func() {
		_, _, _, err := w.Create(c.Context(), "")
		done <- err
	}()
	<-started
	w.Kill()
	c.Check(<-done, tc.ErrorIs, corebackups.ErrBackupCreationWorkerDying)
}

type backupCreatorFunc func(context.Context, string) (*corebackups.Metadata, string, func(), error)

func (f backupCreatorFunc) Create(ctx context.Context, notes string) (*corebackups.Metadata, string, func(), error) {
	return f(ctx, notes)
}

type leaseManagerStub struct {
	operations leaseOperations
	claimerErr error
	checkerErr error
	revokerErr error
}

func (s leaseManagerStub) Claimer(string, string) (lease.Claimer, error) {
	return s.operations, s.claimerErr
}
func (s leaseManagerStub) Checker(string, string) (lease.Checker, error) {
	return s.operations, s.checkerErr
}
func (s leaseManagerStub) Revoker(string, string) (lease.Revoker, error) {
	return s.operations, s.revokerErr
}
func (s leaseManagerStub) Pinner(string, string) (lease.Pinner, error) { return nil, nil }
func (s leaseManagerStub) Reader(string, string) (lease.Reader, error) { return nil, nil }

type leaseOperations struct {
	claim  func(string, string, time.Duration) error
	wait   func(context.Context, string, chan<- struct{}) error
	check  func(string, string) error
	revoke func(string, string) error
}

func (s leaseOperations) Claim(name, holder string, duration time.Duration) error {
	if s.claim != nil {
		return s.claim(name, holder, duration)
	}
	return nil
}

func (s leaseOperations) WaitUntilExpired(ctx context.Context, name string, started chan<- struct{}) error {
	if s.wait != nil {
		return s.wait(ctx, name, started)
	}
	if started != nil {
		close(started)
	}
	<-ctx.Done()
	return lease.ErrWaitCancelled
}

func (s leaseOperations) Token(name, holder string) lease.Token {
	return leaseTokenStub{check: func() error {
		if s.check == nil {
			return nil
		}
		return s.check(name, holder)
	}}
}

func (s leaseOperations) Revoke(name, holder string) error {
	if s.revoke != nil {
		return s.revoke(name, holder)
	}
	return nil
}

type leaseTokenStub struct{ check func() error }

func (s leaseTokenStub) Check() error {
	if s.check == nil {
		return nil
	}
	return s.check()
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
