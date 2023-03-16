// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease_test

import (
	"context"
	"time"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	corelease "github.com/juju/juju/core/lease"
	"github.com/juju/juju/database/testing"
	"github.com/juju/juju/worker/lease"
)

type storeSuite struct {
	testing.ControllerSuite

	store *lease.Store
}

var _ = gc.Suite(&storeSuite{})

func (s *storeSuite) SetUpTest(c *gc.C) {
	s.ControllerSuite.SetUpTest(c)

	s.store = lease.NewStore(lease.StoreConfig{
		TrackedDB: s.TrackedDB(),
		Logger:    lease.StubLogger{},
	})
}

func (s *storeSuite) TestClaimLeaseSuccessAndLeaseQueries(c *gc.C) {
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
	err := s.store.ClaimLease(context.Background(), pgKey, pgReq)
	c.Assert(err, jc.ErrorIsNil)

	mmKey := pgKey
	mmKey.Lease = "mattermost"

	mmReq := pgReq
	mmReq.Holder = "mattermost/0"

	err = s.store.ClaimLease(context.Background(), mmKey, mmReq)
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

func (s *storeSuite) TestClaimLeaseAlreadyHeld(c *gc.C) {
	key := corelease.Key{
		Namespace: "singular-controller",
		ModelUUID: "controller-model-uuid",
		Lease:     "singular",
	}

	req := corelease.Request{
		Holder:   "machine/0",
		Duration: time.Minute,
	}

	err := s.store.ClaimLease(context.Background(), key, req)
	c.Assert(err, jc.ErrorIsNil)

	err = s.store.ClaimLease(context.Background(), key, req)
	c.Assert(errors.Is(err, corelease.ErrHeld), jc.IsTrue)
}

func (s *storeSuite) TestExtendLeaseSuccess(c *gc.C) {
	key := corelease.Key{
		Namespace: "application-leadership",
		ModelUUID: "model-uuid",
		Lease:     "postgresql",
	}

	req := corelease.Request{
		Holder:   "postgresql/0",
		Duration: time.Minute,
	}

	err := s.store.ClaimLease(context.Background(), key, req)
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

func (s *storeSuite) TestExtendLeaseNotHeldInvalid(c *gc.C) {
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

func (s *storeSuite) TestRevokeLeaseSuccess(c *gc.C) {
	key := corelease.Key{
		Namespace: "application-leadership",
		ModelUUID: "model-uuid",
		Lease:     "postgresql",
	}

	req := corelease.Request{
		Holder:   "postgresql/0",
		Duration: time.Minute,
	}

	err := s.store.ClaimLease(context.Background(), key, req)
	c.Assert(err, jc.ErrorIsNil)

	err = s.store.RevokeLease(context.Background(), key, req.Holder)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *storeSuite) TestRevokeLeaseNotHeldInvalid(c *gc.C) {
	key := corelease.Key{
		Namespace: "application-leadership",
		ModelUUID: "model-uuid",
		Lease:     "postgresql",
	}

	err := s.store.RevokeLease(context.Background(), key, "not-the-holder")
	c.Assert(errors.Is(err, corelease.ErrInvalid), jc.IsTrue)
}

func (s *storeSuite) TestPinUnpinLeaseAndPinQueries(c *gc.C) {
	pgKey := corelease.Key{
		Namespace: "application-leadership",
		ModelUUID: "model-uuid",
		Lease:     "postgresql",
	}

	pgReq := corelease.Request{
		Holder:   "postgresql/0",
		Duration: time.Minute,
	}

	err := s.store.ClaimLease(context.Background(), pgKey, pgReq)
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

func (s *storeSuite) TestLeaseOperationCancellation(c *gc.C) {
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

	err := s.store.ClaimLease(ctx, key, req)
	c.Assert(err, gc.ErrorMatches, "context canceled")
}
