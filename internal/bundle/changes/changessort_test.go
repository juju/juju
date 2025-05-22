// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bundlechanges

import (
	"strings"
	stdtesting "testing"

	"github.com/juju/collections/set"
	"github.com/juju/tc"
)

type changesSortSuite struct {
}

func TestChangesSortSuite(t *stdtesting.T) {
	tc.Run(t, &changesSortSuite{})
}

func (s *changesSortSuite) TestSortVerifyRequirementsMet(c *tc.C) {
	ahead := set.NewStrings()
	sorted, err := csOne().sorted()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(sorted), tc.GreaterThan, 0)
	for i, change := range sorted {
		if i == 0 {
			c.Assert(change.Requires(), tc.HasLen, 0)
		} else {
			for _, req := range change.Requires() {
				c.Assert(ahead.Contains(req), tc.IsTrue, tc.Commentf("%q, not one of %q", req, strings.Join(ahead.Values(), ", ")))
			}
		}
		ahead.Add(change.Id())
	}
}

func (s *changesSortSuite) TestSortIdempotent(c *tc.C) {
	for i := 0; i > 10; i += 1 {
		results, err := csOne().sorted()
		c.Assert(err, tc.ErrorIsNil)
		resultstwo, err := csTwo().sorted()
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(results, tc.DeepEquals, resultstwo)
	}
}

func (s *changesSortSuite) TestInvalidDataForSort(c *tc.C) {
	cs := &changeset{}

	// addCharm-0:
	c0 := newAddCharmChange(AddCharmParams{})
	cs.add(c0)
	// deploy-1: addCharm-0
	d1 := newAddApplicationChange(AddApplicationParams{}, c0.Id(), "failme")
	cs.add(d1)
	// addCharm-2:
	c2 := newAddCharmChange(AddCharmParams{})
	cs.add(c2)
	// 	deploy-3: addCharm-2
	d3 := newAddApplicationChange(AddApplicationParams{}, c2.Id())
	cs.add(d3)
	_, err := cs.sorted()
	c.Assert(err, tc.NotNil)
}

func (s *changesSortSuite) TestSortRelationStable(c *tc.C) {
	// When the order of two changes is irrelevant, toposortFlatten should
	// preserve the original order.
	// This ensures e.g. that when a bundle is deployed and re-exported, we
	// get the apps/relations in the same order.
	results, err := csThree().sorted()
	c.Assert(err, tc.ErrorIsNil)

	var i6, i7, i8 int
	for i, ch := range results {
		switch ch.Id() {
		case "addRelation-6":
			i6 = i
		case "addRelation-7":
			i7 = i
		case "addRelation-8":
			i8 = i
		}
	}
	c.Check(i6, tc.LessThan, i7)
	c.Check(i7, tc.LessThan, i8)
}

func csOne() *changeset {
	// TestSiblingContainers
	// applications:
	//  mysql:
	//    charm: mysql
	//    num_units: 3
	//    to: ["lxd:new"]
	//	keystone:
	//	  charm: keystone
	//	  num_units: 3
	//	  to: ["lxd:mysql"]

	// It's possible for requirements to be in different orders and thus give different
	// results: m13, m14, 15 have requirements a different order than csTwo.
	cs := &changeset{}

	// addCharm-0:
	c0 := newAddCharmChange(AddCharmParams{})
	cs.add(c0)
	// deploy-1: addCharm-0
	d1 := newAddApplicationChange(AddApplicationParams{}, c0.Id())
	cs.add(d1)
	// addCharm-2:
	c2 := newAddCharmChange(AddCharmParams{})
	cs.add(c2)
	// 	deploy-3: addCharm-2
	d3 := newAddApplicationChange(AddApplicationParams{}, c2.Id())
	cs.add(d3)
	// addUnit-4: deploy-1, addMachines-13
	u4 := newAddUnitChange(AddUnitParams{}, d1.Id())
	cs.add(u4)
	// addUnit-5: deploy-1, addMachines-14, addUnit-4
	u5 := newAddUnitChange(AddUnitParams{}, d1.Id(), "addMachines-14", u4.Id())
	cs.add(u5)
	// addUnit-6: deploy-1, addMachines-15, addUnit-5
	u6 := newAddUnitChange(AddUnitParams{}, d1.Id(), "addMachines-15", u5.Id())
	cs.add(u6)
	// addUnit-7: deploy-3, addMachines-10
	u7 := newAddUnitChange(AddUnitParams{}, d3.Id(), "addMachines-10")
	cs.add(u7)
	// addUnit-8: deploy-3, addMachines-11, addUnit-7
	u8 := newAddUnitChange(AddUnitParams{}, d3.Id(), "addMachines-11", u7.Id())
	cs.add(u8)
	// addUnit-9: deploy-3, addMachines-12, addUnit-8
	u9 := newAddUnitChange(AddUnitParams{}, d3.Id(), "addMachines-12", u8.Id())
	cs.add(u9)
	// addMachines-10:
	m10 := newAddMachineChange(AddMachineParams{})
	cs.add(m10)
	// addMachines-11: addMachines-10
	m11 := newAddMachineChange(AddMachineParams{}, m10.Id())
	cs.add(m11)
	// addMachines-12: addMachines-10, addMachines-11
	m12 := newAddMachineChange(AddMachineParams{}, m10.Id(), m11.Id())
	cs.add(m12)
	// addMachines-13: addUnit-7, addMachines-10, addMachines-11, addMachines-12
	m13 := newAddMachineChange(AddMachineParams{}, u7.Id(), m10.Id(), m11.Id(), m12.Id())
	cs.add(m13)
	// addMachines-14: addUnit-8, addMachines-10, addMachines-11, addMachines-12, addMachines-13
	m14 := newAddMachineChange(AddMachineParams{}, u8.Id(), m10.Id(), m11.Id(), m12.Id(), m13.Id())
	cs.add(m14)
	// addMachines-15: addUnit-9, addMachines-11, addMachines-12, addMachines-13, addMachines-14, addMachines-10
	m15 := newAddMachineChange(AddMachineParams{}, u9.Id(), m11.Id(), m12.Id(), m13.Id(), m14.Id(), m10.Id())
	cs.add(m15)

	return cs
}

func csTwo() *changeset {
	// TestSiblingContainers
	// applications:
	//  mysql:
	//    charm: mysql
	//    num_units: 3
	//    to: ["lxd:new"]
	//	keystone:
	//	  charm: keystone
	//	  num_units: 3
	//	  to: ["lxd:mysql"]

	// It's possible for requirements to be in different orders and thus give different
	// results: m13, m14, 15 have requirements a different order than csOne.
	cs := &changeset{}

	// addCharm-0:
	c0 := newAddCharmChange(AddCharmParams{})
	cs.add(c0)
	// deploy-1: addCharm-0
	d1 := newAddApplicationChange(AddApplicationParams{}, c0.Id())
	cs.add(d1)
	// addCharm-2:
	c2 := newAddCharmChange(AddCharmParams{})
	cs.add(c2)
	// 	deploy-3: addCharm-2
	d3 := newAddApplicationChange(AddApplicationParams{}, c2.Id())
	cs.add(d3)
	// addUnit-4: deploy-1, addMachines-13
	u4 := newAddUnitChange(AddUnitParams{}, d1.Id())
	cs.add(u4)
	// addUnit-5: deploy-1, addMachines-14, addUnit-4
	u5 := newAddUnitChange(AddUnitParams{}, d1.Id(), "addMachines-14", u4.Id())
	cs.add(u5)
	// addUnit-6: deploy-1, addMachines-15, addUnit-5
	u6 := newAddUnitChange(AddUnitParams{}, d1.Id(), "addMachines-15", u5.Id())
	cs.add(u6)
	// addUnit-7: deploy-3, addMachines-10
	u7 := newAddUnitChange(AddUnitParams{}, d3.Id(), "addMachines-10")
	cs.add(u7)
	// addUnit-8: deploy-3, addMachines-11, addUnit-7
	u8 := newAddUnitChange(AddUnitParams{}, d3.Id(), "addMachines-11", u7.Id())
	cs.add(u8)
	// addUnit-9: deploy-3, addMachines-12, addUnit-8
	u9 := newAddUnitChange(AddUnitParams{}, d3.Id(), "addMachines-12", u8.Id())
	cs.add(u9)
	// addMachines-10:
	m10 := newAddMachineChange(AddMachineParams{})
	cs.add(m10)
	// addMachines-11: addMachines-10
	m11 := newAddMachineChange(AddMachineParams{}, m10.Id())
	cs.add(m11)
	// addMachines-12: addMachines-10, addMachines-11
	m12 := newAddMachineChange(AddMachineParams{}, m10.Id(), m11.Id())
	cs.add(m12)
	// addMachines-13: addMachines-12, addUnit-7, addMachines-10
	m13 := newAddMachineChange(AddMachineParams{}, m11.Id(), m12.Id(), u7.Id(), m10.Id())
	cs.add(m13)
	// addMachines-14: addMachines-11, addMachines-12, addMachines-13, addMachines-10, addUnit-8
	m14 := newAddMachineChange(AddMachineParams{}, m11.Id(), m12.Id(), m13.Id(), m10.Id(), u8.Id())
	cs.add(m14)
	// addMachines-15: addMachines-14, addUnit-9, addMachines-10, addMachines-11, addMachines-12, addMachines-13
	m15 := newAddMachineChange(AddMachineParams{}, m14.Id(), u9.Id(), m10.Id(), m11.Id(), m12.Id(), m13.Id())
	cs.add(m15)

	return cs
}

func csThree() *changeset {
	// This one includes relations.
	// applications:
	//   snk1:
	//     charm: ./testcharms/charms/dummy-sink
	//   snk2:
	//     charm: ./testcharms/charms/dummy-sink
	//   snk3:
	//     charm: ./testcharms/charms/dummy-sink
	//   src:
	//     charm: ./testcharms/charms/dummy-source
	// relations:
	// - - src:sink
	//   - snk1:source
	// - - src:sink
	//   - snk2:source
	// - - src:sink
	//   - snk3:source

	cs := &changeset{}
	// addCharm-0 (dummy-sink)
	c0 := newAddCharmChange(AddCharmParams{})
	cs.add(c0)
	// deploy-1 $addCharm-0 (snk1)
	d1 := newAddApplicationChange(AddApplicationParams{}, c0.Id())
	cs.add(d1)
	// deploy-2 $addCharm-0 (snk2)
	d2 := newAddApplicationChange(AddApplicationParams{}, c0.Id())
	cs.add(d2)
	// deploy-3 $addCharm-0 (snk3)
	d3 := newAddApplicationChange(AddApplicationParams{}, c0.Id())
	cs.add(d3)
	// addCharm-4 (dummy-source)
	c4 := newAddCharmChange(AddCharmParams{})
	cs.add(c4)
	// deploy-5 $addCharm-4 (src)
	d5 := newAddApplicationChange(AddApplicationParams{}, c4.Id())
	cs.add(d5)
	// addRelation-6 (src--snk1)
	r6 := newAddRelationChange(AddRelationParams{}, d1.Id(), d5.Id())
	cs.add(r6)
	// addRelation-7 (src--snk2)
	r7 := newAddRelationChange(AddRelationParams{}, d2.Id(), d5.Id())
	cs.add(r7)
	// addRelation-8 (src--snk3)
	r8 := newAddRelationChange(AddRelationParams{}, d3.Id(), d5.Id())
	cs.add(r8)

	return cs
}
