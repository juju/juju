// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/process"
	"github.com/juju/juju/state"
)

type unit struct {
	svc  *service
	unit *state.Unit
}

// SetConfig implements coretesting.Unit.
func (u *unit) SetConfig(c *gc.C, settings map[string]string) {
	u.svc.SetConfig(c, settings)
}

// RunAction implements coretesting.Unit.
func (u *unit) RunAction(c *gc.C, name string, args map[string]interface{}) map[string]interface{} {
	st := u.svc.env.suite.State
	_, err := st.EnqueueAction(u.unit.Tag(), name, args)
	c.Assert(err, jc.ErrorIsNil)

	// Wait until done...
	var action *state.Action
	for i := 0; ; i++ {
		actions, err := u.unit.CompletedActions()
		c.Assert(err, jc.ErrorIsNil)

		if len(actions) > 0 {
			c.Assert(actions, gc.HasLen, 1)
			action = actions[0]
			break
		}

		if i > 100 {
			panic("timed out")
		}
		// sleep...
	}

	results, msg := action.Results()
	c.Assert(msg, gc.Equals, "")
	c.Assert(results["status"], gc.Equals, "success")
	return results
}

// Procs implements coretesting.Unit.
func (u *unit) Procs(c *gc.C) []process.Info {
	up := u.procMethods(c)
	procs, err := up.List()
	c.Assert(err, jc.ErrorIsNil)
	return procs
}

// Destroy implements coretesting.Unit.
func (u *unit) Destroy(c *gc.C) {
	err := u.unit.Destroy()
	c.Assert(err, jc.ErrorIsNil)
}

func (u *unit) procMethods(c *gc.C) state.UnitProcesses {
	st := u.svc.env.suite.State
	up, err := st.UnitProcesses(u.unit)
	c.Assert(err, jc.ErrorIsNil)
	return up
}
