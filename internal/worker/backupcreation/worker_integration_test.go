// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backupcreation

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/tc"

	corebackups "github.com/juju/juju/core/backups"
	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/trace"
	leaseservice "github.com/juju/juju/domain/lease/service"
	leasestate "github.com/juju/juju/domain/lease/state"
	schematesting "github.com/juju/juju/domain/schema/testing"
	internallease "github.com/juju/juju/internal/lease"
	leaseworker "github.com/juju/juju/internal/worker/lease"
)

type workerIntegrationSuite struct {
	schematesting.ControllerSuite
}

func TestWorkerIntegrationSuite(t *testing.T) {
	tc.Run(t, &workerIntegrationSuite{})
}

func (s *workerIntegrationSuite) TestSameControllerContention(c *tc.C) {
	s.testContention(c, false)
}

func (s *workerIntegrationSuite) TestCrossControllerContention(c *tc.C) {
	s.testContention(c, true)
}

func (s *workerIntegrationSuite) testContention(c *tc.C, crossController bool) {
	store := leaseservice.NewService(leasestate.NewState(s.TxnRunnerFactory()))
	cfg := workerConfig(c)
	cfg.LeaseManager = s.startLeaseManager(c, cfg, store)
	key := lease.Key{
		Namespace: lease.BackupCreationNamespace,
		ModelUUID: cfg.ControllerModelUUID,
		Lease:     cfg.ControllerUUID,
	}

	started := make(chan struct{}, 1)
	finish := make(chan struct{})
	defer close(finish)
	var generations atomic.Int32
	var firstHolder string
	var w *Worker
	cfg.CreateArchive = backupCreatorFunc(func(ctx context.Context, notes string) (*corebackups.Metadata, string, func(), error) {
		generations.Add(1)
		if notes == "first" {
			started <- struct{}{}
			<-finish
		} else if notes == "next" {
			nextHolder := s.leaseHolder(c, store, key)
			c.Check(nextHolder, tc.Not(tc.Equals), firstHolder)
			w.revoke(ctx, firstHolder)
			c.Check(s.leaseHolder(c, store, key), tc.Equals, nextHolder)
		}
		return &corebackups.Metadata{}, "archive", func() {}, ctx.Err()
	})
	w = startWorker(c, cfg)
	contender := w
	if crossController {
		otherCfg := cfg
		otherCfg.ControllerNodeID = "1"
		otherCfg.LeaseManager = s.startLeaseManager(c, otherCfg, store)
		contender = startWorker(c, otherCfg)
	}

	done := make(chan result, 1)
	go func() {
		meta, archive, cleanup, err := w.Create(c.Context(), "first")
		done <- result{meta: meta, archive: archive, cleanup: cleanup, err: err}
	}()
	<-started
	firstHolder = s.leaseHolder(c, store, key)

	_, _, cleanup, err := contender.Create(c.Context(), "contender")
	if cleanup != nil {
		cleanup()
	}
	c.Check(err, tc.ErrorIs, errors.NotYetAvailable)
	c.Check(generations.Load(), tc.Equals, int32(1))
	c.Check(s.leaseHolder(c, store, key), tc.Equals, firstHolder)

	finish <- struct{}{}
	first := <-done
	c.Assert(first.err, tc.ErrorIsNil)
	c.Cleanup(first.cleanup)
	leases, err := store.Leases(c.Context(), key)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(leases, tc.HasLen, 0)

	// A new creation is admitted before the previous archive is cleaned up.
	// Its holder must also be protected from the previous attempt's release.
	_, _, cleanup, err = w.Create(c.Context(), "next")
	c.Assert(err, tc.ErrorIsNil)
	cleanup()
	leases, err = store.Leases(c.Context(), key)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(leases, tc.HasLen, 0)
}

func (s *workerIntegrationSuite) startLeaseManager(c *tc.C, cfg Config, store lease.Store) *leaseworker.Manager {
	manager, err := leaseworker.NewManager(leaseworker.ManagerConfig{
		Store:           store,
		SecretaryFinder: internallease.NewSecretaryFinder(cfg.ControllerUUID),
		Clock:           clock.WallClock,
		MaxSleep:        time.Hour,
		Tracer:          trace.NoopTracer{},
		Logger:          cfg.Logger,
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Cleanup(func() {
		manager.Kill()
		// Shutdown can cancel a database read while checking lease expiry.
		c.Check(manager.Wait(), tc.Or(tc.ErrorIsNil, tc.ErrorIs), context.Canceled)
	})
	return manager
}

func (s *workerIntegrationSuite) leaseHolder(c *tc.C, store lease.Store, key lease.Key) string {
	leases, err := store.Leases(c.Context(), key)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(leases, tc.HasLen, 1)
	return leases[key].Holder
}
