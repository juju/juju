// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease_test

import (
	"reflect"

	gc "gopkg.in/check.v1"
	"gopkg.in/juju/worker.v1"

	"github.com/juju/juju/worker/lease"
)

type deadManagerSuite struct{}

var _ = gc.Suite(&deadManagerSuite{})

type deadManagerError struct{}

const DeadManagerErrorMessage = "DeadManagerError"

func (*deadManagerError) Error() string {
	return DeadManagerErrorMessage
}

// This creates a new DeadManager and calls all of its exported methods with zero
// values. All methods should return the specified DeadManagerError.
func (s *deadManagerSuite) TestDeadManager(c *gc.C) {
	deadManagerErr := deadManagerError{}
	deadManager := lease.NewDeadManager(&deadManagerErr)
	deadManagerType := reflect.TypeOf(deadManager)
	deadManagerValue := reflect.ValueOf(deadManager)
	errorIface := reflect.TypeOf((*error)(nil)).Elem()
	workerIface := reflect.TypeOf((*worker.Worker)(nil)).Elem()

	for i := 0; i < deadManagerType.NumMethod(); i++ {
		method := deadManagerType.Method(i)
		methodV := deadManagerValue.MethodByName(method.Name)

		var args []reflect.Value
		for n := 0; n < methodV.Type().NumIn(); n++ {
			argType := methodV.Type().In(n)
			args = append(args, reflect.Zero(argType))
		}

		for j := 0; j < method.Type.NumOut(); j++ {
			if returnType := method.Type.Out(j); returnType.Implements(errorIface) {
				errorValue := methodV.Call(args)[j]
				if _, ok := workerIface.MethodByName(method.Name); ok {
					c.Check(errorValue.Interface(), gc.ErrorMatches, DeadManagerErrorMessage)
				} else {
					c.Check(errorValue.Interface(), gc.ErrorMatches, "lease manager stopped")
				}

			}
		}
	}
}
