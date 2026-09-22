// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/canonical/gomock/gomock"
	"github.com/canonical/lxd/shared/api"
	"github.com/juju/tc"

	containerlxd "github.com/juju/juju/internal/container/lxd"
	lxdtesting "github.com/juju/juju/internal/container/lxd/testing"
)

type networkForwardSuite struct{}

func TestNetworkForwardSuite(t *testing.T) {
	tc.Run(t, &networkForwardSuite{})
}

func (s *networkForwardSuite) TestNoForwardExtension(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	srv := NewMockServer(ctrl)
	srv.EXPECT().HasExtension("network_forward").Return(false)
	srv.EXPECT().RemoveContainers([]string{"instance0"}).Return(nil)
	err := removeInstances(c.Context(), srv, []string{"instance0"})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *networkForwardSuite) TestNoInstances(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	srv := NewMockServer(ctrl)
	srv.EXPECT().RemoveContainers(nil).Return(nil)
	err := removeInstances(c.Context(), srv, nil)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *networkForwardSuite) TestCancelled(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	srv := NewMockServer(ctrl)
	ctx, cancel := context.WithCancel(c.Context())
	cancel()
	err := removeInstances(ctx, srv, []string{"instance0"})
	c.Assert(err, tc.ErrorIs, context.Canceled)
}

func (s *networkForwardSuite) TestCancelledBeforeInstanceRemoval(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	srv := NewMockServer(ctrl)
	ctx, cancel := context.WithCancel(c.Context())
	defer cancel()
	srv.EXPECT().HasExtension("network_forward").Return(true)
	srv.EXPECT().GetNetworks().DoAndReturn(func() ([]api.Network, error) {
		cancel()
		return []api.Network{{Name: "br0", Type: "bridge"}}, nil
	})
	err := removeInstances(ctx, srv, []string{"instance0"})
	c.Assert(err, tc.ErrorIs, context.Canceled)
}

func (s *networkForwardSuite) TestRemoveOwnedForwardsBeforeInstance(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	srv := NewMockServer(ctrl)
	srv.EXPECT().HasExtension("network_forward").Return(true)
	deleteForward := lxdtesting.NewMockOperation(ctrl)
	gomock.InOrder(
		srv.EXPECT().GetNetworks().Return([]api.Network{
			{Name: "br0", Type: "bridge"}, {Name: "ovn0", Type: "ovn"}, {Name: "ovn1", Type: "ovn"},
		}, nil),
		srv.EXPECT().GetNetworkForwards("ovn0").Return([]api.NetworkForward{
			ownedForward("192.0.2.1", "instance0"),
			ownedForward("192.0.2.2", "instance1"),
			{ListenAddress: "192.0.2.3"},
			ownedForward("192.0.2.4", "instance0"),
		}, nil),
		srv.EXPECT().DeleteNetworkForward("ovn0", "192.0.2.1").Return(deleteForward, nil),
		deleteForward.EXPECT().WaitContext(c.Context()).Return(nil),
		srv.EXPECT().DeleteNetworkForward("ovn0", "192.0.2.4").Return(deleteForward, nil),
		deleteForward.EXPECT().WaitContext(c.Context()).Return(nil),
		srv.EXPECT().GetNetworkForwards("ovn1").Return([]api.NetworkForward{ownedForward("192.0.2.5", "instance0")}, nil),
		srv.EXPECT().DeleteNetworkForward("ovn1", "192.0.2.5").Return(deleteForward, nil),
		deleteForward.EXPECT().WaitContext(c.Context()).Return(nil),
		srv.EXPECT().RemoveContainers([]string{"instance0"}).Return(nil),
	)
	c.Assert(removeInstances(c.Context(), srv, []string{"instance0"}), tc.ErrorIsNil)
}

func (s *networkForwardSuite) TestCleanupAfterInstanceDisappears(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	srv := NewMockServer(ctrl)
	srv.EXPECT().HasExtension("network_forward").Return(true)
	op := lxdtesting.NewMockOperation(ctrl)
	gomock.InOrder(
		srv.EXPECT().GetNetworks().Return([]api.Network{{Name: "ovn0", Type: "ovn"}}, nil),
		srv.EXPECT().GetNetworkForwards("ovn0").Return([]api.NetworkForward{ownedForward("192.0.2.1", "instance0")}, nil),
		srv.EXPECT().DeleteNetworkForward("ovn0", "192.0.2.1").Return(op, nil),
		op.EXPECT().WaitContext(c.Context()).Return(nil),
		srv.EXPECT().RemoveContainers([]string{"instance0"}).Return(nil),
	)
	c.Assert(removeInstances(c.Context(), srv, []string{"instance0"}), tc.ErrorIsNil)
}

func (s *networkForwardSuite) TestForwardAlreadyDeleted(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	srv := NewMockServer(ctrl)
	srv.EXPECT().HasExtension("network_forward").Return(true)
	op := lxdtesting.NewMockOperation(ctrl)
	srv.EXPECT().GetNetworks().Return([]api.Network{{Name: "ovn0", Type: "ovn"}}, nil)
	srv.EXPECT().GetNetworkForwards("ovn0").Return([]api.NetworkForward{
		ownedForward("192.0.2.1", "instance0"), ownedForward("192.0.2.2", "instance0"),
	}, nil)
	srv.EXPECT().DeleteNetworkForward("ovn0", "192.0.2.1").Return(nil, api.StatusErrorf(http.StatusNotFound, "gone"))
	srv.EXPECT().DeleteNetworkForward("ovn0", "192.0.2.2").Return(op, nil)
	op.EXPECT().WaitContext(c.Context()).Return(api.StatusErrorf(http.StatusNotFound, "gone"))
	srv.EXPECT().RemoveContainers([]string{"instance0"}).Return(nil)
	c.Assert(removeInstances(c.Context(), srv, []string{"instance0"}), tc.ErrorIsNil)
}

func (s *networkForwardSuite) TestNetworkDisappears(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	srv := NewMockServer(ctrl)
	srv.EXPECT().HasExtension("network_forward").Return(true)
	srv.EXPECT().GetNetworks().Return([]api.Network{{Name: "ovn0", Type: "ovn"}}, nil)
	srv.EXPECT().GetNetworkForwards("ovn0").Return(nil, api.StatusErrorf(http.StatusNotFound, "gone"))
	srv.EXPECT().RemoveContainers([]string{"instance0"}).Return(nil)
	c.Assert(removeInstances(c.Context(), srv, []string{"instance0"}), tc.ErrorIsNil)
}

func (s *networkForwardSuite) TestNetworkErrorPreservesInstance(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	srv := NewMockServer(ctrl)
	srv.EXPECT().HasExtension("network_forward").Return(true)
	failure := errors.New("networks unavailable")
	srv.EXPECT().GetNetworks().Return(nil, failure)
	err := removeInstances(c.Context(), srv, []string{"instance0"})
	c.Assert(err, tc.ErrorIs, failure)
}

func (s *networkForwardSuite) TestListErrorPreservesInstance(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	srv := NewMockServer(ctrl)
	srv.EXPECT().HasExtension("network_forward").Return(true)
	failure := errors.New("forwards unavailable")
	srv.EXPECT().GetNetworks().Return([]api.Network{{Name: "ovn0", Type: "ovn"}}, nil)
	srv.EXPECT().GetNetworkForwards("ovn0").Return(nil, failure)
	err := removeInstances(c.Context(), srv, []string{"instance0"})
	c.Assert(err, tc.ErrorIs, failure)
}

func (s *networkForwardSuite) TestDeleteErrorPreservesInstance(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	srv := NewMockServer(ctrl)
	srv.EXPECT().HasExtension("network_forward").Return(true)
	failure := errors.New("delete failed")
	srv.EXPECT().GetNetworks().Return([]api.Network{{Name: "ovn0", Type: "ovn"}}, nil)
	srv.EXPECT().GetNetworkForwards("ovn0").Return([]api.NetworkForward{ownedForward("192.0.2.1", "instance0")}, nil)
	srv.EXPECT().DeleteNetworkForward("ovn0", "192.0.2.1").Return(nil, failure)
	err := removeInstances(c.Context(), srv, []string{"instance0"})
	c.Assert(err, tc.ErrorIs, failure)
}

func (s *networkForwardSuite) TestCancelledOperationPreservesInstance(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	srv := NewMockServer(ctrl)
	srv.EXPECT().HasExtension("network_forward").Return(true)
	srv.EXPECT().GetNetworks().Return([]api.Network{{Name: "ovn0", Type: "ovn"}}, nil)
	srv.EXPECT().GetNetworkForwards("ovn0").Return([]api.NetworkForward{ownedForward("192.0.2.1", "instance0")}, nil)
	op := lxdtesting.NewMockOperation(ctrl)
	srv.EXPECT().DeleteNetworkForward("ovn0", "192.0.2.1").Return(op, nil)
	op.EXPECT().WaitContext(c.Context()).Return(context.Canceled)
	err := removeInstances(c.Context(), srv, []string{"instance0"})
	c.Assert(err, tc.ErrorIs, context.Canceled)
}

func (s *networkForwardSuite) TestRetryAfterPartialCleanup(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	srv := NewMockServer(ctrl)
	srv.EXPECT().HasExtension("network_forward").Return(true).Times(2)
	failure := errors.New("delete failed")
	op := lxdtesting.NewMockOperation(ctrl)
	gomock.InOrder(
		srv.EXPECT().GetNetworks().Return([]api.Network{{Name: "ovn0", Type: "ovn"}}, nil),
		srv.EXPECT().GetNetworkForwards("ovn0").Return([]api.NetworkForward{
			ownedForward("192.0.2.1", "instance0"), ownedForward("192.0.2.2", "instance0"),
		}, nil),
		srv.EXPECT().DeleteNetworkForward("ovn0", "192.0.2.1").Return(op, nil),
		op.EXPECT().WaitContext(c.Context()).Return(nil),
		srv.EXPECT().DeleteNetworkForward("ovn0", "192.0.2.2").Return(op, nil),
		op.EXPECT().WaitContext(c.Context()).Return(failure),
		srv.EXPECT().GetNetworks().Return([]api.Network{{Name: "ovn0", Type: "ovn"}}, nil),
		srv.EXPECT().GetNetworkForwards("ovn0").Return([]api.NetworkForward{ownedForward("192.0.2.2", "instance0")}, nil),
		srv.EXPECT().DeleteNetworkForward("ovn0", "192.0.2.2").Return(op, nil),
		op.EXPECT().WaitContext(c.Context()).Return(nil),
		srv.EXPECT().RemoveContainers([]string{"instance0"}).Return(nil),
	)
	err := removeInstances(c.Context(), srv, []string{"instance0"})
	c.Assert(err, tc.ErrorIs, failure)
	c.Assert(removeInstances(c.Context(), srv, []string{"instance0"}), tc.ErrorIsNil)
}

func (s *networkForwardSuite) TestDestroyHostedModelResourcesCleansForwards(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	srv := NewMockServer(ctrl)
	env := &environ{serverUnlocked: srv, uuid: "controller-model"}
	op := lxdtesting.NewMockOperation(ctrl)
	gomock.InOrder(
		srv.EXPECT().AliveContainers("juju-").Return([]containerlxd.Container{
			{Instance: api.Instance{Name: "juju-controller-0", Config: map[string]string{
				"user.juju-model-uuid": "controller-model", "user.juju-controller-uuid": "controller",
			}}},
			{Instance: api.Instance{Name: "juju-hosted-0", Config: map[string]string{
				"user.juju-model-uuid": "hosted-model", "user.juju-controller-uuid": "controller",
			}}},
			{Instance: api.Instance{Name: "juju-other-0", Config: map[string]string{
				"user.juju-model-uuid": "other-model", "user.juju-controller-uuid": "other-controller",
			}}},
		}, nil),
		srv.EXPECT().HasExtension("network_forward").Return(true),
		srv.EXPECT().GetNetworks().Return([]api.Network{{Name: "ovn0", Type: "ovn"}}, nil),
		srv.EXPECT().GetNetworkForwards("ovn0").Return([]api.NetworkForward{
			ownedForward("192.0.2.1", "juju-controller-0"),
			ownedForward("192.0.2.2", "juju-hosted-0"),
			ownedForward("192.0.2.3", "juju-other-0"),
		}, nil),
		srv.EXPECT().DeleteNetworkForward("ovn0", "192.0.2.2").Return(op, nil),
		op.EXPECT().WaitContext(c.Context()).Return(nil),
		srv.EXPECT().RemoveContainers([]string{"juju-hosted-0"}).Return(nil),
	)
	err := env.destroyHostedModelResources(c.Context(), "controller")
	c.Assert(err, tc.ErrorIsNil)
}

func ownedForward(listen, owner string) api.NetworkForward {
	return api.NetworkForward{
		ListenAddress: listen,
		Config: map[string]string{
			"user.juju-instance": owner,
		},
	}
}
