// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease_test

import (
	"database/sql"
	"time"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	_ "github.com/mattn/go-sqlite3"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/database/schema"
)

type storeDBSuite struct {
	testing.IsolationSuite

	db     *sql.DB
	store  *lease.DBStore
	stopCh chan struct{}
}

var _ = gc.Suite(&storeDBSuite{})

func (s *storeDBSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	var err error
	s.db, err = sql.Open("sqlite3", ":memory:")
	c.Assert(err, jc.ErrorIsNil)

	s.primeDB(c)
	s.store = lease.NewDBStore(s.db, lease.StubLogger{})
	s.stopCh = make(chan struct{})
}

func (s *storeDBSuite) TestClaimLeaseSuccessAndLeaseListings(c *gc.C) {
	pgKey := lease.Key{
		Namespace: "application",
		ModelUUID: "model-uuid",
		Lease:     "postgresql",
	}

	pgReq := lease.Request{
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
		lease.Key{
			Namespace: "controller",
			ModelUUID: "controller-model-uuid",
			Lease:     "singular",
		},
		lease.Request{
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
	c.Check(leases[pgKey].Expiry.After(time.Now().UTC()), jc.IsTrue)
	c.Check(leases[mmKey].Holder, gc.Equals, "mattermost/0")
	c.Check(leases[mmKey].Expiry.After(time.Now().UTC()), jc.IsTrue)
}

func (s *storeDBSuite) TestClaimLeaseAlreadyHeld(c *gc.C) {
	key := lease.Key{
		Namespace: "controller",
		ModelUUID: "controller-model-uuid",
		Lease:     "singular",
	}

	req := lease.Request{
		Holder:   "machine/0",
		Duration: time.Minute,
	}

	err := s.store.ClaimLease(key, req, s.stopCh)
	c.Assert(err, jc.ErrorIsNil)

	err = s.store.ClaimLease(key, req, s.stopCh)
	// TODO (manadart 2022-12-01): Check for the right type; ErrHeld?
	c.Assert(err, gc.NotNil)
}

func (s *storeDBSuite) primeDB(c *gc.C) {
	tx, err := s.db.Begin()
	c.Assert(err, jc.ErrorIsNil)

	for _, stmt := range schema.ControllerDDL() {
		_, err := tx.Exec(stmt)
		c.Assert(err, jc.ErrorIsNil)
	}

	err = tx.Commit()
	c.Assert(err, jc.ErrorIsNil)
}
