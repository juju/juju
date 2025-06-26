// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package domain

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/juju/tc"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/leadership"
	lease "github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/testhelpers"
)

type leaseServiceSuite struct {
	testhelpers.IsolationSuite

	modelLeaseManager *MockModelLeaseManagerGetter
	leaseManager      *MockLeaseManager
	token             *MockToken
}

func TestLeaseServiceSuite(t *testing.T) {
	defer goleak.VerifyNone(t)
	tc.Run(t, &leaseServiceSuite{})
}

func (s *leaseServiceSuite) TestWithLeader(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Done is triggered when the lease function is done.
	done := make(chan struct{})

	// Force the lease wait to be triggered.
	s.leaseManager.EXPECT().WaitUntilExpired(gomock.Any(), "leaseName", gomock.Any()).DoAndReturn(
		func(ctx context.Context, leaseName string, start chan<- struct{}) error {
			close(start)

			// Don't return until the lease function is done.
			select {
			case <-done:
			case <-time.After(testhelpers.LongWait):
				c.Fatalf("lease function not done")
			}
			return nil
		},
	)

	// Check we correctly hold the lease.
	s.leaseManager.EXPECT().Token("leaseName", "holderName").Return(s.token)
	s.token.EXPECT().Check().Return(nil)

	service := NewLeaseService(s.modelLeaseManager)

	var called bool
	err := service.WithLeader(c.Context(), "leaseName", "holderName", func(ctx context.Context) error {
		defer close(done)
		called = true
		return ctx.Err()
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(called, tc.IsTrue)
}

// TestWithLeaderWaitReturnsError checks that if WaitUntilExpired returns an
// error, the context is always cancelled.
func (s *leaseServiceSuite) TestWithLeaderWaitReturnsError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.leaseManager.EXPECT().WaitUntilExpired(gomock.Any(), "leaseName", gomock.Any()).DoAndReturn(
		func(ctx context.Context, leaseName string, start chan<- struct{}) error {
			return errors.Errorf("not holding lease")
		},
	)

	service := NewLeaseService(s.modelLeaseManager)

	var called bool
	err := service.WithLeader(c.Context(), "leaseName", "holderName", func(ctx context.Context) error {
		called = true
		return ctx.Err()
	})
	c.Assert(err, tc.ErrorIs, context.Canceled)
	c.Check(called, tc.IsFalse)
}

func (s *leaseServiceSuite) TestWithLeaderWaitHasLeaseChange(c *tc.C) {
	defer s.setupMocks(c).Finish()

	done := make(chan struct{})
	running := make(chan struct{})

	// Cause the start to be triggered right away, but ensure that the
	// lease has changed.
	s.leaseManager.EXPECT().WaitUntilExpired(gomock.Any(), "leaseName", gomock.Any()).DoAndReturn(
		func(ctx context.Context, leaseName string, start chan<- struct{}) error {
			close(start)

			select {
			case <-running:
			case <-time.After(testhelpers.LongWait):
				c.Fatalf("lease function not running")
			}

			close(done)

			return errors.Errorf("not holding lease")
		},
	)

	// Check we correctly hold the lease.
	s.leaseManager.EXPECT().Token("leaseName", "holderName").Return(s.token)
	s.token.EXPECT().Check().Return(nil)

	service := NewLeaseService(s.modelLeaseManager)

	// Finish is used to ensure that the lease function has completed and not
	// left running.
	finish := make(chan struct{})
	defer close(finish)

	// The lease function should be a long running function.

	var called bool
	err := service.WithLeader(c.Context(), "leaseName", "holderName", func(ctx context.Context) error {
		called = true

		// Notify to everyone that we're running.
		close(running)

		select {
		case <-done:
		case <-time.After(testhelpers.LongWait):
			c.Fatalf("lease function not done")
		}
		select {
		case <-finish:
		case <-time.After(time.Millisecond * 100):
		}

		return ctx.Err()
	})
	c.Assert(err, tc.ErrorIs, context.Canceled)
	c.Check(called, tc.IsTrue)
}

func (s *leaseServiceSuite) TestWithLeaderFailsOnWaitCheck(c *tc.C) {
	defer s.setupMocks(c).Finish()

	done := make(chan struct{})
	defer close(done)

	// Cause the start to be triggered right away, but ensure that the
	// lease has changed.
	s.leaseManager.EXPECT().WaitUntilExpired(gomock.Any(), "leaseName", gomock.Any()).DoAndReturn(
		func(ctx context.Context, leaseName string, start chan<- struct{}) error {
			close(start)

			select {
			case <-done:
			case <-time.After(testhelpers.LongWait):
			}

			return nil
		},
	)

	// Fail the lease check.
	s.leaseManager.EXPECT().Token("leaseName", "holderName").Return(s.token)
	s.token.EXPECT().Check().Return(errors.Errorf("not holding lease"))

	service := NewLeaseService(s.modelLeaseManager)

	// The lease function should be a long running function.

	var called bool
	err := service.WithLeader(c.Context(), "leaseName", "holderName", func(ctx context.Context) error {
		called = true
		return nil
	})
	c.Assert(err, tc.ErrorMatches, "checking lease token: not holding lease")
	c.Check(called, tc.IsFalse)
}

func (s *leaseServiceSuite) TestRevokeLeadership(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.leaseManager.EXPECT().Revoke("leaseName", "holderName").Return(nil)

	service := NewLeaseService(s.modelLeaseManager)
	err := service.RevokeLeadership("leaseName", unit.Name("holderName"))
	c.Assert(err, tc.ErrorIsNil)
}

func (s *leaseServiceSuite) TestRevokeLeadershipLeaseNotHeld(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.leaseManager.EXPECT().Revoke("leaseName", "holderName").Return(lease.ErrNotHeld)

	service := NewLeaseService(s.modelLeaseManager)
	err := service.RevokeLeadership("leaseName", unit.Name("holderName"))
	c.Assert(err, tc.ErrorIs, leadership.ErrClaimNotHeld)
}

func (s *leaseServiceSuite) TestRevokeLeadershipLeaseError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.leaseManager.EXPECT().Revoke("leaseName", "holderName").Return(fmt.Errorf("down we go"))

	service := NewLeaseService(s.modelLeaseManager)
	err := service.RevokeLeadership("leaseName", unit.Name("holderName"))
	c.Assert(err, tc.ErrorMatches, ".*down we go")
}

func (s *leaseServiceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.modelLeaseManager = NewMockModelLeaseManagerGetter(ctrl)
	s.leaseManager = NewMockLeaseManager(ctrl)
	s.token = NewMockToken(ctrl)

	s.modelLeaseManager.EXPECT().GetLeaseManager().Return(s.leaseManager, nil).AnyTimes()

	return ctrl
}
