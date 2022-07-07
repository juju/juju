// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/juju/charm/v9"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
)

func AssertPrincipalApplicationDeployed(c *gc.C, st *state.State, applicationName string, curl *charm.URL, forced bool, bundle charm.Charm, cons constraints.Value) *state.Application {
	app, err := st.Application(applicationName)
	c.Assert(err, jc.ErrorIsNil)
	charm, force, err := app.Charm()
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

	appCons, err := app.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(appCons, gc.DeepEquals, cons)

	for a := coretesting.LongAttempt.Start(); a.Next(); {
		units, err := app.AllUnits()
		c.Assert(err, jc.ErrorIsNil)
		for _, unit := range units {
			mid, err := unit.AssignedMachineId()
			if !a.HasNext() {
				c.Assert(err, jc.ErrorIsNil)
			} else if err != nil {
				continue
			}
			machine, err := st.Machine(mid)
			c.Assert(err, jc.ErrorIsNil)
			machineCons, err := machine.Constraints()
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(machineCons, gc.DeepEquals, cons)
		}
		break
	}
	return app
}
