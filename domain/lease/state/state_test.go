// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"context"
	"testing"
	"time"

	"github.com/juju/tc"

	corelease "github.com/juju/juju/core/lease"
	"github.com/juju/juju/domain/lease/state"
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/internal/uuid"
)

type stateSuite struct {
	schematesting.ControllerSuite

	store *state.State
}

func TestStateSuite(t *testing.T) {
	tc.Run(t, &stateSuite{})
}

func (s *stateSuite) SetUpTest(c *tc.C) {
	s.ControllerSuite.SetUpTest(c)

	s.store = state.NewState(s.TxnRunnerFactory())
}

func (s *stateSuite) TestClaimLeaseSuccessAndLeaseQueries(c *tc.C) {
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
	err := s.store.ClaimLease(c.Context(), uuid.MustNewUUID(), pgKey, pgReq)
	c.Assert(err, tc.ErrorIsNil)

	mmKey := pgKey
	mmKey.Lease = "mattermost"

	mmReq := pgReq
	mmReq.Holder = "mattermost/0"

	err = s.store.ClaimLease(c.Context(), uuid.MustNewUUID(), mmKey, mmReq)
	c.Assert(err, tc.ErrorIsNil)

	// Check all the leases.
	leases, err := s.store.Leases(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(leases, tc.HasLen, 2)
	c.Check(leases[pgKey].Holder, tc.Equals, "postgresql/0")
	c.Check(leases[pgKey].Expiry.After(time.Now().UTC()), tc.IsTrue)
	c.Check(leases[mmKey].Holder, tc.Equals, "mattermost/0")
	c.Check(leases[mmKey].Expiry.After(time.Now().UTC()), tc.IsTrue)

	// Check with a filter.
	leases, err = s.store.Leases(c.Context(), pgKey)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(leases, tc.HasLen, 1)
	c.Check(leases[pgKey].Holder, tc.Equals, "postgresql/0")

	// Add a lease from a different group,
	// and check that the group returns the application leases.
	err = s.store.ClaimLease(c.Context(),
		uuid.MustNewUUID(),
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
	c.Assert(err, tc.ErrorIsNil)

	leases, err = s.store.LeaseGroup(c.Context(), "application-leadership", "model-uuid")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(leases, tc.HasLen, 2)
	c.Check(leases[pgKey].Holder, tc.Equals, "postgresql/0")
	c.Check(leases[mmKey].Holder, tc.Equals, "mattermost/0")
}

func (s *stateSuite) TestClaimLeaseAlreadyHeld(c *tc.C) {
	key := corelease.Key{
		Namespace: "singular-controller",
		ModelUUID: "controller-model-uuid",
		Lease:     "singular",
	}

	req := corelease.Request{
		Holder:   "machine/0",
		Duration: time.Minute,
	}

	err := s.store.ClaimLease(c.Context(), uuid.MustNewUUID(), key, req)
	c.Assert(err, tc.ErrorIsNil)

	err = s.store.ClaimLease(c.Context(), uuid.MustNewUUID(), key, req)
	c.Assert(err, tc.ErrorIs, corelease.ErrHeld)
}

func (s *stateSuite) TestExtendLeaseSuccess(c *tc.C) {
	key := corelease.Key{
		Namespace: "application-leadership",
		ModelUUID: "model-uuid",
		Lease:     "postgresql",
	}

	req := corelease.Request{
		Holder:   "postgresql/0",
		Duration: time.Minute,
	}

	err := s.store.ClaimLease(c.Context(), uuid.MustNewUUID(), key, req)
	c.Assert(err, tc.ErrorIsNil)

	leases, err := s.store.Leases(c.Context(), key)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(leases, tc.HasLen, 1)

	// Save the expiry for later comparison.
	originalExpiry := leases[key].Expiry

	req.Duration = 2 * time.Minute
	err = s.store.ExtendLease(c.Context(), key, req)
	c.Assert(err, tc.ErrorIsNil)

	leases, err = s.store.Leases(c.Context(), key)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(leases, tc.HasLen, 1)

	// Check that we extended.
	c.Check(leases[key].Expiry.After(originalExpiry), tc.IsTrue)
}

func (s *stateSuite) TestExtendLeaseNotHeldInvalid(c *tc.C) {
	key := corelease.Key{
		Namespace: "application-leadership",
		ModelUUID: "model-uuid",
		Lease:     "postgresql",
	}

	req := corelease.Request{
		Holder:   "postgresql/0",
		Duration: time.Minute,
	}

	err := s.store.ExtendLease(c.Context(), key, req)
	c.Assert(err, tc.ErrorIs, corelease.ErrInvalid)
}

func (s *stateSuite) TestRevokeLeaseSuccess(c *tc.C) {
	key := corelease.Key{
		Namespace: "application-leadership",
		ModelUUID: "model-uuid",
		Lease:     "postgresql",
	}

	req := corelease.Request{
		Holder:   "postgresql/0",
		Duration: time.Minute,
	}

	err := s.store.ClaimLease(c.Context(), uuid.MustNewUUID(), key, req)
	c.Assert(err, tc.ErrorIsNil)

	err = s.store.RevokeLease(c.Context(), key, req.Holder)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *stateSuite) TestRevokeLeaseNotHeldInvalid(c *tc.C) {
	key := corelease.Key{
		Namespace: "application-leadership",
		ModelUUID: "model-uuid",
		Lease:     "postgresql",
	}

	err := s.store.RevokeLease(c.Context(), key, "not-the-holder")
	c.Assert(err, tc.ErrorIs, corelease.ErrInvalid)
}

func (s *stateSuite) TestPinUnpinLeaseAndPinQueries(c *tc.C) {
	pgKey := corelease.Key{
		Namespace: "application-leadership",
		ModelUUID: "model-uuid",
		Lease:     "postgresql",
	}

	pgReq := corelease.Request{
		Holder:   "postgresql/0",
		Duration: time.Minute,
	}

	err := s.store.ClaimLease(c.Context(), uuid.MustNewUUID(), pgKey, pgReq)
	c.Assert(err, tc.ErrorIsNil)

	// One entity pins the lease.
	err = s.store.PinLease(c.Context(), pgKey, "machine/6")
	c.Assert(err, tc.ErrorIsNil)

	// The same lease/entity is a no-op without error.
	err = s.store.PinLease(c.Context(), pgKey, "machine/6")
	c.Assert(err, tc.ErrorIsNil)

	// Another entity pinning the same lease.
	err = s.store.PinLease(c.Context(), pgKey, "machine/7")
	c.Assert(err, tc.ErrorIsNil)

	pins, err := s.store.Pinned(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(pins, tc.HasLen, 1)
	c.Check(pins[pgKey], tc.SameContents, []string{"machine/6", "machine/7"})

	// Unpin and check the leases.
	err = s.store.UnpinLease(c.Context(), pgKey, "machine/7")
	c.Assert(err, tc.ErrorIsNil)

	pins, err = s.store.Pinned(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(pins, tc.HasLen, 1)
	c.Check(pins[pgKey], tc.SameContents, []string{"machine/6"})
}

func (s *stateSuite) TestLeaseOperationCancellation(c *tc.C) {
	ctx, cancel := context.WithCancel(c.Context())
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

	err := s.store.ClaimLease(ctx, uuid.MustNewUUID(), key, req)
	c.Assert(err, tc.ErrorMatches, "context canceled")
}
