// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"fmt"
	"strings"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	goyaml "gopkg.in/yaml.v1"

	"github.com/juju/juju/process"
)

// Unit exposes the testing-required functionality of a unit.
type Unit interface {
	// SetConfig updates the config settings on the unit's service.
	SetConfig(c *gc.C, settings map[string]string)
	// RunAction runs the requested action and returns the result (if any).
	RunAction(c *gc.C, name string, args map[string]interface{}) map[string]interface{}
	// Procs returns the workload processes known to Juju.
	Procs(c *gc.C) []process.Info
	// Destroy destroys the unit.
	Destroy(c *gc.C)
}

type unit struct {
	svc *service
	id  string
}

func newUnit(svc *service, id string) *unit {
	return &unit{
		svc: svc,
		id:  id,
	}
}

// SetConfig implements Unit.
func (u *unit) SetConfig(c *gc.C, settings map[string]string) {
	u.svc.SetConfig(c, settings)
}

// RunAction implements Unit.
func (u *unit) RunAction(c *gc.C, action string, actionArgs map[string]interface{}) map[string]interface{} {
	args := []string{
		u.id,
		action,
	}
	for k, v := range actionArgs {
		args = append(args, fmt.Sprintf("%s=%q", k, v))
	}
	doOut := u.svc.env.run(c, "action do", args...)
	actionID := strings.Fields(doOut)[1]

	// Now get the result.
	fetchOut := u.svc.env.run(c, "action fetch", "--wait", actionID)
	result := struct {
		Result map[string]interface{}
	}{}
	err := goyaml.Unmarshal([]byte(fetchOut), &result)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result.Result["status"].(string), gc.Equals, "success")
	return result.Result
}

// Procs implements Unit.
func (u *unit) Procs(c *gc.C) []process.Info {
	// TODO(ericsnow) finish!
	return nil
}

// Destroy implements Unit.
func (u *unit) Destroy(c *gc.C) {
	u.svc.env.run(c, "destroy-unit", u.id)

	// TODO(ericsnow) Wait until done.
}
