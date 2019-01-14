// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease_test

import (
	"reflect"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/worker/lease"
)

type deadManagerSuite struct{}

var _ = gc.Suite(&deadManagerSuite{})

type deadManagerError struct{}

const DeadManagerErrorMessage = "DeadManagerError"

func (*deadManagerError) Error() string {
	return DeadManagerErrorMessage
}

func (s *deadManagerSuite) TestWait(c *gc.C) {
	deadManagerErr := deadManagerError{}
	deadManager := lease.NewDeadManager(&deadManagerErr)
	c.Assert(deadManager.Wait(), gc.ErrorMatches, DeadManagerErrorMessage)
}

// This creates a new DeadManager, gets a Claimer, and calls all of
// its exported methods with zero values. All methods should return
// the error indicating that the manager is stopped.
func (s *deadManagerSuite) TestClaimer(c *gc.C) {
	deadManagerErr := deadManagerError{}
	deadManager := lease.NewDeadManager(&deadManagerErr)

	claimer, err := deadManager.Claimer("namespace", "model")
	c.Assert(err, jc.ErrorIsNil)
	checkMethods(c, claimer)
}

// And the same for Checker.
func (s *deadManagerSuite) TestChecker(c *gc.C) {
	deadManagerErr := deadManagerError{}
	deadManager := lease.NewDeadManager(&deadManagerErr)

	checker, err := deadManager.Checker("namespace", "model")
	c.Assert(err, jc.ErrorIsNil)
	checkMethods(c, checker)
}

// And Pinner.
func (s *deadManagerSuite) TestPinner(c *gc.C) {
	deadManagerErr := deadManagerError{}
	deadManager := lease.NewDeadManager(&deadManagerErr)

	checker, err := deadManager.Pinner("namespace", "model")
	c.Assert(err, jc.ErrorIsNil)
	checkMethods(c, checker)
}

func checkMethods(c *gc.C, manager interface{}) {
	managerType := reflect.TypeOf(manager)
	managerValue := reflect.ValueOf(manager)
	errorIface := reflect.TypeOf((*error)(nil)).Elem()

	for i := 0; i < managerType.NumMethod(); i++ {
		method := managerType.Method(i)
		methodV := managerValue.MethodByName(method.Name)

		var args []reflect.Value
		for n := 0; n < methodV.Type().NumIn(); n++ {
			argType := methodV.Type().In(n)
			args = append(args, reflect.Zero(argType))
		}

		for j := 0; j < method.Type.NumOut(); j++ {
			if returnType := method.Type.Out(j); returnType.Implements(errorIface) {
				errorValue := methodV.Call(args)[j]
				c.Logf(method.Name)
				c.Check(errorValue.Interface(), gc.ErrorMatches, "lease manager stopped")

			}
		}
	}
}
