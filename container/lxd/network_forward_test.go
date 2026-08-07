// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd_test

import (
	"errors"

	lxdapi "github.com/canonical/lxd/shared/api"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/container/lxd"
	lxdtesting "github.com/juju/juju/container/lxd/testing"
)

type networkForwardSuite struct {
	lxdtesting.BaseSuite
}

var _ = gc.Suite(&networkForwardSuite{})

func (s *networkForwardSuite) TestEnsureControllerNetworkForwardCreates(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	client := s.NewMockServer(ctrl)
	forwardPut := lxdapi.NetworkForwardPut{
		Description: "Juju controller controller-uuid instance instance-id",
		Config: map[string]string{
			"user.juju-controller-uuid": "controller-uuid",
			"user.juju-instance-id":     "instance-id",
		},
		Ports: []lxdapi.NetworkForwardPort{{
			Protocol:      "tcp",
			ListenPort:    "22,17070",
			TargetAddress: "10.241.0.2",
		}},
	}
	allocated := lxdapi.NetworkForward{
		ListenAddress: "10.19.2.22",
		Description:   forwardPut.Description,
		Config:        forwardPut.Config,
		Ports:         forwardPut.Ports,
	}
	createOp := lxdtesting.NewMockOperation(ctrl)

	gomock.InOrder(
		client.EXPECT().GetNetworkForwards("network-name").Return(nil, nil),
		client.EXPECT().CreateNetworkForward("network-name", lxdapi.NetworkForwardsPost{
			ListenAddress:     "0.0.0.0",
			NetworkForwardPut: forwardPut,
		}).Return(createOp, nil),
		createOp.EXPECT().Wait().Return(nil),
		client.EXPECT().GetNetworkForwards("network-name").Return([]lxdapi.NetworkForward{allocated}, nil),
		client.EXPECT().GetNetworkForward("network-name", "10.19.2.22").Return(&allocated, lxdtesting.ETag, nil),
	)

	server, err := lxd.NewServer(client)
	c.Assert(err, jc.ErrorIsNil)

	address, err := server.EnsureControllerNetworkForward(
		"network-name", "controller-uuid", "instance-id", "10.241.0.2",
		[]int{17070, 22, 17070},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(address, gc.Equals, "10.19.2.22")
}

func (s *networkForwardSuite) TestEnsureControllerNetworkForwardUpdates(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	client := s.NewMockServer(ctrl)
	existing := lxdapi.NetworkForward{
		ListenAddress: "10.19.2.22",
		Config: map[string]string{
			"user.juju-controller-uuid": "controller-uuid",
			"user.juju-instance-id":     "instance-id",
		},
	}
	expected := lxdapi.NetworkForwardPut{
		Description: "Juju controller controller-uuid instance instance-id",
		Config: map[string]string{
			"user.juju-controller-uuid": "controller-uuid",
			"user.juju-instance-id":     "instance-id",
		},
		Ports: []lxdapi.NetworkForwardPort{{
			Protocol:      "tcp",
			ListenPort:    "22,17070",
			TargetAddress: "10.241.0.3",
		}},
	}
	updateOp := lxdtesting.NewMockOperation(ctrl)

	gomock.InOrder(
		client.EXPECT().GetNetworkForwards("network-name").Return([]lxdapi.NetworkForward{existing}, nil),
		client.EXPECT().GetNetworkForward("network-name", "10.19.2.22").Return(&existing, lxdtesting.ETag, nil),
		client.EXPECT().UpdateNetworkForward("network-name", "10.19.2.22", expected, lxdtesting.ETag).Return(updateOp, nil),
		updateOp.EXPECT().Wait().Return(nil),
	)

	server, err := lxd.NewServer(client)
	c.Assert(err, jc.ErrorIsNil)

	address, err := server.EnsureControllerNetworkForward(
		"network-name", "controller-uuid", "instance-id", "10.241.0.3",
		[]int{22, 17070},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(address, gc.Equals, "10.19.2.22")
}

func (s *networkForwardSuite) TestControllerNetworkForwardAddressNotFound(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	client := s.NewMockServer(ctrl)
	client.EXPECT().GetNetworkForwards("network-name").Return(nil, nil)

	server, err := lxd.NewServer(client)
	c.Assert(err, jc.ErrorIsNil)

	address, found, err := server.ControllerNetworkForwardAddress(
		"network-name", "controller-uuid", "instance-id",
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(found, jc.IsFalse)
	c.Check(address, gc.Equals, "")
}

func (s *networkForwardSuite) TestDeleteControllerNetworkForwards(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	client := s.NewMockServer(ctrl)
	forwards := []lxdapi.NetworkForward{{
		ListenAddress: "10.19.2.22",
		Config: map[string]string{
			"user.juju-controller-uuid": "controller-uuid",
			"user.juju-instance-id":     "instance-id",
		},
	}, {
		ListenAddress: "10.19.2.23",
		Config: map[string]string{
			"user.juju-controller-uuid": "other-controller",
			"user.juju-instance-id":     "other-instance",
		},
	}}
	deleteOp := lxdtesting.NewMockOperation(ctrl)

	client.EXPECT().GetNetworkForwards("network-name").Return(forwards, nil)
	client.EXPECT().DeleteNetworkForward("network-name", "10.19.2.22").Return(deleteOp, nil)
	deleteOp.EXPECT().Wait().Return(nil)

	server, err := lxd.NewServer(client)
	c.Assert(err, jc.ErrorIsNil)

	err = server.DeleteControllerNetworkForwards("network-name", "controller-uuid", "")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *networkForwardSuite) TestDeleteControllerNetworkForwardsReportsErrors(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	client := s.NewMockServer(ctrl)
	forward := lxdapi.NetworkForward{
		ListenAddress: "10.19.2.22",
		Config: map[string]string{
			"user.juju-controller-uuid": "controller-uuid",
			"user.juju-instance-id":     "instance-id",
		},
	}

	client.EXPECT().GetNetworkForwards("network-name").Return([]lxdapi.NetworkForward{forward}, nil)
	client.EXPECT().DeleteNetworkForward("network-name", "10.19.2.22").Return(nil, errors.New("boom"))

	server, err := lxd.NewServer(client)
	c.Assert(err, jc.ErrorIsNil)

	err = server.DeleteControllerNetworkForwards("network-name", "controller-uuid", "")
	c.Assert(err, gc.ErrorMatches, `deleting controller network forward "10.19.2.22": boom`)
}
