// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/network"
)

type reconcileSuite struct{}

var _ = gc.Suite(&reconcileSuite{})

func (s *reconcileSuite) TestReconcilePorts(c *gc.C) {
	currentOpened := []network.PortRange{
		{Protocol: "tcp", FromPort: 80, ToPort: 80},
		{Protocol: "udp", FromPort: 53, ToPort: 53},
	}
	openPorts := []network.PortRange{
		{Protocol: "tcp", FromPort: 443, ToPort: 443},
		{Protocol: "udp", FromPort: 53, ToPort: 53},
	}
	closePorts := []network.PortRange{
		{Protocol: "tcp", FromPort: 80, ToPort: 80},
	}
	reconciled := reconcilePorts(currentOpened, openPorts, closePorts)
	c.Assert(reconciled, jc.SameContents, []network.PortRange{
		{Protocol: "tcp", FromPort: 443, ToPort: 443},
		{Protocol: "udp", FromPort: 53, ToPort: 53},
	})
}

func (s *reconcileSuite) TestReconcilePortRangesEmpty(c *gc.C) {
	currentOpened := []network.PortRange{
		{Protocol: "tcp", FromPort: 100, ToPort: 200},
	}
	openPorts := []network.PortRange{}
	closePorts := []network.PortRange{}
	reconciled := reconcilePorts(currentOpened, openPorts, closePorts)
	c.Assert(reconciled, jc.SameContents, []network.PortRange{
		{Protocol: "tcp", FromPort: 100, ToPort: 200},
	})
}

func (s *reconcileSuite) TestReconcilePortsIcmp(c *gc.C) {
	currentOpened := []network.PortRange{}
	openPorts := []network.PortRange{
		{Protocol: "icmp"},
	}
	closePorts := []network.PortRange{}
	reconciled := reconcilePorts(currentOpened, openPorts, closePorts)
	c.Assert(reconciled, jc.SameContents, []network.PortRange{
		{Protocol: "icmp"},
	})

	currentOpened = []network.PortRange{
		{Protocol: "icmp"},
	}
	openPorts = []network.PortRange{}
	closePorts = []network.PortRange{
		{Protocol: "icmp"},
	}
	reconciled = reconcilePorts(currentOpened, openPorts, closePorts)
	c.Assert(reconciled, jc.SameContents, []network.PortRange{})
}

func (s *reconcileSuite) TestReconcilePortRanges(c *gc.C) {
	currentOpened := []network.PortRange{
		{Protocol: "tcp", FromPort: 100, ToPort: 200},
	}
	openPorts := []network.PortRange{
		{Protocol: "tcp", FromPort: 150, ToPort: 250},
	}
	closePorts := []network.PortRange{
		{Protocol: "tcp", FromPort: 75, ToPort: 125},
	}
	reconciled := reconcilePorts(currentOpened, openPorts, closePorts)
	c.Assert(reconciled, jc.SameContents, []network.PortRange{
		{Protocol: "tcp", FromPort: 126, ToPort: 250},
	})
}

func (s *reconcileSuite) TestReconcilePortRangesSplit(c *gc.C) {
	currentOpened := []network.PortRange{
		{Protocol: "tcp", FromPort: 100, ToPort: 200},
	}
	openPorts := []network.PortRange{
		{Protocol: "tcp", FromPort: 201, ToPort: 250},
	}
	closePorts := []network.PortRange{
		{Protocol: "tcp", FromPort: 150, ToPort: 175},
	}
	reconciled := reconcilePorts(currentOpened, openPorts, closePorts)
	c.Assert(reconciled, jc.SameContents, []network.PortRange{
		{Protocol: "tcp", FromPort: 100, ToPort: 149},
		{Protocol: "tcp", FromPort: 176, ToPort: 250},
	})
}

func (s *reconcileSuite) TestReconcileMixedProtocolPortRanges(c *gc.C) {
	currentOpened := []network.PortRange{
		{Protocol: "tcp", FromPort: 100, ToPort: 200},
		{Protocol: "udp", FromPort: 100, ToPort: 200},
	}
	openPorts := []network.PortRange{
		{Protocol: "tcp", FromPort: 150, ToPort: 250},
		{Protocol: "icmp"},
	}
	closePorts := []network.PortRange{
		{Protocol: "udp", FromPort: 75, ToPort: 125},
	}
	reconciled := reconcilePorts(currentOpened, openPorts, closePorts)
	c.Assert(reconciled, jc.SameContents, []network.PortRange{
		{Protocol: "tcp", FromPort: 100, ToPort: 250},
		{Protocol: "udp", FromPort: 126, ToPort: 200},
		{Protocol: "icmp"},
	})
}

func (s *reconcileSuite) TestReconcilePortRangesBridgesRanges(c *gc.C) {
	currentOpened := []network.PortRange{
		{Protocol: "tcp", FromPort: 100, ToPort: 200},
		{Protocol: "tcp", FromPort: 300, ToPort: 400},
	}
	openPorts := []network.PortRange{
		{Protocol: "tcp", FromPort: 150, ToPort: 350},
	}
	closePorts := []network.PortRange{}
	reconciled := reconcilePorts(currentOpened, openPorts, closePorts)
	c.Assert(reconciled, jc.SameContents, []network.PortRange{
		{Protocol: "tcp", FromPort: 100, ToPort: 400},
	})
}
