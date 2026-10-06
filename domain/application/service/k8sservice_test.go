// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/tc"

	coreapplication "github.com/juju/juju/core/application"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/domain/application/internal"
	"github.com/juju/juju/internal/errors"
)

func (s *applicationServiceSuite) TestClearK8sServiceAddresses(c *tc.C) {
	defer s.setupMocks(c).Finish()
	appUUID := tc.Must0(c, coreapplication.NewUUID)
	for _, stateErr := range []error{nil, errors.New("state failure")} {
		s.state.EXPECT().DeleteK8sServiceAddresses(gomock.Any(), appUUID.String()).Return(stateErr)
		err := s.service.ClearK8sServiceAddresses(c.Context(), appUUID)
		if stateErr == nil {
			c.Assert(err, tc.ErrorIsNil)
		} else {
			c.Assert(err, tc.ErrorIs, stateErr)
		}
	}
}

func (s *applicationServiceSuite) TestClearK8sServiceAddressesInvalidUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()
	c.Assert(s.service.ClearK8sServiceAddresses(c.Context(), "invalid"), tc.NotNil)
}

func (s *applicationServiceSuite) TestUpdateK8sServiceAllocatesUUIDs(c *tc.C) {
	defer s.setupMocks(c).Finish()
	addresses := network.ProviderAddresses{
		network.NewMachineAddress("10.0.0.1", network.WithScope(network.ScopeCloudLocal)).AsProviderAddress(),
		network.NewMachineAddress("lb.example.com", network.WithScope(network.ScopePublic)).AsProviderAddress(),
	}
	s.state.EXPECT().UpsertK8sService(gomock.Any(), "foo", "provider", gomock.Any()).DoAndReturn(
		func(_ context.Context, _, _ string, args internal.UpsertK8sServiceArgs) error {
			c.Check(args.ServiceUUID, tc.IsNonZeroUUID)
			c.Check(args.NetNodeUUID, tc.IsNonZeroUUID)
			c.Check(args.DeviceUUID, tc.IsNonZeroUUID)
			c.Assert(args.Addresses, tc.HasLen, 2)
			for i, addr := range args.Addresses {
				c.Check(addr.UUID, tc.IsNonZeroUUID)
				c.Check(addr.ProviderAddress, tc.DeepEquals, addresses[i])
			}
			return nil
		})
	c.Assert(s.service.UpdateK8sService(c.Context(), "foo", "provider", addresses), tc.ErrorIsNil)
}

func (s *applicationServiceSuite) TestUpdateK8sServiceRejectsHostnameScope(c *tc.C) {
	defer s.setupMocks(c).Finish()
	for _, scope := range []network.Scope{network.ScopeMachineLocal, network.ScopeLinkLocal, ""} {
		err := s.service.UpdateK8sService(c.Context(), "foo", "provider", network.ProviderAddresses{
			network.ProviderAddress{MachineAddress: network.MachineAddress{Value: "lb.example.com", Type: network.HostName, Scope: scope}},
		})
		c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
	}
}
