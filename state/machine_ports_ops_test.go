// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/collections/set"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/network"
)

type MachinePortsOpsSuite struct {
}

var _ = gc.Suite(&MachinePortsOpsSuite{})

func (MachinePortsOpsSuite) TestPruneOpenPorts(c *gc.C) {
	op := &openClosePortRangesOperation{
		updatedUnitPortRanges: map[string]unitPortRangesDoc{
			"enigma/0": {
				"": []network.PortRange{
					network.MustParsePortRange("1234-1337/tcp"),
					network.MustParsePortRange("8080/tcp"),
					network.MustParsePortRange("17017/tcp"),
				},
				// The following ranges are also present in the wildcard list.
				// They are therefore redundant and should be pruned.
				"dmz": []network.PortRange{
					network.MustParsePortRange("1234-1337/tcp"),
				},
				"public": []network.PortRange{
					network.MustParsePortRange("8080/tcp"),
				},
			},
		},
	}

	modified := op.pruneOpenPorts()
	c.Assert(modified, jc.IsTrue, gc.Commentf("expected pruneOpenPorts to modify the port list"))

	exp := map[string]unitPortRangesDoc{
		"enigma/0": {
			"": []network.PortRange{
				network.MustParsePortRange("1234-1337/tcp"),
				network.MustParsePortRange("8080/tcp"),
				network.MustParsePortRange("17017/tcp"),
			},
			// Pruned endpoint lists should remain empty
			"dmz":    []network.PortRange{},
			"public": []network.PortRange{},
		},
	}
	c.Assert(op.updatedUnitPortRanges, gc.DeepEquals, exp, gc.Commentf("expected pruneOpenPorts to remove redundant sections for the dmz, public endpoints"))
}

func (MachinePortsOpsSuite) TestPruneEmptySections(c *gc.C) {
	op := &openClosePortRangesOperation{
		updatedUnitPortRanges: map[string]unitPortRangesDoc{
			"enigma/0": {
				"": []network.PortRange{
					network.MustParsePortRange("1234-1337/tcp"),
					network.MustParsePortRange("8080/tcp"),
					network.MustParsePortRange("17017/tcp"),
				},
				"dmz":    []network.PortRange{},
				"public": []network.PortRange{},
			},
			// Since all sections are empty, the prune code is expected
			// to remove the entire map entry for enigma/1
			"enigma/1": {
				"":        []network.PortRange{},
				"coffee":  []network.PortRange{},
				"private": []network.PortRange{},
			},
		},
	}

	modified := op.pruneEmptySections()
	c.Assert(modified, jc.IsTrue, gc.Commentf("expected pruneEmptySections to modify the port list"))

	exp := map[string]unitPortRangesDoc{
		"enigma/0": {
			"": []network.PortRange{
				network.MustParsePortRange("1234-1337/tcp"),
				network.MustParsePortRange("8080/tcp"),
				network.MustParsePortRange("17017/tcp"),
			},
		},
	}
	c.Assert(op.updatedUnitPortRanges, gc.DeepEquals, exp, gc.Commentf("expected prineEmptySections to remove all empty sections and unit docs"))
}

func (MachinePortsOpsSuite) TestMergePendingOpenPortRangesConflict(c *gc.C) {
	op := &openClosePortRangesOperation{
		mpr: &machinePortRanges{
			doc: machinePortRangesDoc{
				UnitRanges: map[string]unitPortRangesDoc{
					"enigma/0": {
						"": []network.PortRange{
							network.MustParsePortRange("1234-1337/tcp"),
							network.MustParsePortRange("8080/tcp"),
							network.MustParsePortRange("17017/tcp"),
						},
					},
				},
			},
			pendingOpenRanges: map[string]unitPortRangesDoc{
				"enigma/1": {
					"tea": []network.PortRange{
						network.MustParsePortRange("1242/tcp"),
					},
				},
			},
		},
	}

	op.cloneExistingUnitPortRanges()
	op.buildPortRangeToUnitMap()

	_, err := op.mergePendingOpenPortRanges()
	c.Assert(err, gc.ErrorMatches, `.*port ranges 1234-1337/tcp \("enigma/0"\) and 1242/tcp \("enigma/1"\) conflict`)
}

func (MachinePortsOpsSuite) TestMergePendingOpenPortRangeDupHandling(c *gc.C) {
	specs := []struct {
		descr       string
		existing    map[string]unitPortRangesDoc
		pendingOpen map[string]unitPortRangesDoc
		exp         map[string]unitPortRangesDoc
		expModified bool
	}{
		{
			descr: "port range already opened by the unit for all endpoints",
			existing: map[string]unitPortRangesDoc{
				"enigma/0": {
					"": []network.PortRange{network.MustParsePortRange("8080/tcp")},
				},
			},
			pendingOpen: map[string]unitPortRangesDoc{
				"enigma/0": {
					"sky": []network.PortRange{network.MustParsePortRange("8080/tcp")},
				},
			},
			exp: map[string]unitPortRangesDoc{
				"enigma/0": {
					"": []network.PortRange{network.MustParsePortRange("8080/tcp")},
				},
			},
			expModified: false,
		},
		{
			descr: "port range already opened by the unit for same endpoint",
			existing: map[string]unitPortRangesDoc{
				"enigma/0": {
					"sky": []network.PortRange{network.MustParsePortRange("8080/tcp")},
				},
			},
			pendingOpen: map[string]unitPortRangesDoc{
				"enigma/0": {
					"sky": []network.PortRange{network.MustParsePortRange("8080/tcp")},
				},
			},
			exp: map[string]unitPortRangesDoc{
				"enigma/0": {
					"sky": []network.PortRange{network.MustParsePortRange("8080/tcp")},
				},
			},
			expModified: false,
		},
		{
			descr: "port range already opened by the unit for other endpoint",
			existing: map[string]unitPortRangesDoc{
				"enigma/0": {
					"sky": []network.PortRange{network.MustParsePortRange("8080/tcp")},
				},
			},
			pendingOpen: map[string]unitPortRangesDoc{
				"enigma/0": {
					"sea": []network.PortRange{network.MustParsePortRange("8080/tcp")},
				},
			},
			exp: map[string]unitPortRangesDoc{
				"enigma/0": {
					"sky": []network.PortRange{network.MustParsePortRange("8080/tcp")},
					"sea": []network.PortRange{network.MustParsePortRange("8080/tcp")},
				},
			},
			expModified: true,
		},
	}

	for i, spec := range specs {
		c.Logf("%d: %s", i, spec.descr)

		op := &openClosePortRangesOperation{
			mpr: &machinePortRanges{
				doc:               machinePortRangesDoc{UnitRanges: spec.existing},
				pendingOpenRanges: spec.pendingOpen,
			},
		}

		op.cloneExistingUnitPortRanges()
		op.buildPortRangeToUnitMap()

		modified, err := op.mergePendingOpenPortRanges()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(modified, gc.Equals, spec.expModified)
		c.Assert(op.updatedUnitPortRanges, gc.DeepEquals, spec.exp)
	}
}

func (MachinePortsOpsSuite) TestMergePendingClosePortRanges(c *gc.C) {
	specs := []struct {
		descr              string
		endpointNamesByApp map[string]set.Strings
		existing           map[string]unitPortRangesDoc
		pendingClose       map[string]unitPortRangesDoc
		exp                map[string]unitPortRangesDoc
		expModified        bool
	}{
		{
			descr: "port range opened by the unit for same endpoint",
			existing: map[string]unitPortRangesDoc{
				"enigma/0": {
					"lava": []network.PortRange{
						network.MustParsePortRange("8080/tcp"),
						network.MustParsePortRange("9999/tcp"),
					},
				},
			},
			pendingClose: map[string]unitPortRangesDoc{
				"enigma/0": {
					"lava": []network.PortRange{network.MustParsePortRange("8080/tcp")},
				},
			},
			exp: map[string]unitPortRangesDoc{
				"enigma/0": {
					"lava": []network.PortRange{network.MustParsePortRange("9999/tcp")},
				},
			},
			expModified: true,
		},
		{
			descr: "port range opened by the unit for another endpoint",
			existing: map[string]unitPortRangesDoc{
				"enigma/0": {
					"lava": []network.PortRange{network.MustParsePortRange("8080/tcp")},
				},
			},
			pendingClose: map[string]unitPortRangesDoc{
				"enigma/0": {
					"volcano": []network.PortRange{network.MustParsePortRange("8080/tcp")},
				},
			},
			exp: map[string]unitPortRangesDoc{
				"enigma/0": {
					// Close request is a no-op
					"lava": []network.PortRange{network.MustParsePortRange("8080/tcp")},
				},
			},
			expModified: false,
		},
		{
			descr: "port range opened by the unit for all endpoints and closed for specific endpoint",
			endpointNamesByApp: map[string]set.Strings{
				"enigma": set.NewStrings("volcano", "lava", "sea"),
			},
			existing: map[string]unitPortRangesDoc{
				"enigma/0": {
					"": []network.PortRange{
						network.MustParsePortRange("7337/tcp"),
						network.MustParsePortRange("8080/tcp"),
					},
				},
			},
			pendingClose: map[string]unitPortRangesDoc{
				"enigma/0": {
					"lava": []network.PortRange{network.MustParsePortRange("8080/tcp")},
				},
			},
			exp: map[string]unitPortRangesDoc{
				"enigma/0": {
					// range removed from wildcard and replaced with
					// entries for the all _other_ known endpoints
					"":        []network.PortRange{network.MustParsePortRange("7337/tcp")},
					"volcano": []network.PortRange{network.MustParsePortRange("8080/tcp")},
					"sea":     []network.PortRange{network.MustParsePortRange("8080/tcp")},
				},
			},
			expModified: true,
		},
	}

	for i, spec := range specs {
		c.Logf("%d: %s", i, spec.descr)

		op := &openClosePortRangesOperation{
			mpr: &machinePortRanges{
				doc:                machinePortRangesDoc{UnitRanges: spec.existing},
				pendingCloseRanges: spec.pendingClose,
			},
			endpointsNamesByApp: spec.endpointNamesByApp,
		}

		op.cloneExistingUnitPortRanges()
		op.buildPortRangeToUnitMap()

		modified, err := op.mergePendingClosePortRanges()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(modified, gc.Equals, spec.expModified)
		c.Assert(op.updatedUnitPortRanges, gc.DeepEquals, spec.exp)
	}
}

func (MachinePortsOpsSuite) TestMergePendingClosePortRangesConflict(c *gc.C) {
	op := &openClosePortRangesOperation{
		mpr: &machinePortRanges{
			doc: machinePortRangesDoc{
				UnitRanges: map[string]unitPortRangesDoc{
					"enigma/0": {
						"": []network.PortRange{
							network.MustParsePortRange("1234-1337/tcp"),
							network.MustParsePortRange("8080/tcp"),
							network.MustParsePortRange("17017/tcp"),
						},
					},
				},
			},
			pendingCloseRanges: map[string]unitPortRangesDoc{
				"codebreaker/0": {
					"tea": []network.PortRange{
						network.MustParsePortRange("1242/tcp"),
					},
				},
			},
		},
	}

	op.cloneExistingUnitPortRanges()
	op.buildPortRangeToUnitMap()

	_, err := op.mergePendingClosePortRanges()
	c.Assert(err, gc.ErrorMatches, `.*port ranges 1234-1337/tcp \("enigma/0"\) and 1242/tcp \("codebreaker/0"\) conflict`)
}
