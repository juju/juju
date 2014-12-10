// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network_test

import (
	"strconv"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/network"
	"github.com/juju/juju/testing"
)

type PortSetSuite struct {
	testing.BaseSuite

	portRange1 network.PortRange
	portRange2 network.PortRange
	portRange3 network.PortRange
	portRange4 network.PortRange
}

var _ = gc.Suite(&PortSetSuite{})

func (s *PortSetSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	portRange1, err := network.ParsePortRange("8000-8099/tcp")
	c.Assert(err, jc.ErrorIsNil)
	portRange2, err := network.ParsePortRange("80/tcp")
	c.Assert(err, jc.ErrorIsNil)
	portRange3, err := network.ParsePortRange("79-81/tcp")
	c.Assert(err, jc.ErrorIsNil)
	portRange4, err := network.ParsePortRange("5000-5123/udp")
	c.Assert(err, jc.ErrorIsNil)

	s.portRange1 = *portRange1
	s.portRange2 = *portRange2
	s.portRange3 = *portRange3
	s.portRange4 = *portRange4
}

func (s *PortSetSuite) getPorts(start, end int) []int {
	var result []int
	for i := start; i <= end; i++ {
		result = append(result, i)
	}
	return result
}

func (s *PortSetSuite) checkPortSet(c *gc.C, ports network.PortSet, protocol string, expected ...int) {
	var sexpected []string
	for _, port := range expected {
		sexpected = append(sexpected, strconv.Itoa(port))
	}
	values := ports.PortStrings(protocol)

	c.Check(values, jc.SameContents, sexpected)
}

func (s *PortSetSuite) checkPortSetTCP(c *gc.C, ports network.PortSet, expected ...int) {
	c.Check(ports.Protocols(), jc.SameContents, []string{"tcp"})
	s.checkPortSet(c, ports, "tcp", expected...)
}

func (s *PortSetSuite) checkPorts(c *gc.C, ports []network.Port, protocol string, expected ...int) {
	var found []int
	for _, port := range ports {
		c.Check(port.Protocol, gc.Equals, protocol)
		found = append(found, port.Number)
	}
	c.Check(found, jc.SameContents, expected)
}

func (s *PortSetSuite) TestNewPortSet(c *gc.C) {
	portSet := network.NewPortSet(s.portRange1)
	c.Assert(portSet.IsEmpty(), jc.IsFalse)

	s.checkPortSetTCP(c, portSet, s.getPorts(8000, 8099)...)
}

func (s *PortSetSuite) TestPortSetUnion(c *gc.C) {
	portSet1 := network.NewPortSet(s.portRange2)
	portSet2 := network.NewPortSet(s.portRange3)
	result := portSet1.Union(portSet2)

	s.checkPortSetTCP(c, result, 79, 80, 81)
}

func (s *PortSetSuite) TestPortSetIntersection(c *gc.C) {
	s.portRange2.ToPort = 83
	portSet1 := network.NewPortSet(s.portRange2)
	portSet2 := network.NewPortSet(s.portRange3)
	result := portSet1.Intersection(portSet2)

	s.checkPortSetTCP(c, result, 80, 81)
}

func (s *PortSetSuite) TestPortSetDifference(c *gc.C) {
	s.portRange2.ToPort = 83
	portSet1 := network.NewPortSet(s.portRange2)
	portSet2 := network.NewPortSet(s.portRange3)
	result := portSet1.Difference(portSet2)

	s.checkPortSetTCP(c, result, 82, 83)
}

func (s *PortSetSuite) TestPortSetSize(c *gc.C) {
	portSet := network.NewPortSet(s.portRange1)
	c.Assert(portSet.Size(), gc.Equals, 100)
}

func (s *PortSetSuite) TestPortSetIsEmpty(c *gc.C) {
	portSet := network.NewPortSet()
	c.Assert(portSet.IsEmpty(), jc.IsTrue)
}

func (s *PortSetSuite) TestPortSetIsNotEmpty(c *gc.C) {
	portSet := network.NewPortSet(s.portRange2)
	c.Assert(portSet.IsEmpty(), jc.IsFalse)
}

func (s *PortSetSuite) TestPortSetAdd(c *gc.C) {
	portSet := network.NewPortSet(s.portRange2)
	c.Check(portSet.IsEmpty(), jc.IsFalse)
	portSet.Add(network.Port{Number: 81, Protocol: "tcp"})

	s.checkPortSetTCP(c, portSet, 80, 81)
}

func (s *PortSetSuite) TestPortSetAddRanges(c *gc.C) {
	s.portRange2.ToPort = 83
	portSet := network.NewPortSet(s.portRange2)
	c.Check(portSet.IsEmpty(), jc.IsFalse)
	portSet.AddRanges(s.portRange3)

	s.checkPortSetTCP(c, portSet, s.getPorts(79, 83)...)
}

func (s *PortSetSuite) TestPortSetRemove(c *gc.C) {
	portSet := network.NewPortSet(s.portRange2)
	portSet.Remove(network.Port{Number: 80, Protocol: "tcp"})

	c.Assert(portSet.Ports(), gc.HasLen, 0)
}

func (s *PortSetSuite) TestPortSetContains(c *gc.C) {
	portSet := network.NewPortSet(s.portRange2)
	isfound := portSet.Contains(network.Port{Number: 80, Protocol: "tcp"})

	c.Assert(isfound, jc.IsTrue)
}

func (s *PortSetSuite) TestPortSetContainsNotFound(c *gc.C) {
	portSet := network.NewPortSet(s.portRange2)
	isfound := portSet.Contains(network.Port{Number: 81, Protocol: "tcp"})

	c.Assert(isfound, jc.IsFalse)
}

func (s *PortSetSuite) TestPortSetValues(c *gc.C) {
	portSet := network.NewPortSet(s.portRange3)
	ports := portSet.Values()

	s.checkPorts(c, ports, "tcp", 79, 80, 81)
}

func (s *PortSetSuite) TestPortSetProtocols(c *gc.C) {
	portSet := network.NewPortSet(s.portRange2, s.portRange4)
	protocols := portSet.Protocols()

	c.Check(protocols, jc.SameContents, []string{"tcp", "udp"})
}

func (s *PortSetSuite) TestPortSetPorts(c *gc.C) {
	portSet := network.NewPortSet(s.portRange3)
	ports := portSet.Ports()

	s.checkPorts(c, ports, "tcp", 79, 80, 81)
}

func (s *PortSetSuite) TestPortSetPortStrings(c *gc.C) {
	portSet := network.NewPortSet(s.portRange3)
	ports := portSet.PortStrings("tcp")

	c.Check(ports, jc.SameContents, []string{"79", "80", "81"})
}
