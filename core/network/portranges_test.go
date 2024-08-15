// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type PortRangesSuite struct{}

var _ = gc.Suite(&PortRangesSuite{})

func (s *PortRangesSuite) TestCanMergePortRanges(c *gc.C) {
	c.Check(canMergePortRanges(
		PortRange{Protocol: "tcp", FromPort: 1, ToPort: 10},
		PortRange{Protocol: "tcp", FromPort: 11, ToPort: 20},
	), jc.IsTrue)

	c.Check(canMergePortRanges(
		PortRange{Protocol: "tcp", FromPort: 1, ToPort: 13},
		PortRange{Protocol: "tcp", FromPort: 8, ToPort: 20},
	), jc.IsTrue)

	c.Check(canMergePortRanges(
		PortRange{Protocol: "tcp", FromPort: 1, ToPort: 8},
		PortRange{Protocol: "tcp", FromPort: 12, ToPort: 20},
	), jc.IsFalse)

	c.Check(canMergePortRanges(
		PortRange{Protocol: "tcp", FromPort: 1, ToPort: 10},
		PortRange{Protocol: "udp", FromPort: 11, ToPort: 20},
	), jc.IsFalse)

	c.Check(canMergePortRanges(
		PortRange{Protocol: "tcp", FromPort: 1, ToPort: 13},
		PortRange{Protocol: "udp", FromPort: 8, ToPort: 20},
	), jc.IsFalse)

	c.Check(canMergePortRanges(
		PortRange{Protocol: "icmp"},
		PortRange{Protocol: "icmp"},
	), jc.IsTrue)
}

func (s *PortRangesSuite) TestPortRangeDifference(c *gc.C) {
	diff := portRangeDifference(
		PortRange{Protocol: "tcp", FromPort: 1, ToPort: 10},
		PortRange{Protocol: "tcp", FromPort: 15, ToPort: 15},
	)
	c.Check(diff, gc.HasLen, 1)
	c.Check(diff[0], gc.DeepEquals, PortRange{Protocol: "tcp", FromPort: 1, ToPort: 10})

	diff = portRangeDifference(
		PortRange{Protocol: "tcp", FromPort: 1, ToPort: 10},
		PortRange{Protocol: "tcp", FromPort: 5, ToPort: 15},
	)
	c.Check(diff, gc.HasLen, 1)
	c.Check(diff[0], gc.DeepEquals, PortRange{Protocol: "tcp", FromPort: 1, ToPort: 4})

	diff = portRangeDifference(
		PortRange{Protocol: "tcp", FromPort: 5, ToPort: 10},
		PortRange{Protocol: "tcp", FromPort: 1, ToPort: 15},
	)
	c.Check(diff, gc.HasLen, 0)

	diff = portRangeDifference(
		PortRange{Protocol: "tcp", FromPort: 1, ToPort: 15},
		PortRange{Protocol: "tcp", FromPort: 5, ToPort: 10},
	)
	c.Check(diff, gc.HasLen, 2)
	c.Check(diff[0], gc.DeepEquals, PortRange{Protocol: "tcp", FromPort: 1, ToPort: 4})
	c.Check(diff[1], gc.DeepEquals, PortRange{Protocol: "tcp", FromPort: 11, ToPort: 15})

	diff = portRangeDifference(
		PortRange{Protocol: "tcp", FromPort: 1, ToPort: 10},
		PortRange{Protocol: "tcp", FromPort: 1, ToPort: 10},
	)
	c.Check(diff, gc.HasLen, 0)

	diff = portRangeDifference(
		PortRange{Protocol: "tcp", FromPort: 1, ToPort: 10},
		PortRange{Protocol: "tcp", FromPort: 1, ToPort: 5},
	)
	c.Check(diff, gc.HasLen, 1)
	c.Check(diff[0], gc.DeepEquals, PortRange{Protocol: "tcp", FromPort: 6, ToPort: 10})
}

func (s *PortRangesSuite) TestPortRangeDifferenceAcrossProtocol(c *gc.C) {
	diff := portRangeDifference(
		PortRange{Protocol: "tcp", FromPort: 1, ToPort: 10},
		PortRange{Protocol: "udp", FromPort: 5, ToPort: 15},
	)
	c.Check(diff, gc.HasLen, 1)
	c.Check(diff[0], gc.DeepEquals, PortRange{Protocol: "tcp", FromPort: 1, ToPort: 10})

	diff = portRangeDifference(
		PortRange{Protocol: "tcp", FromPort: 1, ToPort: 10},
		PortRange{Protocol: "icmp"},
	)
	c.Check(diff, gc.HasLen, 1)
	c.Check(diff[0], gc.DeepEquals, PortRange{Protocol: "tcp", FromPort: 1, ToPort: 10})

	diff = portRangeDifference(
		PortRange{Protocol: "icmp"},
		PortRange{Protocol: "tcp", FromPort: 1, ToPort: 10},
	)
	c.Check(diff, gc.HasLen, 1)
	c.Check(diff[0], gc.DeepEquals, PortRange{Protocol: "icmp"})
}

func (s *PortRangesSuite) TestNewPortRanges(c *gc.C) {
	prs := NewPortRanges()
	c.Check(prs, gc.HasLen, 0)

	prs = NewPortRanges(PortRange{Protocol: "tcp", FromPort: 1, ToPort: 10})
	c.Check(prs, gc.HasLen, 1)
	c.Check(prs[0], gc.DeepEquals, PortRange{Protocol: "tcp", FromPort: 1, ToPort: 10})

	prs = NewPortRanges(
		PortRange{Protocol: "tcp", FromPort: 1, ToPort: 10},
		PortRange{Protocol: "udp", FromPort: 20, ToPort: 30},
	)
	c.Check(prs, gc.HasLen, 2)
	c.Check(prs[0], gc.DeepEquals, PortRange{Protocol: "tcp", FromPort: 1, ToPort: 10})
	c.Check(prs[1], gc.DeepEquals, PortRange{Protocol: "udp", FromPort: 20, ToPort: 30})

	prs = NewPortRanges(
		PortRange{Protocol: "udp", FromPort: 20, ToPort: 30},
		PortRange{Protocol: "tcp", FromPort: 1, ToPort: 10},
	)
	c.Check(prs, gc.HasLen, 2)
	c.Check(prs[0], gc.DeepEquals, PortRange{Protocol: "tcp", FromPort: 1, ToPort: 10})
	c.Check(prs[1], gc.DeepEquals, PortRange{Protocol: "udp", FromPort: 20, ToPort: 30})

	prs = NewPortRanges(
		PortRange{Protocol: "tcp", FromPort: 1, ToPort: 10},
		PortRange{Protocol: "tcp", FromPort: 11, ToPort: 20},
	)
	c.Check(prs, gc.HasLen, 1)
	c.Check(prs[0], gc.DeepEquals, PortRange{Protocol: "tcp", FromPort: 1, ToPort: 20})

	prs = NewPortRanges(
		PortRange{Protocol: "tcp", FromPort: 1, ToPort: 10},
		PortRange{Protocol: "udp", FromPort: 11, ToPort: 20},
	)
	c.Check(prs, gc.HasLen, 2)
	c.Check(prs[0], gc.DeepEquals, PortRange{Protocol: "tcp", FromPort: 1, ToPort: 10})
	c.Check(prs[1], gc.DeepEquals, PortRange{Protocol: "udp", FromPort: 11, ToPort: 20})
}

func (s *PortRangesSuite) TestPortRangesEqual(c *gc.C) {
	prs1 := NewPortRanges(
		PortRange{Protocol: "tcp", FromPort: 1, ToPort: 10},
		PortRange{Protocol: "udp", FromPort: 20, ToPort: 30},
	)
	prs2 := NewPortRanges(
		PortRange{Protocol: "tcp", FromPort: 1, ToPort: 10},
		PortRange{Protocol: "udp", FromPort: 20, ToPort: 30},
	)
	c.Check(prs1.EqualTo(prs2), jc.IsTrue)

	prs2 = NewPortRanges(
		PortRange{Protocol: "tcp", FromPort: 1, ToPort: 10},
		PortRange{Protocol: "udp", FromPort: 20, ToPort: 31},
	)
	c.Check(prs1.EqualTo(prs2), jc.IsFalse)

	prs2 = NewPortRanges(
		PortRange{Protocol: "tcp", FromPort: 1, ToPort: 10},
	)
	c.Check(prs1.EqualTo(prs2), jc.IsFalse)

	prs2 = NewPortRanges(
		PortRange{Protocol: "tcp", FromPort: 1, ToPort: 10},
		PortRange{Protocol: "udp", FromPort: 20, ToPort: 30},
		PortRange{Protocol: "icmp"},
	)
	c.Check(prs1.EqualTo(prs2), jc.IsFalse)
}

func (s *PortRangesSuite) TestPortRangesAddOverlappingBridge(c *gc.C) {
	prs := NewPortRanges()
	prs = prs.Add(PortRange{Protocol: "tcp", FromPort: 1, ToPort: 8})
	c.Check(prs, gc.HasLen, 1)
	c.Check(prs[0], gc.DeepEquals, PortRange{Protocol: "tcp", FromPort: 1, ToPort: 8})

	prs = prs.Add(PortRange{Protocol: "tcp", FromPort: 12, ToPort: 20})
	c.Check(prs, gc.HasLen, 2)
	c.Check(prs[0], gc.DeepEquals, PortRange{Protocol: "tcp", FromPort: 1, ToPort: 8})
	c.Check(prs[1], gc.DeepEquals, PortRange{Protocol: "tcp", FromPort: 12, ToPort: 20})

	prs = prs.Add(PortRange{Protocol: "tcp", FromPort: 5, ToPort: 15})
	c.Check(prs, gc.HasLen, 1)
	c.Check(prs[0], gc.DeepEquals, PortRange{Protocol: "tcp", FromPort: 1, ToPort: 20})
}

func (s *PortRangesSuite) TestPortRangesAddAdjacent(c *gc.C) {
	prs := NewPortRanges()
	prs = prs.Add(PortRange{Protocol: "tcp", FromPort: 1, ToPort: 10})
	c.Check(prs, gc.HasLen, 1)
	c.Check(prs[0], gc.DeepEquals, PortRange{Protocol: "tcp", FromPort: 1, ToPort: 10})

	prs = prs.Add(PortRange{Protocol: "tcp", FromPort: 11, ToPort: 20})
	c.Check(prs, gc.HasLen, 1)
	c.Check(prs[0], gc.DeepEquals, PortRange{Protocol: "tcp", FromPort: 1, ToPort: 20})
}

func (s *PortRangesSuite) TestPortRangesAddSubset(c *gc.C) {
	prs := NewPortRanges()
	prs = prs.Add(PortRange{Protocol: "tcp", FromPort: 1, ToPort: 10})
	c.Check(prs, gc.HasLen, 1)
	c.Check(prs[0], gc.DeepEquals, PortRange{Protocol: "tcp", FromPort: 1, ToPort: 10})

	prs = prs.Add(PortRange{Protocol: "tcp", FromPort: 5, ToPort: 8})
	c.Check(prs, gc.HasLen, 1)
	c.Check(prs[0], gc.DeepEquals, PortRange{Protocol: "tcp", FromPort: 1, ToPort: 10})
}

func (s *PortRangesSuite) TestPortRangesAddAdjacentSingletons(c *gc.C) {
	prs := NewPortRanges()
	prs = prs.Add(PortRange{Protocol: "tcp", FromPort: 1, ToPort: 1})
	c.Check(prs, gc.HasLen, 1)
	c.Check(prs[0], gc.DeepEquals, PortRange{Protocol: "tcp", FromPort: 1, ToPort: 1})

	prs = prs.Add(PortRange{Protocol: "tcp", FromPort: 2, ToPort: 2})
	c.Check(prs, gc.HasLen, 1)
	c.Check(prs[0], gc.DeepEquals, PortRange{Protocol: "tcp", FromPort: 1, ToPort: 2})
}

func (s *PortRangesSuite) TestPortRangesAddAdjacentEdgeCase(c *gc.C) {
	prs := NewPortRanges()
	prs = prs.Add(PortRange{Protocol: "tcp", FromPort: 1, ToPort: 10})
	c.Check(prs, gc.HasLen, 1)
	c.Check(prs[0], gc.DeepEquals, PortRange{Protocol: "tcp", FromPort: 1, ToPort: 10})

	prs = prs.Add(PortRange{Protocol: "tcp", FromPort: 10, ToPort: 11})
	c.Check(prs, gc.HasLen, 1)
	c.Check(prs[0], gc.DeepEquals, PortRange{Protocol: "tcp", FromPort: 1, ToPort: 11})
}

func (s *PortRangesSuite) TestPortRangesAddSuperSet(c *gc.C) {
	prs := NewPortRanges()
	prs = prs.Add(PortRange{Protocol: "tcp", FromPort: 10, ToPort: 20})
	c.Check(prs, gc.HasLen, 1)
	c.Check(prs[0], gc.DeepEquals, PortRange{Protocol: "tcp", FromPort: 10, ToPort: 20})

	prs = prs.Add(PortRange{Protocol: "tcp", FromPort: 5, ToPort: 25})
	c.Check(prs, gc.HasLen, 1)
	c.Check(prs[0], gc.DeepEquals, PortRange{Protocol: "tcp", FromPort: 5, ToPort: 25})

	prs = prs.Add(PortRange{Protocol: "tcp", FromPort: 30, ToPort: 40})
	c.Check(prs, gc.HasLen, 2)

	prs = prs.Add(PortRange{Protocol: "tcp", FromPort: 2, ToPort: 45})
	c.Check(prs, gc.HasLen, 1)
	c.Check(prs[0], gc.DeepEquals, PortRange{Protocol: "tcp", FromPort: 2, ToPort: 45})
}

func (s *PortRangesSuite) TestPortRangesAddDifferingProtocol(c *gc.C) {
	prs := NewPortRanges()
	prs = prs.Add(PortRange{Protocol: "tcp", FromPort: 10, ToPort: 20})
	c.Check(prs, gc.HasLen, 1)
	c.Check(prs[0], gc.DeepEquals, PortRange{Protocol: "tcp", FromPort: 10, ToPort: 20})

	prs = prs.Add(PortRange{Protocol: "udp", FromPort: 5, ToPort: 25})
	c.Check(prs, gc.HasLen, 2)
	c.Check(prs[0], gc.DeepEquals, PortRange{Protocol: "tcp", FromPort: 10, ToPort: 20})
	c.Check(prs[1], gc.DeepEquals, PortRange{Protocol: "udp", FromPort: 5, ToPort: 25})

	prs = prs.Add(PortRange{Protocol: "tcp", FromPort: 15, ToPort: 25})
	c.Check(prs, gc.HasLen, 2)
	c.Check(prs[0], gc.DeepEquals, PortRange{Protocol: "tcp", FromPort: 10, ToPort: 25})
	c.Check(prs[1], gc.DeepEquals, PortRange{Protocol: "udp", FromPort: 5, ToPort: 25})
}

func (s *PortRangesSuite) TestPortRangesAddDifferingProtocolMany(c *gc.C) {
	prs := NewPortRanges(
		PortRange{Protocol: "tcp", FromPort: 10, ToPort: 20},
		PortRange{Protocol: "udp", FromPort: 5, ToPort: 25},
		PortRange{Protocol: "icmp"},
		PortRange{Protocol: "tcp", FromPort: 30, ToPort: 40},
		PortRange{Protocol: "udp", FromPort: 25, ToPort: 35},
		PortRange{Protocol: "udp", FromPort: 45, ToPort: 55},
	)
	c.Check(prs, gc.HasLen, 5)

	prs = prs.Add(PortRange{Protocol: "tcp", FromPort: 15, ToPort: 25})
	c.Check(prs, gc.HasLen, 5)
	c.Check(prs[0], gc.DeepEquals, PortRange{Protocol: "icmp"})
	c.Check(prs[1], gc.DeepEquals, PortRange{Protocol: "tcp", FromPort: 10, ToPort: 25})
	c.Check(prs[2], gc.DeepEquals, PortRange{Protocol: "tcp", FromPort: 30, ToPort: 40})
	c.Check(prs[3], gc.DeepEquals, PortRange{Protocol: "udp", FromPort: 5, ToPort: 35})
	c.Check(prs[4], gc.DeepEquals, PortRange{Protocol: "udp", FromPort: 45, ToPort: 55})

	prs = prs.Add(PortRange{Protocol: "tcp", FromPort: 15, ToPort: 35})
	c.Check(prs, gc.HasLen, 4)
	c.Check(prs[0], gc.DeepEquals, PortRange{Protocol: "icmp"})
	c.Check(prs[1], gc.DeepEquals, PortRange{Protocol: "tcp", FromPort: 10, ToPort: 40})
	c.Check(prs[2], gc.DeepEquals, PortRange{Protocol: "udp", FromPort: 5, ToPort: 35})
	c.Check(prs[3], gc.DeepEquals, PortRange{Protocol: "udp", FromPort: 45, ToPort: 55})

	prs = prs.Add(PortRange{Protocol: "tcp", FromPort: 80, ToPort: 80})
	c.Check(prs, gc.HasLen, 5)
	c.Check(prs[0], gc.DeepEquals, PortRange{Protocol: "icmp"})
	c.Check(prs[1], gc.DeepEquals, PortRange{Protocol: "tcp", FromPort: 10, ToPort: 40})
	c.Check(prs[2], gc.DeepEquals, PortRange{Protocol: "tcp", FromPort: 80, ToPort: 80})
	c.Check(prs[3], gc.DeepEquals, PortRange{Protocol: "udp", FromPort: 5, ToPort: 35})
	c.Check(prs[4], gc.DeepEquals, PortRange{Protocol: "udp", FromPort: 45, ToPort: 55})
}

func (s *PortRangesSuite) TestPortRangesAddICMP(c *gc.C) {
	prs := NewPortRanges()
	prs = prs.Add(PortRange{Protocol: "icmp"})
	c.Check(prs, gc.HasLen, 1)
	c.Check(prs[0], gc.DeepEquals, PortRange{Protocol: "icmp"})

	prs = prs.Add(PortRange{Protocol: "icmp"})
	c.Check(prs, gc.HasLen, 1)
	c.Check(prs[0], gc.DeepEquals, PortRange{Protocol: "icmp"})
}

func (s *PortRangesSuite) TestPortRangesRemove(c *gc.C) {
	prs := NewPortRanges(
		PortRange{Protocol: "tcp", FromPort: 1, ToPort: 10},
		PortRange{Protocol: "tcp", FromPort: 20, ToPort: 30},
	)
	prs = prs.Remove(PortRange{Protocol: "tcp", FromPort: 5, ToPort: 15})
	c.Check(prs, gc.HasLen, 2)
	c.Check(prs[0], gc.DeepEquals, PortRange{Protocol: "tcp", FromPort: 1, ToPort: 4})
	c.Check(prs[1], gc.DeepEquals, PortRange{Protocol: "tcp", FromPort: 20, ToPort: 30})

	prs = prs.Remove(PortRange{Protocol: "tcp", FromPort: 20, ToPort: 35})
	c.Check(prs, gc.HasLen, 1)
	c.Check(prs[0], gc.DeepEquals, PortRange{Protocol: "tcp", FromPort: 1, ToPort: 4})

	prs = prs.Remove(PortRange{Protocol: "tcp", FromPort: 2, ToPort: 3})
	c.Check(prs, gc.HasLen, 2)
	c.Check(prs[0], gc.DeepEquals, PortRange{Protocol: "tcp", FromPort: 1, ToPort: 1})
	c.Check(prs[1], gc.DeepEquals, PortRange{Protocol: "tcp", FromPort: 4, ToPort: 4})
}

func (s *PortRangesSuite) TestPortRangeRemoveManyOverlaps(c *gc.C) {
	prs := NewPortRanges(
		PortRange{Protocol: "tcp", FromPort: 1, ToPort: 10},
		PortRange{Protocol: "tcp", FromPort: 20, ToPort: 30},
		PortRange{Protocol: "tcp", FromPort: 40, ToPort: 50},
	)
	prs = prs.Remove(PortRange{Protocol: "tcp", FromPort: 5, ToPort: 45})
	c.Check(prs, gc.HasLen, 2)
	c.Check(prs[0], gc.DeepEquals, PortRange{Protocol: "tcp", FromPort: 1, ToPort: 4})
	c.Check(prs[1], gc.DeepEquals, PortRange{Protocol: "tcp", FromPort: 46, ToPort: 50})
}

func (s *PortRangesSuite) TestPortRangesRemoveAcrossProtocol(c *gc.C) {
	prs := NewPortRanges(
		PortRange{Protocol: "tcp", FromPort: 1, ToPort: 10},
		PortRange{Protocol: "udp", FromPort: 20, ToPort: 30},
		PortRange{Protocol: "tcp", FromPort: 40, ToPort: 50},
	)
	prs = prs.Remove(PortRange{Protocol: "tcp", FromPort: 5, ToPort: 45})
	c.Check(prs, gc.HasLen, 3)
	c.Check(prs[0], gc.DeepEquals, PortRange{Protocol: "tcp", FromPort: 1, ToPort: 4})
	c.Check(prs[1], gc.DeepEquals, PortRange{Protocol: "tcp", FromPort: 46, ToPort: 50})
	c.Check(prs[2], gc.DeepEquals, PortRange{Protocol: "udp", FromPort: 20, ToPort: 30})
}

func (s *PortRangesSuite) TestPortRangesRemoveICMP(c *gc.C) {
	prs := NewPortRanges(
		PortRange{Protocol: "tcp", FromPort: 1, ToPort: 10},
		PortRange{Protocol: "icmp"},
	)
	prs = prs.Remove(PortRange{Protocol: "icmp"})
	c.Check(prs, gc.HasLen, 1)
	c.Check(prs[0], gc.DeepEquals, PortRange{Protocol: "tcp", FromPort: 1, ToPort: 10})
}

func (s *PortRangesSuite) TestUpdatePorts(c *gc.C) {
	currentOpened := NewPortRanges(
		PortRange{Protocol: "tcp", FromPort: 80, ToPort: 80},
		PortRange{Protocol: "udp", FromPort: 53, ToPort: 53},
	)
	openPorts := NewPortRanges(
		PortRange{Protocol: "tcp", FromPort: 443, ToPort: 443},
		PortRange{Protocol: "udp", FromPort: 53, ToPort: 53},
	)
	closePorts := NewPortRanges(
		PortRange{Protocol: "tcp", FromPort: 80, ToPort: 80},
	)
	reconciled := currentOpened.Update(openPorts, closePorts)
	c.Assert(reconciled, jc.SameContents, PortRanges{
		{Protocol: "tcp", FromPort: 443, ToPort: 443},
		{Protocol: "udp", FromPort: 53, ToPort: 53},
	})
}

func (s *PortRangesSuite) TestUpdatePortRangesEmpty(c *gc.C) {
	currentOpened := NewPortRanges(
		PortRange{Protocol: "tcp", FromPort: 100, ToPort: 200},
	)
	openPorts := NewPortRanges()
	closePorts := NewPortRanges()
	reconciled := currentOpened.Update(openPorts, closePorts)
	c.Assert(reconciled, jc.SameContents, PortRanges{
		PortRange{Protocol: "tcp", FromPort: 100, ToPort: 200},
	})
}

func (s *PortRangesSuite) TestUpdatePortsIcmp(c *gc.C) {
	currentOpened := NewPortRanges()
	openPorts := NewPortRanges(
		PortRange{Protocol: "icmp"},
	)
	closePorts := NewPortRanges()
	reconciled := currentOpened.Update(openPorts, closePorts)
	c.Assert(reconciled, jc.SameContents, PortRanges{
		{Protocol: "icmp"},
	})

	currentOpened = NewPortRanges(
		PortRange{Protocol: "icmp"},
	)
	openPorts = NewPortRanges()
	closePorts = NewPortRanges(
		PortRange{Protocol: "icmp"},
	)
	reconciled = currentOpened.Update(openPorts, closePorts)
	c.Assert(reconciled, jc.SameContents, PortRanges{})
}

func (s *PortRangesSuite) TestUpdatePortRanges(c *gc.C) {
	currentOpened := NewPortRanges(
		PortRange{Protocol: "tcp", FromPort: 100, ToPort: 200},
	)
	openPorts := NewPortRanges(
		PortRange{Protocol: "tcp", FromPort: 150, ToPort: 250},
	)
	closePorts := NewPortRanges(
		PortRange{Protocol: "tcp", FromPort: 75, ToPort: 125},
	)
	reconciled := currentOpened.Update(openPorts, closePorts)
	c.Assert(reconciled, jc.SameContents, PortRanges{
		{Protocol: "tcp", FromPort: 126, ToPort: 250},
	})
}

func (s *PortRangesSuite) TestUpdatePortRangesSplit(c *gc.C) {
	currentOpened := NewPortRanges(
		PortRange{Protocol: "tcp", FromPort: 100, ToPort: 200},
	)
	openPorts := NewPortRanges(
		PortRange{Protocol: "tcp", FromPort: 201, ToPort: 250},
	)
	closePorts := NewPortRanges(
		PortRange{Protocol: "tcp", FromPort: 150, ToPort: 175},
	)
	reconciled := currentOpened.Update(openPorts, closePorts)
	c.Assert(reconciled, jc.SameContents, PortRanges{
		{Protocol: "tcp", FromPort: 100, ToPort: 149},
		{Protocol: "tcp", FromPort: 176, ToPort: 250},
	})
}

func (s *PortRangesSuite) TestUpdateMixedProtocolPortRanges(c *gc.C) {
	currentOpened := NewPortRanges(
		PortRange{Protocol: "tcp", FromPort: 100, ToPort: 200},
		PortRange{Protocol: "udp", FromPort: 100, ToPort: 200},
	)
	openPorts := NewPortRanges(
		PortRange{Protocol: "tcp", FromPort: 150, ToPort: 250},
		PortRange{Protocol: "icmp"},
	)
	closePorts := NewPortRanges(
		PortRange{Protocol: "udp", FromPort: 75, ToPort: 125},
	)
	reconciled := currentOpened.Update(openPorts, closePorts)
	c.Assert(reconciled, jc.SameContents, PortRanges{
		{Protocol: "tcp", FromPort: 100, ToPort: 250},
		{Protocol: "udp", FromPort: 126, ToPort: 200},
		{Protocol: "icmp"},
	})
}

func (s *PortRangesSuite) TestUpdatePortRangesBridgesRanges(c *gc.C) {
	currentOpened := NewPortRanges(
		PortRange{Protocol: "tcp", FromPort: 100, ToPort: 200},
		PortRange{Protocol: "tcp", FromPort: 300, ToPort: 400},
	)
	openPorts := NewPortRanges(
		PortRange{Protocol: "tcp", FromPort: 150, ToPort: 350},
	)
	closePorts := NewPortRanges()
	reconciled := currentOpened.Update(openPorts, closePorts)
	c.Assert(reconciled, jc.SameContents, PortRanges{
		{Protocol: "tcp", FromPort: 100, ToPort: 400},
	})
}
