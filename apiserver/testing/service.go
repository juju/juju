// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/state"
)

func AssertPrincipalServiceDeployed(c *gc.C, st *state.State, serviceName string, curl *charm.URL, forced bool, bundle charm.Charm, cons constraints.Value) *state.Service {
	service, err := st.Service(serviceName)
	c.Assert(err, jc.ErrorIsNil)
	charm, force, err := service.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(force, gc.Equals, forced)
	c.Assert(charm.URL(), gc.DeepEquals, curl)
	// When charms are read from state, storage properties are
	// always deserialised as empty slices if empty or nil, so
	// update bundle to match (bundle comes from parsing charm
	// metadata yaml where nil means nil).
	for name, bundleMeta := range bundle.Meta().Storage {
		if bundleMeta.Properties == nil {
			bundleMeta.Properties = []string{}
			bundle.Meta().Storage[name] = bundleMeta
		}
	}
	c.Assert(charm.Meta(), jc.DeepEquals, bundle.Meta())
	c.Assert(charm.Config(), jc.DeepEquals, bundle.Config())

	serviceCons, err := service.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(serviceCons, gc.DeepEquals, cons)
	units, err := service.AllUnits()
	c.Assert(err, jc.ErrorIsNil)
	for _, unit := range units {
		mid, err := unit.AssignedMachineId()
		c.Assert(err, jc.ErrorIsNil)
		machine, err := st.Machine(mid)
		c.Assert(err, jc.ErrorIsNil)
		machineCons, err := machine.Constraints()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(machineCons, gc.DeepEquals, cons)
	}
	return service
}
