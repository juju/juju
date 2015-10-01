// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"

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

	retry(func(last bool) bool {
		units, err := service.AllUnits()
		c.Assert(err, jc.ErrorIsNil)
		for _, unit := range units {
			mid, err := unit.AssignedMachineId()
			if last {
				c.Assert(err, jc.ErrorIsNil)
			} else if err != nil {
				return false
			}
			machine, err := st.Machine(mid)
			c.Assert(err, jc.ErrorIsNil)
			machineCons, err := machine.Constraints()
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(machineCons, gc.DeepEquals, cons)
		}
		return true
	})
	return service
}

// retry is a helper that will retry the given function until it returns true
// for up to 3 seconds.  The last time it is run it'll pass in true to the
// function.
func retry(f func(last bool) bool) {
	x := 0
	for ; x < 30; x++ {
		if f(false) {
			break
		}
		<-time.After(100 * time.Millisecond)
	}
	if x == 30 {
		f(true)
	}
}
