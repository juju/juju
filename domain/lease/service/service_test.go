// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"time"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/uuid"
)

type serviceSuite struct {
	testhelpers.IsolationSuite

	state *MockState
}

var _ = tc.Suite(&serviceSuite{})

func (s *serviceSuite) TestLeases(c *tc.C) {
	defer s.setupMocks(c).Finish()

	expected := map[lease.Key]lease.Info{
		fixedKey(): {
			Holder: "postgresql/0",
		},
	}

	s.state.EXPECT().Leases(gomock.Any()).Return(expected, nil)

	service := NewService(s.state)
	val, err := service.Leases(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(val, tc.DeepEquals, expected)
}

func (s *serviceSuite) TestLeasesWithKey(c *tc.C) {
	defer s.setupMocks(c).Finish()

	key := fixedKey()
	expected := map[lease.Key]lease.Info{
		key: {
			Holder: "postgresql/0",
		},
	}

	s.state.EXPECT().Leases(gomock.Any(), key).Return(expected, nil)

	service := NewService(s.state)
	val, err := service.Leases(c.Context(), key)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(val, tc.DeepEquals, expected)
}

func (s *serviceSuite) TestLeasesWithMultipleKeys(c *tc.C) {
	defer s.setupMocks(c).Finish()

	service := NewService(s.state)
	_, err := service.Leases(c.Context(), fixedKey(), fixedKey())
	c.Assert(err, tc.ErrorMatches, "filtering with more than one lease key not supported")
}

func (s *serviceSuite) TestClaimLease(c *tc.C) {
	defer s.setupMocks(c).Finish()

	key, req := fixedKey(), lease.Request{
		Holder:   "postgresql/0",
		Duration: time.Minute,
	}

	s.state.EXPECT().ClaimLease(gomock.Any(), gomock.AssignableToTypeOf(uuid.UUID{}), key, req).Return(nil)

	service := NewService(s.state)
	err := service.ClaimLease(c.Context(), key, req)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestClaimLeaseValidation(c *tc.C) {
	defer s.setupMocks(c).Finish()

	service := NewService(s.state)
	err := service.ClaimLease(c.Context(), fixedKey(), lease.Request{})
	c.Assert(err, tc.ErrorMatches, "invalid holder: string is empty")
}

func (s *serviceSuite) TestExtendLease(c *tc.C) {
	defer s.setupMocks(c).Finish()

	key, req := fixedKey(), lease.Request{
		Holder:   "postgresql/0",
		Duration: time.Minute,
	}

	s.state.EXPECT().ExtendLease(gomock.Any(), key, req).Return(nil)

	service := NewService(s.state)
	err := service.ExtendLease(c.Context(), key, req)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestExtendLeaseValidation(c *tc.C) {
	defer s.setupMocks(c).Finish()

	service := NewService(s.state)
	err := service.ClaimLease(c.Context(), fixedKey(), lease.Request{})
	c.Assert(err, tc.ErrorMatches, "invalid holder: string is empty")
}

func (s *serviceSuite) TestRevokeLease(c *tc.C) {
	defer s.setupMocks(c).Finish()

	key := fixedKey()

	s.state.EXPECT().RevokeLease(gomock.Any(), key, "postgresql/0").Return(nil)

	service := NewService(s.state)
	err := service.RevokeLease(c.Context(), key, "postgresql/0")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestLeaseGroup(c *tc.C) {
	defer s.setupMocks(c).Finish()

	key := fixedKey()
	expected := map[lease.Key]lease.Info{
		key: {
			Holder: "postgresql/0",
		},
	}

	s.state.EXPECT().LeaseGroup(gomock.Any(), "foo", "123").Return(expected, nil)

	service := NewService(s.state)
	got, err := service.LeaseGroup(c.Context(), "foo", "123")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(got, tc.DeepEquals, expected)
}

func (s *serviceSuite) TestPinLease(c *tc.C) {
	defer s.setupMocks(c).Finish()

	key := fixedKey()

	s.state.EXPECT().PinLease(gomock.Any(), key, "machine/6").Return(nil)

	service := NewService(s.state)
	err := service.PinLease(c.Context(), key, "machine/6")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestUnpinLease(c *tc.C) {
	defer s.setupMocks(c).Finish()

	key := fixedKey()

	s.state.EXPECT().UnpinLease(gomock.Any(), key, "machine/6").Return(nil)

	service := NewService(s.state)
	err := service.UnpinLease(c.Context(), key, "machine/6")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestPinned(c *tc.C) {
	defer s.setupMocks(c).Finish()

	key := fixedKey()
	expected := map[lease.Key][]string{
		key: {"machine/6"},
	}

	s.state.EXPECT().Pinned(gomock.Any()).Return(expected, nil)

	service := NewService(s.state)
	got, err := service.Pinned(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(got, tc.DeepEquals, expected)
}

func (s *serviceSuite) TestExpireLeases(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().ExpireLeases(gomock.Any()).Return(nil)

	service := NewService(s.state)
	err := service.ExpireLeases(c.Context())
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockState(ctrl)

	return ctrl
}

func fixedKey() lease.Key {
	return lease.Key{
		Namespace: "application-leadership",
		ModelUUID: "model-uuid",
		Lease:     "postgresql",
	}
}
