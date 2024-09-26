// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package domain

import (
	"context"
	"fmt"
	"time"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/errors"
)

type leaseServiceSuite struct {
	testing.IsolationSuite

	leaseChecker *MockLeaseChecker
	token        *MockToken
}

var _ = gc.Suite(&leaseServiceSuite{})

func (s *leaseServiceSuite) TestWithLease(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Done is triggered when the lease function is done.
	done := make(chan struct{})

	// Check we already hold the lease.
	s.leaseChecker.EXPECT().Token("leaseName", "holderName").Return(s.token)
	s.token.EXPECT().Check().Return(nil)

	// Force the lease wait to be triggered.
	s.leaseChecker.EXPECT().WaitUntilExpired(gomock.Any(), "leaseName", gomock.Any()).DoAndReturn(func(ctx context.Context, leaseName string, start chan<- struct{}) error {
		close(start)

		// Don't return until the lease function is done.
		select {
		case <-done:
		case <-time.After(testing.LongWait):
			c.Fatalf("lease function not done")
		}
		return nil
	})

	// Now check that the lease is still held once we've started waiting. If
	// the wait is triggered before the check, the test will fail.
	s.leaseChecker.EXPECT().Token("leaseName", "holderName").Return(s.token)
	s.token.EXPECT().Check().Return(nil)

	service := LeaseService{
		leaseChecker: func() LeaseChecker {
			return s.leaseChecker
		},
	}

	var called bool
	err := service.WithLease(context.Background(), "leaseName", "holderName", func(ctx context.Context) error {
		defer close(done)
		called = true
		return ctx.Err()
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(called, jc.IsTrue)
}

func (s *leaseServiceSuite) TestWithLeaseWaitReturnsError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Check we already hold the lease.
	s.leaseChecker.EXPECT().Token("leaseName", "holderName").Return(s.token)
	s.token.EXPECT().Check().Return(nil)

	s.leaseChecker.EXPECT().WaitUntilExpired(gomock.Any(), "leaseName", gomock.Any()).DoAndReturn(func(ctx context.Context, leaseName string, start chan<- struct{}) error {
		return fmt.Errorf("not holding lease")
	})

	service := LeaseService{
		leaseChecker: func() LeaseChecker {
			return s.leaseChecker
		},
	}

	var called bool
	err := service.WithLease(context.Background(), "leaseName", "holderName", func(ctx context.Context) error {
		called = true
		return ctx.Err()
	})
	c.Assert(err, gc.ErrorMatches, "not holding lease")
	c.Check(called, jc.IsFalse)
}

func (s *leaseServiceSuite) TestWithLeaseWaitHasLeaseChange(c *gc.C) {
	defer s.setupMocks(c).Finish()

	done := make(chan struct{})
	running := make(chan struct{})

	// Check we already hold the lease.
	s.leaseChecker.EXPECT().Token("leaseName", "holderName").Return(s.token)
	s.token.EXPECT().Check().Return(nil)

	// Cause the start to be triggered right away, but ensure that the
	// lease has changed.
	s.leaseChecker.EXPECT().WaitUntilExpired(gomock.Any(), "leaseName", gomock.Any()).DoAndReturn(func(ctx context.Context, leaseName string, start chan<- struct{}) error {
		close(start)

		select {
		case <-running:
		case <-time.After(testing.LongWait):
			c.Fatalf("lease function not running")
		}

		close(done)

		return fmt.Errorf("not holding lease")
	})

	// The lease is still held by the holder.
	s.leaseChecker.EXPECT().Token("leaseName", "holderName").Return(s.token)
	s.token.EXPECT().Check().Return(nil)

	service := LeaseService{
		leaseChecker: func() LeaseChecker {
			return s.leaseChecker
		},
	}

	// Finish is used to ensure that the lease function has completed and not
	// left running.
	finish := make(chan struct{})
	defer close(finish)

	// The lease function should be a long running function.

	var called bool
	err := service.WithLease(context.Background(), "leaseName", "holderName", func(ctx context.Context) error {
		called = true

		// Notify to everyone that we're running.
		close(running)

		select {
		case <-done:
		case <-time.After(testing.LongWait):
			c.Fatalf("lease function not done")
		}
		select {
		case <-finish:
		case <-time.After(time.Millisecond * 100):
		}

		return ctx.Err()
	})
	c.Assert(err, gc.ErrorMatches, "not holding lease")
	c.Check(called, jc.IsTrue)
}

func (s *leaseServiceSuite) TestWithLeaseFailsOnWaitCheck(c *gc.C) {
	defer s.setupMocks(c).Finish()

	done := make(chan struct{})
	defer close(done)

	// Check we already hold the lease.
	s.leaseChecker.EXPECT().Token("leaseName", "holderName").Return(s.token)
	s.token.EXPECT().Check().Return(nil)

	// Cause the start to be triggered right away, but ensure that the
	// lease has changed.
	s.leaseChecker.EXPECT().WaitUntilExpired(gomock.Any(), "leaseName", gomock.Any()).DoAndReturn(func(ctx context.Context, leaseName string, start chan<- struct{}) error {
		close(start)

		select {
		case <-done:
		case <-time.After(testing.LongWait):
		}

		return nil
	})

	// The lease is still held by the holder.
	s.leaseChecker.EXPECT().Token("leaseName", "holderName").Return(s.token)
	s.token.EXPECT().Check().Return(errors.Errorf("not holding lease"))

	service := LeaseService{
		leaseChecker: func() LeaseChecker {
			return s.leaseChecker
		},
	}

	// The lease function should be a long running function.

	var called bool
	err := service.WithLease(context.Background(), "leaseName", "holderName", func(ctx context.Context) error {
		called = true
		return nil
	})
	c.Assert(err, gc.ErrorMatches, "not holding lease")
	c.Check(called, jc.IsFalse)
}

func (s *leaseServiceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.leaseChecker = NewMockLeaseChecker(ctrl)
	s.token = NewMockToken(ctrl)

	return ctrl
}
