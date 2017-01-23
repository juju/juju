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

type RuleSetSuite struct {
	testing.BaseSuite

	rule1 network.IngressRule
	rule2 network.IngressRule
	rule3 network.IngressRule
	rule4 network.IngressRule
}

var _ = gc.Suite(&RuleSetSuite{})

func (s *RuleSetSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.rule1 = network.NewOpenIngressRule("tcp", 8000, 8099)
	s.rule2 = network.NewOpenIngressRule("tcp", 80, 80)
	s.rule3 = network.NewOpenIngressRule("tcp", 79, 81)
	s.rule4 = network.NewOpenIngressRule("udp", 5123, 8099)
}

func (s *RuleSetSuite) getPorts(start, end int) []int {
	var result []int
	for i := start; i <= end; i++ {
		result = append(result, i)
	}
	return result
}

func (s *RuleSetSuite) checkPortSet(c *gc.C, ports network.RuleSet, protocol string, expected ...int) {
	var sexpected []string
	for _, port := range expected {
		sexpected = append(sexpected, strconv.Itoa(port))
	}
	values := ports.PortStrings(protocol)

	c.Check(values, jc.SameContents, sexpected)
}

func (s *RuleSetSuite) checkPortSetTCP(c *gc.C, ports network.RuleSet, expected ...int) {
	c.Check(ports.Protocols(), jc.SameContents, []string{"tcp"})
	s.checkPortSet(c, ports, "tcp", expected...)
}

func (s *RuleSetSuite) checkPorts(c *gc.C, ports []network.Port, protocol string, expected ...int) {
	var found []int
	for _, port := range ports {
		c.Check(port.Protocol, gc.Equals, protocol)
		found = append(found, port.Number)
	}
	c.Check(found, jc.SameContents, expected)
}

func (s *RuleSetSuite) TestNewPortSet(c *gc.C) {
	portSet := network.NewRuleSet(s.rule1)
	c.Assert(portSet.IsEmpty(), jc.IsFalse)

	s.checkPortSetTCP(c, portSet, s.getPorts(8000, 8099)...)
}

func (s *RuleSetSuite) TestPortSetUnion(c *gc.C) {
	portSet1 := network.NewRuleSet(s.rule2)
	portSet2 := network.NewRuleSet(s.rule3)
	result := portSet1.Union(portSet2)

	s.checkPortSetTCP(c, result, 79, 80, 81)
}

func (s *RuleSetSuite) TestPortSetIntersection(c *gc.C) {
	s.rule2.ToPort = 83
	portSet1 := network.NewRuleSet(s.rule2)
	portSet2 := network.NewRuleSet(s.rule3)
	result := portSet1.Intersection(portSet2)

	s.checkPortSetTCP(c, result, 80, 81)
}

func (s *RuleSetSuite) TestPortSetIntersectionEmpty(c *gc.C) {
	portSet1 := network.NewRuleSet(s.rule1)
	portSet2 := network.NewRuleSet(s.rule2)
	result := portSet1.Intersection(portSet2)
	isEmpty := result.IsEmpty()

	c.Check(isEmpty, jc.IsTrue)
}

func (s *RuleSetSuite) TestPortSetDifference(c *gc.C) {
	s.rule2.ToPort = 83
	portSet1 := network.NewRuleSet(s.rule2)
	portSet2 := network.NewRuleSet(s.rule3)
	result := portSet1.Difference(portSet2)

	s.checkPortSetTCP(c, result, 82, 83)
}

func (s *RuleSetSuite) TestPortSetDifferenceEmpty(c *gc.C) {
	portSet1 := network.NewRuleSet(s.rule2)
	result := portSet1.Difference(portSet1)
	isEmpty := result.IsEmpty()

	c.Check(isEmpty, jc.IsTrue)
}

func (s *RuleSetSuite) TestPortSetSize(c *gc.C) {
	portSet := network.NewRuleSet(s.rule1)
	c.Assert(portSet.Size(), gc.Equals, 100)
}

func (s *RuleSetSuite) TestPortSetIsEmpty(c *gc.C) {
	portSet := network.NewRuleSet()
	c.Assert(portSet.IsEmpty(), jc.IsTrue)
}

func (s *RuleSetSuite) TestPortSetIsNotEmpty(c *gc.C) {
	portSet := network.NewRuleSet(s.rule2)
	c.Assert(portSet.IsEmpty(), jc.IsFalse)
}

func (s *RuleSetSuite) TestPortSetAdd(c *gc.C) {
	portSet := network.NewRuleSet(s.rule2)
	c.Check(portSet.IsEmpty(), jc.IsFalse)
	portSet.Add("tcp", 81)

	s.checkPortSetTCP(c, portSet, 80, 81)
}

func (s *RuleSetSuite) TestPortSetAddRanges(c *gc.C) {
	s.rule2.ToPort = 83
	portSet := network.NewRuleSet(s.rule2)
	c.Check(portSet.IsEmpty(), jc.IsFalse)
	portSet.AddRanges(s.rule3)

	s.checkPortSetTCP(c, portSet, s.getPorts(79, 83)...)
}

func (s *RuleSetSuite) TestPortSetRemove(c *gc.C) {
	portSet := network.NewRuleSet(s.rule2)
	portSet.Remove("tcp", 80)

	c.Assert(portSet.Ports(), gc.HasLen, 0)
}

func (s *RuleSetSuite) TestPortSetRemoveRanges(c *gc.C) {
	portSet := network.NewRuleSet(s.rule1)
	portSet.RemoveRanges(
		s.rule2,
		network.NewOpenIngressRule("tcp", 7000, 8049),
		network.NewOpenIngressRule("tcp", 8051, 8074),
		network.NewOpenIngressRule("tcp", 8080, 9000),
	)

	s.checkPortSetTCP(c, portSet, 8050, 8075, 8076, 8077, 8078, 8079)
}

func (s *RuleSetSuite) TestPortSetContains(c *gc.C) {
	portSet := network.NewRuleSet(s.rule2)
	isfound := portSet.Contains("tcp", 80)

	c.Assert(isfound, jc.IsTrue)
}

func (s *RuleSetSuite) TestPortSetContainsNotFound(c *gc.C) {
	portSet := network.NewRuleSet(s.rule2)
	isfound := portSet.Contains("tcp", 81)

	c.Assert(isfound, jc.IsFalse)
}

func (s *RuleSetSuite) TestPortSetContainsRangesSingleMatch(c *gc.C) {
	portSet := network.NewRuleSet(s.rule1)
	isfound := portSet.ContainsRanges(network.NewOpenIngressRule("tcp", 8080, 8080))

	c.Assert(isfound, jc.IsTrue)
}

func (s *RuleSetSuite) TestPortSetContainsRangesSingleNoMatch(c *gc.C) {
	portSet := network.NewRuleSet(s.rule1)
	isfound := portSet.ContainsRanges(s.rule2)

	c.Assert(isfound, jc.IsFalse)
}

func (s *RuleSetSuite) TestPortSetContainsRangesOverlapping(c *gc.C) {
	portSet := network.NewRuleSet(s.rule1)
	isfound := portSet.ContainsRanges(network.NewOpenIngressRule("tcp", 7000, 8049))

	c.Assert(isfound, jc.IsFalse)
}

func (s *RuleSetSuite) TestPortSetValues(c *gc.C) {
	portSet := network.NewRuleSet(s.rule3)
	ports := portSet.Values()

	s.checkPorts(c, ports, "tcp", 79, 80, 81)
}

func (s *RuleSetSuite) TestPortSetProtocols(c *gc.C) {
	portSet := network.NewRuleSet(s.rule2, s.rule4)
	protocols := portSet.Protocols()

	c.Check(protocols, jc.SameContents, []string{"tcp", "udp"})
}

func (s *RuleSetSuite) TestPortSetPorts(c *gc.C) {
	portSet := network.NewRuleSet(s.rule3)
	ports := portSet.Ports()

	s.checkPorts(c, ports, "tcp", 79, 80, 81)
}

func (s *RuleSetSuite) TestPortSetPortNumbers(c *gc.C) {
	portSet := network.NewRuleSet(s.rule3)
	ports := portSet.PortNumbers("tcp")

	c.Check(ports, jc.SameContents, []int{79, 80, 81})
}

func (s *RuleSetSuite) TestPortSetPortStrings(c *gc.C) {
	portSet := network.NewRuleSet(s.rule3)
	ports := portSet.PortStrings("tcp")

	c.Check(ports, jc.SameContents, []string{"79", "80", "81"})
}
