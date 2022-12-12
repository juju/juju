// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease_test

import (
	"database/sql"
	"time"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	_ "github.com/mattn/go-sqlite3"
	gc "gopkg.in/check.v1"

	corelease "github.com/juju/juju/core/lease"
	"github.com/juju/juju/database/schema"
	"github.com/juju/juju/worker/lease"
)

type storeSuite struct {
	testing.IsolationSuite

	db     *sql.DB
	store  *lease.Store
	stopCh chan struct{}
}

var _ = gc.Suite(&storeSuite{})

func (s *storeSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	var err error
	s.db, err = sql.Open("sqlite3", ":memory:?_foreign_keys=1")
	c.Assert(err, jc.ErrorIsNil)

	s.primeDB(c)
	s.store = lease.NewStore(lease.StoreConfig{
		DB:     s.db,
		Logger: lease.StubLogger{},
	})

	// Single-buffered to allow us to queue up a stoppage.
	s.stopCh = make(chan struct{}, 1)
}

func (s *storeSuite) TestClaimLeaseSuccessAndLeaseQueries(c *gc.C) {
	pgKey := corelease.Key{
		Namespace: "application",
		ModelUUID: "model-uuid",
		Lease:     "postgresql",
	}

	pgReq := corelease.Request{
		Holder:   "postgresql/0",
		Duration: time.Minute,
	}

	// Add 2 leases.
	err := s.store.ClaimLease(pgKey, pgReq, s.stopCh)
	c.Assert(err, jc.ErrorIsNil)

	mmKey := pgKey
	mmKey.Lease = "mattermost"

	mmReq := pgReq
	mmReq.Holder = "mattermost/0"

	err = s.store.ClaimLease(mmKey, mmReq, s.stopCh)
	c.Assert(err, jc.ErrorIsNil)

	// Check all the leases.
	leases, err := s.store.Leases()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(leases, gc.HasLen, 2)
	c.Check(leases[pgKey].Holder, gc.Equals, "postgresql/0")
	c.Check(leases[pgKey].Expiry.After(time.Now().UTC()), jc.IsTrue)
	c.Check(leases[mmKey].Holder, gc.Equals, "mattermost/0")
	c.Check(leases[mmKey].Expiry.After(time.Now().UTC()), jc.IsTrue)

	// Check with a filter.
	leases, err = s.store.Leases(pgKey)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(leases, gc.HasLen, 1)
	c.Check(leases[pgKey].Holder, gc.Equals, "postgresql/0")

	// Add a lease from a different group,
	// and check that the group returns the application leases.
	err = s.store.ClaimLease(
		corelease.Key{
			Namespace: "controller",
			ModelUUID: "controller-model-uuid",
			Lease:     "singular",
		},
		corelease.Request{
			Holder:   "machine/0",
			Duration: time.Minute,
		},
		s.stopCh,
	)
	c.Assert(err, jc.ErrorIsNil)

	leases, err = s.store.LeaseGroup("application", "model-uuid")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(leases, gc.HasLen, 2)
	c.Check(leases[pgKey].Holder, gc.Equals, "postgresql/0")
	c.Check(leases[mmKey].Holder, gc.Equals, "mattermost/0")
}

func (s *storeSuite) TestClaimLeaseAlreadyHeld(c *gc.C) {
	key := corelease.Key{
		Namespace: "controller",
		ModelUUID: "controller-model-uuid",
		Lease:     "singular",
	}

	req := corelease.Request{
		Holder:   "machine/0",
		Duration: time.Minute,
	}

	err := s.store.ClaimLease(key, req, s.stopCh)
	c.Assert(err, jc.ErrorIsNil)

	err = s.store.ClaimLease(key, req, s.stopCh)
	c.Assert(errors.Is(err, corelease.ErrHeld), jc.IsTrue)
}

func (s *storeSuite) TestExtendLeaseSuccess(c *gc.C) {
	key := corelease.Key{
		Namespace: "application",
		ModelUUID: "model-uuid",
		Lease:     "postgresql",
	}

	req := corelease.Request{
		Holder:   "postgresql/0",
		Duration: time.Minute,
	}

	err := s.store.ClaimLease(key, req, s.stopCh)
	c.Assert(err, jc.ErrorIsNil)

	leases, err := s.store.Leases(key)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(leases, gc.HasLen, 1)

	// Save the expiry for later comparison.
	originalExpiry := leases[key].Expiry

	req.Duration = 2 * time.Minute
	err = s.store.ExtendLease(key, req, s.stopCh)
	c.Assert(err, jc.ErrorIsNil)

	leases, err = s.store.Leases(key)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(leases, gc.HasLen, 1)

	// Check that we extended.
	c.Check(leases[key].Expiry.After(originalExpiry), jc.IsTrue)
}

func (s *storeSuite) TestExtendLeaseNotHeldInvalid(c *gc.C) {
	key := corelease.Key{
		Namespace: "application",
		ModelUUID: "model-uuid",
		Lease:     "postgresql",
	}

	req := corelease.Request{
		Holder:   "postgresql/0",
		Duration: time.Minute,
	}

	err := s.store.ExtendLease(key, req, s.stopCh)
	c.Assert(errors.Is(err, corelease.ErrInvalid), jc.IsTrue)
}

func (s *storeSuite) TestRevokeLeaseSuccess(c *gc.C) {
	key := corelease.Key{
		Namespace: "application",
		ModelUUID: "model-uuid",
		Lease:     "postgresql",
	}

	req := corelease.Request{
		Holder:   "postgresql/0",
		Duration: time.Minute,
	}

	err := s.store.ClaimLease(key, req, s.stopCh)
	c.Assert(err, jc.ErrorIsNil)

	err = s.store.RevokeLease(key, req.Holder, s.stopCh)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *storeSuite) TestRevokeLeaseNotHeldInvalid(c *gc.C) {
	key := corelease.Key{
		Namespace: "application",
		ModelUUID: "model-uuid",
		Lease:     "postgresql",
	}

	err := s.store.RevokeLease(key, "not-the-holder", s.stopCh)
	c.Assert(errors.Is(err, corelease.ErrInvalid), jc.IsTrue)
}

func (s *storeSuite) TestPinUnpinLeaseAndPinQueries(c *gc.C) {
	pgKey := corelease.Key{
		Namespace: "application",
		ModelUUID: "model-uuid",
		Lease:     "postgresql",
	}

	pgReq := corelease.Request{
		Holder:   "postgresql/0",
		Duration: time.Minute,
	}

	err := s.store.ClaimLease(pgKey, pgReq, s.stopCh)
	c.Assert(err, jc.ErrorIsNil)

	// One entity pins the lease.
	err = s.store.PinLease(pgKey, "machine/6", s.stopCh)
	c.Assert(err, jc.ErrorIsNil)

	// The same lease/entity is a no-op without error.
	err = s.store.PinLease(pgKey, "machine/6", s.stopCh)
	c.Assert(err, jc.ErrorIsNil)

	// Another entity pinning the same lease.
	err = s.store.PinLease(pgKey, "machine/7", s.stopCh)
	c.Assert(err, jc.ErrorIsNil)

	pins, err := s.store.Pinned()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(pins, gc.HasLen, 1)
	c.Check(pins[pgKey], jc.SameContents, []string{"machine/6", "machine/7"})

	// Unpin and check the leases.
	err = s.store.UnpinLease(pgKey, "machine/7", s.stopCh)
	c.Assert(err, jc.ErrorIsNil)

	pins, err = s.store.Pinned()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(pins, gc.HasLen, 1)
	c.Check(pins[pgKey], jc.SameContents, []string{"machine/6"})
}

func (s *storeSuite) TestLeaseOperationCancellation(c *gc.C) {
	s.stopCh <- struct{}{}

	key := corelease.Key{
		Namespace: "application",
		ModelUUID: "model-uuid",
		Lease:     "postgresql",
	}

	req := corelease.Request{
		Holder:   "postgresql/0",
		Duration: time.Minute,
	}

	err := s.store.ClaimLease(key, req, s.stopCh)
	c.Assert(err, gc.ErrorMatches, "context canceled")
}

func (s *storeSuite) primeDB(c *gc.C) {
	tx, err := s.db.Begin()
	c.Assert(err, jc.ErrorIsNil)

	for _, stmt := range schema.ControllerDDL() {
		_, err := tx.Exec(stmt)
		c.Assert(err, jc.ErrorIsNil)
	}

	err = tx.Commit()
	c.Assert(err, jc.ErrorIsNil)
}
