// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package testhelpers

import (
	"fmt"
	"reflect"
	"sync"

	"github.com/juju/tc"
)

// StubC provides check and assert to Stub test helper.
type StubC interface {
	Check(obtained any, checker tc.Checker, args ...any) bool
}

// StubCall records the name of a called function and the passed args.
type StubCall struct {
	// Funcname is the name of the function that was called.
	FuncName string

	// Args is the set of arguments passed to the function. They are
	// in the same order as the function's parameters
	Args []interface{}
}

// Stub is used in testing to stand in for some other value, to record
// all calls to stubbed methods/functions, and to allow users to set the
// values that are returned from those calls. Stub is intended to be
// embedded in another struct that will define the methods to track:
//
//	type stubConn struct {
//	    *testing.Stub
//	    Response []byte
//	}
//
//	func newStubConn() *stubConn {
//	    return &stubConn{
//	        Stub: &testing.Stub{},
//	    }
//	}
//
//	// Send implements Connection.
//	func (fc *stubConn) Send(request string) []byte {
//	    fc.MethodCall(fc, "Send", request)
//	    return fc.Response, fc.NextErr()
//	}
//
// As demonstrated in the example, embed a pointer to testing.Stub. This
// allows a single testing.Stub to be shared between multiple stubs.
//
// Error return values are set through Stub.Errors. Set it to the errors
// you want returned (or use the convenience method `SetErrors`). The
// `NextErr` method returns the errors from Stub.Errors in sequence,
// falling back to `DefaultError` when the sequence is exhausted. Thus
// each stubbed method should call `NextErr` to get its error return value.
//
// To validate calls made to the stub in a test call the CheckCalls
// (or CheckCall) method:
//
//	s.stub.CheckCalls(c, []StubCall{{
//	    FuncName: "Send",
//	    Args: []interface{}{
//	        expected,
//	    },
//	}})
//
//	s.stub.CheckCall(c, 0, "Send", expected)
//
// Not only is Stub useful for building a interface implementation to
// use in testing (e.g. a network API client), it is also useful in
// regular function patching situations:
//
//	type myStub struct {
//	    *testing.Stub
//	}
//
//	func (f *myStub) SomeFunc(arg interface{}) error {
//	    f.AddCall("SomeFunc", arg)
//	    return f.NextErr()
//	}
//
//	s.PatchValue(&somefunc, s.myStub.SomeFunc)
//
// This allows for easily monitoring the args passed to the patched
// func, as well as controlling the return value from the func in a
// clean manner (by simply setting the correct field on the stub).
type Stub struct {
	mu sync.Mutex // serialises access the to following fields

	// calls is the list of calls that have been registered on the stub
	// (i.e. made on the stub's methods), in the order that they were
	// made.
	calls []StubCall

	// receivers is the list of receivers for all the recorded calls.
	// In the case of non-methods, the receiver is set to nil. The
	// receivers are tracked here rather than as a Receiver field on
	// StubCall because StubCall represents the common case for
	// testing. Typically the receiver does not need to be checked.
	receivers []interface{}

	// errors holds the list of error return values to use for
	// successive calls to methods that return an error. Each call
	// pops the next error off the list. An empty list (the default)
	// implies a nil error. nil may be precede actual errors in the
	// list, which means that the first calls will succeed, followed
	// by the failure. All this is facilitated through the Err method.
	errors []error
}

// TODO(ericsnow) Add something similar to NextErr for all return values
// using reflection?

// NextErr returns the error that should be returned on the nth call to
// any method on the stub. It should be called for the error return in
// all stubbed methods.
func (f *Stub) NextErr() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.errors) == 0 {
		return nil
	}
	err := f.errors[0]
	f.errors = f.errors[1:]
	return err
}

// PopNoErr pops off the next error without returning it. If the error
// is not nil then PopNoErr will panic.
//
// PopNoErr is useful in stub methods that do not return an error.
func (f *Stub) PopNoErr() {
	if err := f.NextErr(); err != nil {
		panic(fmt.Sprintf("expected a nil error, got %v", err))
	}
}

func (f *Stub) addCall(rcvr interface{}, funcName string, args []interface{}) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, StubCall{
		FuncName: funcName,
		Args:     args,
	})
	f.receivers = append(f.receivers, rcvr)
}

// Calls returns the list of calls that have been registered on the stub
// (i.e. made on the stub's methods), in the order that they were made.
func (f *Stub) Calls() []StubCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	v := make([]StubCall, len(f.calls))
	copy(v, f.calls)
	return v
}

// ResetCalls erases the calls recorded by this Stub.
func (f *Stub) ResetCalls() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = nil
}

// AddCall records a stubbed function call for later inspection using the
// CheckCalls method. A nil receiver is recorded. Thus for methods use
// MethodCall. All stubbed functions should call AddCall.
func (f *Stub) AddCall(funcName string, args ...interface{}) {
	f.addCall(nil, funcName, args)
}

// MethodCall records a stubbed method call for later inspection using
// the CheckCalls method. The receiver is added to Stub.Receivers.
func (f *Stub) MethodCall(receiver interface{}, funcName string, args ...interface{}) {
	f.addCall(receiver, funcName, args)
}

// SetErrors sets the sequence of error returns for the stub. Each call
// to Err (thus each stub method call) pops an error off the front. So
// frontloading nil here will allow calls to pass, followed by a
// failure.
func (f *Stub) SetErrors(errors ...error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.errors = errors
}

// CheckCalls verifies that the history of calls on the stub's methods
// matches the expected calls. The receivers are not checked. If they
// are significant then check Stub.Receivers separately.
func (f *Stub) CheckCalls(c StubC, expected []StubCall) {
	if !f.CheckCallNames(c, stubCallNames(expected...)...) {
		return
	}
	c.Check(f.calls, tc.DeepEquals, expected)
}

// CheckCallsUnordered verifies that the history of calls on the stub's methods
// contains the expected calls. The receivers are not checked. If they
// are significant then check Stub.Receivers separately.
// This method explicitly does not check if the calls were made in order, just
// whether they have been made.
func (f *Stub) CheckCallsUnordered(c StubC, expected []StubCall) {
	// Take a copy of all calls made to the stub.
	calls := f.calls[:]
	checkCallMade := func(call StubCall) {
		for i, madeCall := range calls {
			if reflect.DeepEqual(call, madeCall) {
				// Remove found call from the copy of all-calls-made collection.
				calls = append(calls[:i], calls[i+1:]...)
				break
			}
		}
	}

	for _, call := range expected {
		checkCallMade(call)
	}
	// If all expected calls were made, our resulting collection should be empty.
	c.Check(calls, tc.DeepEquals, []StubCall{})
}

// CheckCall checks the recorded call at the given index against the
// provided values. If the index is out of bounds then the check fails.
// The receiver is not checked. If it is significant for a test then it
// can be checked separately:
//
//	c.Check(mystub.Receivers[index], tc.Equals, expected)
func (f *Stub) CheckCall(c StubC, index int, funcName string, args ...interface{}) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !c.Check(index, tc.LessThan, len(f.calls)) {
		return
	}
	call := f.calls[index]
	expected := StubCall{
		FuncName: funcName,
		Args:     args,
	}
	c.Check(call, tc.DeepEquals, expected)
}

// CheckCallNames verifies that the in-order list of called method names
// matches the expected calls.
func (f *Stub) CheckCallNames(c StubC, expected ...string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	funcNames := stubCallNames(f.calls...)
	return c.Check(funcNames, tc.DeepEquals, expected)
}

// CheckNoCalls verifies that none of the stub's methods have been called.
func (f *Stub) CheckNoCalls(c StubC) {
	f.CheckCalls(c, nil)
}

// CheckErrors verifies that the list of errors is matches the expected list.
func (f *Stub) CheckErrors(c StubC, expected ...error) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return c.Check(f.errors, tc.DeepEquals, expected)
}

// CheckReceivers verifies that the list of errors is matches the expected list.
func (f *Stub) CheckReceivers(c StubC, expected ...interface{}) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return c.Check(f.receivers, tc.DeepEquals, expected)
}

func stubCallNames(calls ...StubCall) []string {
	var funcNames []string
	for _, call := range calls {
		funcNames = append(funcNames, call.FuncName)
	}
	return funcNames
}
