// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/juju/tc"

	"github.com/juju/juju/internal/testhelpers"
)

// StubMethodCall is like testing.StubCall, but includes the receiver
// as well.
type StubMethodCall struct {
	Receiver interface{}
	FuncName string
	Args     []interface{}
}

// CheckMethodCalls works like testing.Stub.CheckCalls, but also
// checks the receivers.
func CheckMethodCalls(c *tc.C, stub *testhelpers.Stub, calls ...StubMethodCall) {
	receivers := make([]interface{}, len(calls))
	for i, call := range calls {
		receivers[i] = call.Receiver
	}
	stub.CheckReceivers(c, receivers...)
	c.Check(stub.Calls(), tc.HasLen, len(calls))
	for i, call := range calls {
		stub.CheckCall(c, i, call.FuncName, call.Args...)
	}
}
