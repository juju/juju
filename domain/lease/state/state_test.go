// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"context"
	"time"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/database"
	corelease "github.com/juju/juju/core/lease"
	"github.com/juju/juju/database/testing"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/lease/state"
)

type stateSuite struct {
	testing.ControllerSuite

	store *state.State
}

var _ = gc.Suite(&stateSuite{})

func (s *stateSuite) SetUpTest(c *gc.C) {
	s.ControllerSuite.SetUpTest(c)

	s.store = state.NewState(domain.NewTxnRunnerFactory(s.getWatchableDB))
}

func (s *stateSuite) TestClaimLeaseSuccessAndLeaseQueries(c *gc.C) {
	pgKey := corelease.Key{
		Namespace: "application-leadership",
		ModelUUID: "model-uuid",
		Lease:     "postgresql",
	}

	pgReq := corelease.Request{
		Holder:   "postgresql/0",
		Duration: time.Minute,
	}

	// Add 2 leases.
	err := s.store.ClaimLease(context.Background(), utils.MustNewUUID(), pgKey, pgReq)
	c.Assert(err, jc.ErrorIsNil)

	mmKey := pgKey
	mmKey.Lease = "mattermost"

	mmReq := pgReq
	mmReq.Holder = "mattermost/0"

	err = s.store.ClaimLease(context.Background(), utils.MustNewUUID(), mmKey, mmReq)
	c.Assert(err, jc.ErrorIsNil)

	// Check all the leases.
	leases, err := s.store.Leases(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(leases, gc.HasLen, 2)
	c.Check(leases[pgKey].Holder, gc.Equals, "postgresql/0")
	c.Check(leases[pgKey].Expiry.After(time.Now().UTC()), jc.IsTrue)
	c.Check(leases[mmKey].Holder, gc.Equals, "mattermost/0")
	c.Check(leases[mmKey].Expiry.After(time.Now().UTC()), jc.IsTrue)

	// Check with a filter.
	leases, err = s.store.Leases(context.Background(), pgKey)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(leases, gc.HasLen, 1)
	c.Check(leases[pgKey].Holder, gc.Equals, "postgresql/0")

	// Add a lease from a different group,
	// and check that the group returns the application leases.
	err = s.store.ClaimLease(context.Background(),
		utils.MustNewUUID(),
		corelease.Key{
			Namespace: "singular-controller",
			ModelUUID: "controller-model-uuid",
			Lease:     "singular",
		},
		corelease.Request{
			Holder:   "machine/0",
			Duration: time.Minute,
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	leases, err = s.store.LeaseGroup(context.Background(), "application-leadership", "model-uuid")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(leases, gc.HasLen, 2)
	c.Check(leases[pgKey].Holder, gc.Equals, "postgresql/0")
	c.Check(leases[mmKey].Holder, gc.Equals, "mattermost/0")
}

func (s *stateSuite) TestClaimLeaseAlreadyHeld(c *gc.C) {
	key := corelease.Key{
		Namespace: "singular-controller",
		ModelUUID: "controller-model-uuid",
		Lease:     "singular",
	}

	req := corelease.Request{
		Holder:   "machine/0",
		Duration: time.Minute,
	}

	err := s.store.ClaimLease(context.Background(), utils.MustNewUUID(), key, req)
	c.Assert(err, jc.ErrorIsNil)

	err = s.store.ClaimLease(context.Background(), utils.MustNewUUID(), key, req)
	c.Assert(errors.Is(err, corelease.ErrHeld), jc.IsTrue)
}

func (s *stateSuite) TestExtendLeaseSuccess(c *gc.C) {
	key := corelease.Key{
		Namespace: "application-leadership",
		ModelUUID: "model-uuid",
		Lease:     "postgresql",
	}

	req := corelease.Request{
		Holder:   "postgresql/0",
		Duration: time.Minute,
	}

	err := s.store.ClaimLease(context.Background(), utils.MustNewUUID(), key, req)
	c.Assert(err, jc.ErrorIsNil)

	leases, err := s.store.Leases(context.Background(), key)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(leases, gc.HasLen, 1)

	// Save the expiry for later comparison.
	originalExpiry := leases[key].Expiry

	req.Duration = 2 * time.Minute
	err = s.store.ExtendLease(context.Background(), key, req)
	c.Assert(err, jc.ErrorIsNil)

	leases, err = s.store.Leases(context.Background(), key)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(leases, gc.HasLen, 1)

	// Check that we extended.
	c.Check(leases[key].Expiry.After(originalExpiry), jc.IsTrue)
}

func (s *stateSuite) TestExtendLeaseNotHeldInvalid(c *gc.C) {
	key := corelease.Key{
		Namespace: "application-leadership",
		ModelUUID: "model-uuid",
		Lease:     "postgresql",
	}

	req := corelease.Request{
		Holder:   "postgresql/0",
		Duration: time.Minute,
	}

	err := s.store.ExtendLease(context.Background(), key, req)
	c.Assert(errors.Is(err, corelease.ErrInvalid), jc.IsTrue)
}

func (s *stateSuite) TestRevokeLeaseSuccess(c *gc.C) {
	key := corelease.Key{
		Namespace: "application-leadership",
		ModelUUID: "model-uuid",
		Lease:     "postgresql",
	}

	req := corelease.Request{
		Holder:   "postgresql/0",
		Duration: time.Minute,
	}

	err := s.store.ClaimLease(context.Background(), utils.MustNewUUID(), key, req)
	c.Assert(err, jc.ErrorIsNil)

	err = s.store.RevokeLease(context.Background(), key, req.Holder)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *stateSuite) TestRevokeLeaseNotHeldInvalid(c *gc.C) {
	key := corelease.Key{
		Namespace: "application-leadership",
		ModelUUID: "model-uuid",
		Lease:     "postgresql",
	}

	err := s.store.RevokeLease(context.Background(), key, "not-the-holder")
	c.Assert(errors.Is(err, corelease.ErrInvalid), jc.IsTrue)
}

func (s *stateSuite) TestPinUnpinLeaseAndPinQueries(c *gc.C) {
	pgKey := corelease.Key{
		Namespace: "application-leadership",
		ModelUUID: "model-uuid",
		Lease:     "postgresql",
	}

	pgReq := corelease.Request{
		Holder:   "postgresql/0",
		Duration: time.Minute,
	}

	err := s.store.ClaimLease(context.Background(), utils.MustNewUUID(), pgKey, pgReq)
	c.Assert(err, jc.ErrorIsNil)

	// One entity pins the lease.
	err = s.store.PinLease(context.Background(), pgKey, "machine/6")
	c.Assert(err, jc.ErrorIsNil)

	// The same lease/entity is a no-op without error.
	err = s.store.PinLease(context.Background(), pgKey, "machine/6")
	c.Assert(err, jc.ErrorIsNil)

	// Another entity pinning the same lease.
	err = s.store.PinLease(context.Background(), pgKey, "machine/7")
	c.Assert(err, jc.ErrorIsNil)

	pins, err := s.store.Pinned(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(pins, gc.HasLen, 1)
	c.Check(pins[pgKey], jc.SameContents, []string{"machine/6", "machine/7"})

	// Unpin and check the leases.
	err = s.store.UnpinLease(context.Background(), pgKey, "machine/7")
	c.Assert(err, jc.ErrorIsNil)

	pins, err = s.store.Pinned(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(pins, gc.HasLen, 1)
	c.Check(pins[pgKey], jc.SameContents, []string{"machine/6"})
}

func (s *stateSuite) TestLeaseOperationCancellation(c *gc.C) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	key := corelease.Key{
		Namespace: "application-leadership",
		ModelUUID: "model-uuid",
		Lease:     "postgresql",
	}

	req := corelease.Request{
		Holder:   "postgresql/0",
		Duration: time.Minute,
	}

	err := s.store.ClaimLease(ctx, utils.MustNewUUID(), key, req)
	c.Assert(err, gc.ErrorMatches, "context canceled")
}

func (s *stateSuite) getWatchableDB() (changestream.WatchableDB, error) {
	return &stubWatchableDB{TxnRunner: s.TxnRunner()}, nil
}

func (s *stateSuite) getWatchableDBForNameSpace(_ string) (changestream.WatchableDB, error) {
	return &stubWatchableDB{TxnRunner: s.TxnRunner()}, nil
}

type stubWatchableDB struct {
	database.TxnRunner
	changestream.EventSource
}
