// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backup

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/juju/clock"
	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/tc"

	corebackups "github.com/juju/juju/core/backups"
	"github.com/juju/juju/core/lease"
	internalerrors "github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/uuid"
)

type workerSuite struct{ coretesting.BaseSuite }

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

func (s *workerSuite) TestCreateClaimsChecksRenewsAndRevokesAttemptLease(c *tc.C) {
	cfg := workerConfig(c)
	clk := &observedClock{Clock: testclock.NewClock(time.Now()), timers: make(chan struct{}, 1)}
	cfg.Clock = clk
	var attemptHolder string
	var claims atomic.Int32
	renewed := make(chan struct{})
	var claimed, checked, revoked atomic.Bool
	cfg.LeaseManager = leaseManagerStub{operations: leaseOperations{
		claim: func(name, holder string, duration time.Duration) error {
			c.Check(name, tc.Equals, cfg.ControllerUUID)
			c.Check(duration, tc.Equals, leaseDuration)
			if claims.Add(1) == 1 {
				attemptHolder = holder
				c.Check(strings.HasPrefix(holder, cfg.ControllerNodeID+":"), tc.IsTrue)
				_, err := uuid.UUIDFromString(strings.TrimPrefix(holder, cfg.ControllerNodeID+":"))
				c.Assert(err, tc.ErrorIsNil)
			} else {
				c.Check(holder, tc.Equals, attemptHolder)
				close(renewed)
			}
			claimed.Store(true)
			return nil
		},
		check: func(name, holder string) error {
			c.Check(claimed.Load(), tc.IsTrue)
			c.Check(name, tc.Equals, cfg.ControllerUUID)
			c.Check(holder, tc.Equals, attemptHolder)
			checked.Store(true)
			return nil
		},
		revoke: func(name, holder string) error {
			c.Check(checked.Load(), tc.IsTrue)
			c.Check(name, tc.Equals, cfg.ControllerUUID)
			c.Check(holder, tc.Equals, attemptHolder)
			revoked.Store(true)
			return nil
		},
	}}
	cfg.CreateArchive = backupCreatorFunc(func(context.Context, string) (*corebackups.Metadata, string, func(), error) {
		<-renewed
		return &corebackups.Metadata{}, "archive", func() {}, nil
	})
	w := startWorker(c, cfg)
	done := make(chan result, 1)
	go func() {
		meta, archive, cleanup, err := w.Create(c.Context(), "notes")
		done <- result{meta: meta, archive: archive, cleanup: cleanup, err: err}
	}()
	<-clk.timers
	clk.Advance(renewInterval)
	res := <-done
	c.Assert(res.err, tc.ErrorIsNil)
	c.Check(claims.Load(), tc.Equals, int32(2))
	c.Check(checked.Load(), tc.IsTrue)
	c.Check(revoked.Load(), tc.IsTrue)
	res.cleanup()
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
	failure := errors.New("renewal failed")
	archiveFailure := errors.New("archive failed")
	for _, test := range []struct {
		name      string
		createErr error
		expected  error
	}{
		{"success", nil, failure},
		{"cancelled", context.Canceled, failure},
		{"wrapped cancellation", internalerrors.Capture(internalerrors.Errorf("writing archive: %w", context.Canceled)), failure},
		{"archive failure", archiveFailure, archiveFailure},
	} {
		c.Run(test.name, func(t *testing.T) {
			c := &tc.C{T: t}
			cfg := workerConfig(c)
			clk := &observedClock{Clock: testclock.NewClock(time.Now()), timers: make(chan struct{}, 1)}
			cfg.Clock = clk
			var claims, cleaned atomic.Int32
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
				c.Check(context.Cause(ctx), tc.ErrorIs, failure)
				return &corebackups.Metadata{}, "archive", func() { cleaned.Add(1) }, test.createErr
			})
			w := startWorker(c, cfg)
			done := make(chan result, 1)
			go func() {
				meta, archive, cleanup, err := w.Create(c.Context(), "notes")
				done <- result{meta: meta, archive: archive, cleanup: cleanup, err: err}
			}()
			<-clk.timers
			clk.Advance(renewInterval)
			res := <-done
			c.Check(res.err, tc.ErrorIs, test.expected)
			c.Check(res.meta, tc.IsNil)
			c.Check(res.archive, tc.Equals, "")
			c.Check(res.cleanup, tc.IsNil)
			c.Check(cleaned.Load(), tc.Equals, int32(1))
		})
	}
}

func (s *workerSuite) TestGenerationCancellationWithoutContextCause(c *tc.C) {
	cfg := workerConfig(c)
	cfg.CreateArchive = backupCreatorFunc(func(context.Context, string) (*corebackups.Metadata, string, func(), error) {
		return nil, "", nil, internalerrors.Capture(context.Canceled)
	})
	w := startWorker(c, cfg)
	_, _, _, err := w.Create(c.Context(), "notes")
	c.Check(err, tc.ErrorIs, context.Canceled)
}

func (s *workerSuite) TestCancellationDuringReleaseCleansArchive(c *tc.C) {
	ctx, cancel := context.WithCancelCause(c.Context())
	defer cancel(nil)
	failure := errors.New("caller cancelled backup")
	cleaned := make(chan struct{})
	cfg := workerConfig(c)
	cfg.CreateArchive = backupCreatorFunc(func(context.Context, string) (*corebackups.Metadata, string, func(), error) {
		return &corebackups.Metadata{}, "archive", func() { close(cleaned) }, nil
	})
	cfg.LeaseManager = leaseManagerStub{operations: leaseOperations{
		revoke: func(string, string) error {
			cancel(failure)
			return nil
		},
	}}
	w := startWorker(c, cfg)
	meta, archive, cleanup, err := w.Create(ctx, "notes")
	c.Check(err, tc.ErrorIs, failure)
	c.Check(meta, tc.IsNil)
	c.Check(archive, tc.Equals, "")
	c.Check(cleanup, tc.IsNil)
	<-cleaned
}

func (s *workerSuite) TestShutdownDuringReleasePreservesArchive(c *tc.C) {
	cfg := workerConfig(c)
	expected := &corebackups.Metadata{}
	var cleaned atomic.Bool
	cfg.CreateArchive = backupCreatorFunc(func(context.Context, string) (*corebackups.Metadata, string, func(), error) {
		return expected, "archive", func() { cleaned.Store(true) }, nil
	})
	var w *Worker
	cfg.LeaseManager = leaseManagerStub{operations: leaseOperations{
		revoke: func(string, string) error { w.Kill(); return nil },
	}}
	w = startWorker(c, cfg)
	meta, archive, cleanup, err := w.Create(c.Context(), "notes")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(meta, tc.Equals, expected)
	c.Check(archive, tc.Equals, "archive")
	c.Assert(cleanup, tc.NotNil)
	c.Assert(w.Wait(), tc.ErrorIsNil)
	c.Check(cleaned.Load(), tc.IsFalse)
	cleanup()
	c.Check(cleaned.Load(), tc.IsTrue)
}

func (s *workerSuite) TestShutdownWaitsForHandoffCleanup(c *tc.C) {
	synctest.Test(c.T, func(t *testing.T) {
		c := &tc.C{T: t}
		ctx, cancel := context.WithCancelCause(c.Context())
		defer cancel(nil)
		failure := errors.New("caller cancelled backup")
		cleaning := make(chan struct{})
		finish := make(chan struct{})
		defer close(finish)
		cfg := workerConfig(c)
		cfg.CreateArchive = backupCreatorFunc(func(context.Context, string) (*corebackups.Metadata, string, func(), error) {
			return &corebackups.Metadata{}, "archive", func() {
				close(cleaning)
				<-finish
			}, nil
		})
		var w *Worker
		cfg.LeaseManager = leaseManagerStub{operations: leaseOperations{
			revoke: func(string, string) error {
				cancel(failure)
				w.Kill()
				return nil
			},
		}}
		w = startWorker(c, cfg)
		defer w.Kill()
		created := make(chan result, 1)
		go func() {
			meta, archive, cleanup, err := w.Create(ctx, "notes")
			created <- result{meta: meta, archive: archive, cleanup: cleanup, err: err}
		}()
		<-cleaning
		stopped := make(chan error, 1)
		go func() { stopped <- w.Wait() }()
		synctest.Wait()
		c.Check(len(stopped), tc.Equals, 0)
		c.Check(len(created), tc.Equals, 0)
		finish <- struct{}{}
		res := <-created
		c.Check(res.err, tc.ErrorIs, failure)
		c.Check(res.meta, tc.IsNil)
		c.Check(res.archive, tc.Equals, "")
		c.Check(res.cleanup, tc.IsNil)
		c.Assert(<-stopped, tc.ErrorIsNil)
	})
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

func (s *workerSuite) TestReleaseWaitsForRenewal(c *tc.C) {
	for _, createErr := range []error{nil, errors.New("archive failed")} {
		synctest.Test(c.T, func(t *testing.T) {
			c := &tc.C{T: t}
			cfg := workerConfig(c)
			clk := &observedClock{Clock: testclock.NewClock(time.Now()), timers: make(chan struct{}, 1)}
			cfg.Clock = clk
			renewing := make(chan struct{})
			finish := make(chan struct{})
			defer close(finish)
			var claims, cleaned atomic.Int32
			var renewed, revoked atomic.Bool
			cfg.LeaseManager = leaseManagerStub{operations: leaseOperations{
				claim: func(string, string, time.Duration) error {
					if claims.Add(1) == 2 {
						close(renewing)
						<-finish
						renewed.Store(true)
					}
					return nil
				},
				revoke: func(string, string) error {
					c.Check(renewed.Load(), tc.IsTrue)
					revoked.Store(true)
					return nil
				},
			}}
			cfg.CreateArchive = backupCreatorFunc(func(context.Context, string) (*corebackups.Metadata, string, func(), error) {
				<-renewing
				return &corebackups.Metadata{}, "archive", func() { cleaned.Add(1) }, createErr
			})
			w := startWorker(c, cfg)
			defer w.Kill()
			done := make(chan result, 1)
			go func() {
				meta, archive, cleanup, err := w.Create(c.Context(), "notes")
				done <- result{meta: meta, archive: archive, cleanup: cleanup, err: err}
			}()
			<-clk.timers
			clk.Advance(renewInterval)
			// Generation has finished, but renewal is still blocked.
			synctest.Wait()
			c.Check(revoked.Load(), tc.IsFalse)
			c.Check(len(done), tc.Equals, 0)
			finish <- struct{}{}
			res := <-done
			synctest.Wait()
			c.Check(revoked.Load(), tc.IsTrue)
			c.Check(claims.Load(), tc.Equals, int32(2))
			if createErr != nil {
				c.Check(res.err, tc.ErrorIs, createErr)
				c.Check(res.cleanup, tc.IsNil)
			} else {
				c.Assert(res.err, tc.ErrorIsNil)
				c.Check(cleaned.Load(), tc.Equals, int32(0))
				res.cleanup()
			}
			c.Check(cleaned.Load(), tc.Equals, int32(1))
		})
	}
}

func (s *workerSuite) TestReleaseWaitsForExpiryMonitor(c *tc.C) {
	for _, checkErr := range []error{nil, lease.ErrNotHeld} {
		synctest.Test(c.T, func(t *testing.T) {
			c := &tc.C{T: t}
			cfg := workerConfig(c)
			stopping := make(chan struct{})
			finish := make(chan struct{})
			defer close(finish)
			var monitored, revoked atomic.Bool
			cfg.LeaseManager = leaseManagerStub{operations: leaseOperations{
				wait: func(ctx context.Context, _ string, started chan<- struct{}) error {
					close(started)
					<-ctx.Done()
					close(stopping)
					<-finish
					monitored.Store(true)
					return lease.ErrWaitCancelled
				},
				check: func(string, string) error { return checkErr },
				revoke: func(string, string) error {
					c.Check(monitored.Load(), tc.IsTrue)
					revoked.Store(true)
					return nil
				},
			}}
			w := startWorker(c, cfg)
			defer w.Kill()
			done := make(chan result, 1)
			go func() {
				meta, archive, cleanup, err := w.Create(c.Context(), "notes")
				done <- result{meta: meta, archive: archive, cleanup: cleanup, err: err}
			}()
			<-stopping
			synctest.Wait()
			c.Check(revoked.Load(), tc.IsFalse)
			c.Check(len(done), tc.Equals, 0)
			finish <- struct{}{}
			res := <-done
			synctest.Wait()
			c.Check(revoked.Load(), tc.IsTrue)
			if checkErr != nil {
				c.Check(res.err, tc.ErrorIs, checkErr)
				c.Check(res.cleanup, tc.IsNil)
			} else {
				c.Assert(res.err, tc.ErrorIsNil)
				res.cleanup()
			}
		})
	}
}

func (s *workerSuite) TestCreateReturnsCancellationAfterShutdown(c *tc.C) {
	w := startWorker(c, workerConfig(c))
	w.Kill()
	c.Assert(w.Wait(), tc.ErrorIsNil)
	_, _, _, err := w.Create(c.Context(), "")
	c.Check(err, tc.ErrorIs, context.Canceled)
}

func (s *workerSuite) TestShutdownWaitsForAcceptedRequests(c *tc.C) {
	for _, operation := range []string{"claim", "check"} {
		synctest.Test(c.T, func(t *testing.T) {
			c := &tc.C{T: t}
			cfg := workerConfig(c)
			entered := make(chan struct{}, 2)
			finish := []chan struct{}{make(chan struct{}), make(chan struct{})}
			defer close(finish[0])
			defer close(finish[1])
			var acquisitions, revoked atomic.Int32
			var generated atomic.Bool
			block := func() {
				i := acquisitions.Add(1) - 1
				entered <- struct{}{}
				<-finish[i]
			}
			cfg.LeaseManager = leaseManagerStub{operations: leaseOperations{
				claim: func(string, string, time.Duration) error {
					if operation == "claim" {
						block()
					}
					return nil
				},
				check: func(string, string) error {
					if operation == "check" {
						block()
					}
					return nil
				},
				revoke: func(string, string) error { revoked.Add(1); return nil },
			}}
			cfg.CreateArchive = backupCreatorFunc(func(context.Context, string) (*corebackups.Metadata, string, func(), error) {
				generated.Store(true)
				return &corebackups.Metadata{}, "archive", func() {}, nil
			})
			w := startWorker(c, cfg)
			defer w.Kill()
			created := make(chan error, 2)
			for range 2 {
				go func() { _, _, _, err := w.Create(c.Context(), "notes"); created <- err }()
			}
			<-entered
			<-entered
			w.Kill()
			stopped := make(chan error, 1)
			go func() { stopped <- w.Wait() }()
			synctest.Wait()
			c.Check(len(stopped), tc.Equals, 0)
			c.Check(len(created), tc.Equals, 0)
			finish[0] <- struct{}{}
			synctest.Wait()
			c.Check(len(stopped), tc.Equals, 0)
			c.Check(len(created), tc.Equals, 1)
			finish[1] <- struct{}{}
			for range 2 {
				c.Check(<-created, tc.ErrorIs, context.Canceled)
			}
			c.Assert(<-stopped, tc.ErrorIsNil)
			c.Check(generated.Load(), tc.IsFalse)
			c.Check(revoked.Load(), tc.Equals, int32(2))
		})
	}
}

func (s *workerSuite) TestCancellationWaitsForRequestCompletion(c *tc.C) {
	for _, shutdown := range []bool{false, true} {
		synctest.Test(c.T, func(t *testing.T) {
			c := &tc.C{T: t}
			cfg := workerConfig(c)
			started, finishGeneration := make(chan struct{}), make(chan struct{})
			cleaning, finishCleanup := make(chan struct{}), make(chan struct{})
			revoking, finishRelease := make(chan struct{}), make(chan struct{})
			defer close(finishGeneration)
			defer close(finishCleanup)
			defer close(finishRelease)
			var cleaned atomic.Int32
			cfg.CreateArchive = backupCreatorFunc(func(ctx context.Context, _ string) (*corebackups.Metadata, string, func(), error) {
				close(started)
				<-ctx.Done()
				<-finishGeneration
				return &corebackups.Metadata{}, "archive", func() {
					close(cleaning)
					<-finishCleanup
					cleaned.Add(1)
				}, nil
			})
			cfg.LeaseManager = leaseManagerStub{operations: leaseOperations{
				revoke: func(string, string) error {
					close(revoking)
					<-finishRelease
					return nil
				},
			}}
			w := startWorker(c, cfg)
			defer w.Kill()
			ctx, cancel := context.WithCancelCause(c.Context())
			defer cancel(nil)
			created := make(chan result, 1)
			go func() {
				meta, archive, cleanup, err := w.Create(ctx, "notes")
				created <- result{meta: meta, archive: archive, cleanup: cleanup, err: err}
			}()
			<-started
			expected := errors.New("caller cancelled backup")
			if shutdown {
				expected = context.Canceled
				w.Kill()
			} else {
				cancel(expected)
			}
			stopped := make(chan error, 1)
			go func() { stopped <- w.Wait() }()
			synctest.Wait()
			c.Check(len(stopped), tc.Equals, 0)
			c.Check(len(created), tc.Equals, 0)
			finishGeneration <- struct{}{}
			<-cleaning
			synctest.Wait()
			c.Check(len(stopped), tc.Equals, 0)
			c.Check(len(created), tc.Equals, 0)
			finishCleanup <- struct{}{}
			<-revoking
			synctest.Wait()
			c.Check(len(stopped), tc.Equals, 0)
			c.Check(len(created), tc.Equals, 0)
			finishRelease <- struct{}{}
			res := <-created
			c.Check(res.err, tc.ErrorIs, expected)
			c.Check(res.cleanup, tc.IsNil)
			c.Check(cleaned.Load(), tc.Equals, int32(1))
			w.Kill()
			c.Assert(<-stopped, tc.ErrorIsNil)
		})
	}
}

func (s *workerSuite) TestShutdownPreservesHandedOffArchive(c *tc.C) {
	cfg := workerConfig(c)
	var cleaned atomic.Bool
	cfg.CreateArchive = backupCreatorFunc(func(context.Context, string) (*corebackups.Metadata, string, func(), error) {
		return &corebackups.Metadata{}, "archive", func() { cleaned.Store(true) }, nil
	})
	w := startWorker(c, cfg)
	_, _, cleanup, err := w.Create(c.Context(), "notes")
	c.Assert(err, tc.ErrorIsNil)
	w.Kill()
	c.Assert(w.Wait(), tc.ErrorIsNil)
	c.Check(cleaned.Load(), tc.IsFalse)
	cleanup()
	c.Check(cleaned.Load(), tc.IsTrue)
}

func (s *workerSuite) TestAcceptedRequestReturnsCancellationDuringShutdown(c *tc.C) {
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
	c.Check(<-done, tc.ErrorIs, context.Canceled)
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
