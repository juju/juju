// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backuplock_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/tc"

	corelease "github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/trace"
	leaseservice "github.com/juju/juju/domain/lease/service"
	leasestate "github.com/juju/juju/domain/lease/state"
	schematesting "github.com/juju/juju/domain/schema/testing"
	internallease "github.com/juju/juju/internal/lease"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
	"github.com/juju/juju/internal/worker/backuplock"
	leaseworker "github.com/juju/juju/internal/worker/lease"
)

type lockSuite struct {
	schematesting.ControllerSuite
	clock          *testclock.Clock
	store          *leaseservice.Service
	controllerUUID string
	modelUUID      string
}

func TestLockSuite(t *testing.T) { tc.Run(t, &lockSuite{}) }

func (s *lockSuite) SetUpTest(c *tc.C) {
	s.ControllerSuite.SetUpTest(c)
	s.clock = testclock.NewClock(time.Now())
	s.store = leaseservice.NewService(leasestate.NewState(s.TxnRunnerFactory()))
	s.controllerUUID = uuid.MustNewUUID().String()
	s.modelUUID = uuid.MustNewUUID().String()
}

func (s *lockSuite) worker(c *tc.C, decorate ...func(corelease.Manager) corelease.Manager) *backuplock.Worker {
	manager, err := leaseworker.NewManager(leaseworker.ManagerConfig{
		SecretaryFinder: internallease.NewSecretaryFinder(s.controllerUUID),
		Store:           s.store,
		Clock:           s.clock,
		Logger:          loggertesting.WrapCheckLog(c),
		Tracer:          trace.NoopTracer{},
		MaxSleep:        time.Minute,
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Cleanup(func() {
		manager.Kill()
		err := manager.Wait()
		if !errors.Is(err, context.Canceled) {
			c.Check(err, tc.ErrorIsNil)
		}
	})
	var lockManager corelease.Manager = manager
	for _, wrap := range decorate {
		lockManager = wrap(lockManager)
	}
	w, err := backuplock.NewWorker(backuplock.Config{
		Manager:             lockManager,
		ControllerUUID:      s.controllerUUID,
		ControllerModelUUID: s.modelUUID,
		Clock:               s.clock,
		Logger:              loggertesting.WrapCheckLog(c),
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Cleanup(func() { w.Kill(); c.Check(w.Wait(), tc.ErrorIsNil) })
	return w
}

// Independent managers sharing a real controller database must contend on one
// lease. Releasing an old request must not revoke its successor's lease.
func (s *lockSuite) TestCrossNodeContentionAndStaleRelease(c *tc.C) {
	w1, w2 := s.worker(c), s.worker(c)
	ctx, release, err := w1.Acquire(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(ctx.Err(), tc.ErrorIsNil)
	_, _, err = w1.Acquire(c.Context())
	c.Check(err, tc.ErrorIs, errors.NotYetAvailable)
	_, _, err = w2.Acquire(c.Context())
	c.Check(err, tc.ErrorIs, errors.NotYetAvailable)
	release()
	ctx2, release2, err := w1.Acquire(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	defer release2()
	release()
	c.Check(ctx2.Err(), tc.ErrorIsNil)
	_, _, err = w2.Acquire(c.Context())
	c.Check(err, tc.ErrorIs, errors.NotYetAvailable)
}

func (s *lockSuite) TestConcurrentCrossNodeAcquisition(c *tc.C) {
	w1, w2 := s.worker(c), s.worker(c)
	type outcome struct {
		release func()
		err     error
	}
	results := make(chan outcome, 2)
	start := make(chan struct{})
	for _, w := range []*backuplock.Worker{w1, w2} {
		go func() { <-start; _, release, err := w.Acquire(c.Context()); results <- outcome{release, err} }()
	}
	close(start)
	successes, busy := 0, 0
	for range 2 {
		r := <-results
		if r.err == nil {
			successes++
			defer r.release()
		} else {
			c.Check(r.err, tc.ErrorIs, errors.NotYetAvailable)
			busy++
		}
	}
	c.Check(successes, tc.Equals, 1)
	c.Check(busy, tc.Equals, 1)
}

func (s *lockSuite) TestLeaseLossCancelsCreation(c *tc.C) {
	w := s.worker(c)
	ctx, release, err := w.Acquire(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	defer release()
	key := corelease.Key{Namespace: corelease.BackupCreationNamespace, ModelUUID: s.modelUUID, Lease: s.controllerUUID}
	leases, err := s.store.Leases(c.Context(), key)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(leases, tc.HasLen, 1)
	err = s.store.RevokeLease(c.Context(), key, leases[key].Holder)
	c.Assert(err, tc.ErrorIsNil)
	s.clock.Advance(20 * time.Second)
	<-ctx.Done()
	c.Check(context.Cause(ctx), tc.ErrorMatches, "renewing backup creation lease: .*not held.*")
}

func (s *lockSuite) TestCancelledRequestAndShutdown(c *tc.C) {
	w := s.worker(c)
	cancelled, cancel := context.WithCancel(c.Context())
	cancel()
	_, _, err := w.Acquire(cancelled)
	c.Check(err, tc.ErrorIs, context.Canceled)
	ctx, release, err := w.Acquire(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	w.Kill()
	c.Assert(w.Wait(), tc.ErrorIsNil)
	<-ctx.Done()
	release()
	leases, err := s.store.Leases(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	// Shutdown cancels reads but cannot revoke before their caller unwinds.
	c.Check(leases, tc.HasLen, 1)
}

func (s *lockSuite) TestRenewsWhileCreating(c *tc.C) {
	claims := make(chan string, 2)
	w := s.worker(c, func(manager corelease.Manager) corelease.Manager {
		return observingManager{Manager: manager, claim: func(claimer corelease.Claimer, name, holder string, duration time.Duration) error {
			err := claimer.Claim(name, holder, duration)
			claims <- holder
			return err
		}}
	})
	ctx, release, err := w.Acquire(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	defer release()
	holder := <-claims
	s.clock.Advance(20 * time.Second)
	c.Check(<-claims, tc.Equals, holder)
	c.Check(ctx.Err(), tc.ErrorIsNil)
	_, _, err = s.worker(c).Acquire(c.Context())
	c.Check(err, tc.ErrorIs, errors.NotYetAvailable)
}

func (s *lockSuite) TestExpiryCancelsStalledRenewal(c *tc.C) {
	renewing, unblock := make(chan struct{}), make(chan struct{})
	calls := 0
	w := s.worker(c, func(manager corelease.Manager) corelease.Manager {
		return observingManager{Manager: manager, claim: func(claimer corelease.Claimer, name, holder string, duration time.Duration) error {
			calls++
			if calls == 2 {
				close(renewing)
				select {
				case <-unblock:
				case <-c.Context().Done():
					return c.Context().Err()
				}
			}
			return claimer.Claim(name, holder, duration)
		}}
	})
	unblockRenewal := sync.OnceFunc(func() { close(unblock) })
	c.Cleanup(unblockRenewal)
	ctx, release, err := w.Acquire(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	s.clock.Advance(20 * time.Second)
	<-renewing
	s.clock.Advance(40 * time.Second)
	<-ctx.Done()
	c.Check(context.Cause(ctx), tc.ErrorMatches, "backup creation lease expired")
	unblockRenewal()
	release()
}

// observingManager intercepts claims while preserving real database operations.
type observingManager struct {
	corelease.Manager
	claim func(corelease.Claimer, string, string, time.Duration) error
}

func (m observingManager) Claimer(namespace, modelUUID string) (corelease.Claimer, error) {
	claimer, err := m.Manager.Claimer(namespace, modelUUID)
	return observingClaimer{Claimer: claimer, claim: m.claim}, err
}

type observingClaimer struct {
	corelease.Claimer
	claim func(corelease.Claimer, string, string, time.Duration) error
}

func (c observingClaimer) Claim(name, holder string, duration time.Duration) error {
	return c.claim(c.Claimer, name, holder, duration)
}
