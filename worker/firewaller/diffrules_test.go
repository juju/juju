// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/network"
)

type DiffRulesSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&DiffRulesSuite{})

func (s *DiffRulesSuite) TestOpenAll(c *gc.C) {
	wanted := []network.IngressRule{
		network.MustNewIngressRule("tcp", 80, 90, "0.0.0.0/0"),
		network.MustNewIngressRule("tcp", 443, 443, "10.0.0.0/24", "192.168.1.0/24"),
		network.MustNewIngressRule("udp", 80, 90, "0.0.0.0/0"),
	}
	toOpen, toClose := diffRanges([]network.IngressRule{}, wanted)
	c.Assert(toClose, gc.HasLen, 0)
	c.Assert(toOpen, jc.DeepEquals, wanted)
}

func (s *DiffRulesSuite) TestCloseAll(c *gc.C) {
	current := []network.IngressRule{
		network.MustNewIngressRule("tcp", 80, 90, "0.0.0.0/0"),
		network.MustNewIngressRule("tcp", 443, 443, "10.0.0.0/24", "192.168.1.0/24"),
		network.MustNewIngressRule("udp", 80, 90, "0.0.0.0/0"),
	}
	toOpen, toClose := diffRanges(current, []network.IngressRule{})
	c.Assert(toOpen, gc.HasLen, 0)
	c.Assert(toClose, jc.DeepEquals, current)
}

func (s *DiffRulesSuite) TestNoPortRangeOverlap(c *gc.C) {
	current := []network.IngressRule{
		network.MustNewIngressRule("tcp", 80, 90, "0.0.0.0/0"),
		network.MustNewIngressRule("tcp", 443, 443, "10.0.0.0/24", "192.168.1.0/24"),
		network.MustNewIngressRule("udp", 80, 90, "0.0.0.0/0"),
	}
	extra := []network.IngressRule{
		network.MustNewIngressRule("tcp", 100, 110, "0.0.0.0/0"),
		network.MustNewIngressRule("tcp", 8080, 8080, "10.0.0.0/24", "192.168.1.0/24"),
		network.MustNewIngressRule("udp", 67, 67, "0.0.0.0/0"),
	}
	wanted := append(current, extra...)
	toOpen, toClose := diffRanges(current, wanted)
	c.Assert(toClose, gc.HasLen, 0)

	network.SortIngressRules(extra)
	c.Assert(toOpen, jc.DeepEquals, extra)
}

func (s *DiffRulesSuite) TestPortRangeOverlapToOpen(c *gc.C) {
	current := []network.IngressRule{
		network.MustNewIngressRule("tcp", 80, 90, "10.0.0.0/24"),
		network.MustNewIngressRule("tcp", 443, 443, "10.0.0.0/24", "192.168.1.0/24"),
		network.MustNewIngressRule("udp", 80, 90, "0.0.0.0/0"),
	}
	extra := []network.IngressRule{
		network.MustNewIngressRule("tcp", 80, 90, "192.168.1.0/24", "10.0.0.0/24"),
		network.MustNewIngressRule("tcp", 8080, 8080, "10.0.0.0/24", "192.168.1.0/24"),
		network.MustNewIngressRule("udp", 67, 67, "0.0.0.0/0"),
	}
	wanted := append(current, extra...)
	toOpen, toClose := diffRanges(current, wanted)
	c.Assert(toClose, gc.HasLen, 0)

	c.Assert(toOpen, jc.DeepEquals, []network.IngressRule{
		network.MustNewIngressRule("tcp", 80, 90, "192.168.1.0/24"),
		network.MustNewIngressRule("tcp", 8080, 8080, "10.0.0.0/24", "192.168.1.0/24"),
		network.MustNewIngressRule("udp", 67, 67, "0.0.0.0/0"),
	})
}

func (s *DiffRulesSuite) TestPortRangeOverlapToClose(c *gc.C) {
	current := []network.IngressRule{
		network.MustNewIngressRule("tcp", 80, 90, "10.0.0.0/24", "192.168.1.0/24"),
		network.MustNewIngressRule("tcp", 443, 443, "10.0.0.0/24", "192.168.1.0/24"),
		network.MustNewIngressRule("udp", 80, 90, "0.0.0.0/0"),
	}
	wanted := []network.IngressRule{
		network.MustNewIngressRule("tcp", 80, 90, "10.0.0.0/24"),
		network.MustNewIngressRule("tcp", 443, 443, "10.0.0.0/24", "192.168.1.0/24"),
		network.MustNewIngressRule("udp", 80, 90, "0.0.0.0/0"),
	}
	toOpen, toClose := diffRanges(current, wanted)
	c.Assert(toOpen, gc.HasLen, 0)

	c.Assert(toClose, jc.DeepEquals, []network.IngressRule{
		network.MustNewIngressRule("tcp", 80, 90, "192.168.1.0/24"),
	})
}

func (s *DiffRulesSuite) TestPortRangeOverlap(c *gc.C) {
	current := []network.IngressRule{
		network.MustNewIngressRule("tcp", 80, 90, "10.0.0.0/24", "192.168.1.0/24"),
		network.MustNewIngressRule("tcp", 443, 443, "10.0.0.0/24", "192.168.1.0/24"),
	}
	wanted := []network.IngressRule{
		network.MustNewIngressRule("tcp", 80, 90, "10.0.0.0/24"),
		network.MustNewIngressRule("tcp", 443, 443, "10.0.0.0/24", "192.168.1.0/24"),
		network.MustNewIngressRule("udp", 80, 90, "0.0.0.0/0"),
	}
	toOpen, toClose := diffRanges(current, wanted)
	c.Assert(toOpen, jc.DeepEquals, []network.IngressRule{
		network.MustNewIngressRule("udp", 80, 90, "0.0.0.0/0"),
	})
	c.Assert(toClose, jc.DeepEquals, []network.IngressRule{
		network.MustNewIngressRule("tcp", 80, 90, "192.168.1.0/24"),
	})
}

func (s *DiffRulesSuite) TestDiffRangesClosesPortsIfRulesAreDisjoint(c *gc.C) {
	current := []network.IngressRule{
		network.MustNewIngressRule("tcp", 3306, 3306, "35.187.158.35/32"),
	}
	wanted := []network.IngressRule{
		network.MustNewIngressRule("tcp", 3306, 3306, "35.187.152.241/32"),
	}
	toOpen, toClose := diffRanges(current, wanted)
	c.Assert(toOpen, gc.DeepEquals, wanted)
	c.Assert(toClose, gc.DeepEquals, current)
}
