// Copyright 2013, 2014 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package testhelpers

import (
	"reflect"
	"sync"

	"github.com/juju/loggo/v2"
)

// NewCallMocker returns a CallMocker which will log calls and results
// utilizing the given logger.
func NewCallMocker(logger loggo.Logger) *CallMocker {
	return &CallMocker{
		logger:  logger,
		results: make(map[string][]*callMockReturner),
	}
}

// CallMocker is a tool which allows tests to dynamically specify
// results for a given set of input parameters.
type CallMocker struct {
	Stub

	logger  loggo.Logger
	results map[string][]*callMockReturner
}

// MethodCall logs the call to a method and any results that will be
// returned. It returns the results previously specified by the Call
// function. If no results were specified, the returned slice will be
// nil.
func (m *CallMocker) MethodCall(receiver interface{}, fnName string, args ...interface{}) []interface{} {
	m.Stub.MethodCall(receiver, fnName, args...)
	m.logger.Debugf("Call: %s(%v)", fnName, args)
	results := m.Results(fnName, args...)
	m.logger.Debugf("Results: %v", results)
	return results
}

// Results returns any results previously specified by calls to the
// Call method. If there are no results, the returned slice will be
// nil.
func (m *CallMocker) Results(fnName string, args ...interface{}) []interface{} {
	for _, r := range m.results[fnName] {
		if reflect.DeepEqual(r.args, args) == false {
			continue
		}
		r.logCall()
		return r.retVals
	}
	return nil
}

// Call is the first half a chained-predicate which registers that
// calls to a function named fnName with arguments args should return
// some value. The returned values are handled by the returned type,
// callMockReturner.
func (m *CallMocker) Call(fnName string, args ...interface{}) *callMockReturner {
	returner := &callMockReturner{args: args}
	// Push on the front to hide old results.
	m.results[fnName] = append([]*callMockReturner{returner}, m.results[fnName]...)
	return returner
}

type callMockReturner struct {
	// args holds a reference to the arguments for which the retVals
	// are valid.
	args []interface{}

	// retVals holds a reference to the values that should be returned
	// when the values held by args are seen.
	retVals []interface{}

	// timesInvoked records the number of times this return has been
	// reached.
	timesInvoked struct {
		sync.Mutex

		value int
	}
}

// Returns declares that this returner should return retVals when
// called. It returns a closure which can be called to determine the
// number of times this return has happened.
func (m *callMockReturner) Returns(retVals ...interface{}) func() int {
	m.retVals = retVals
	return m.numTimesInvoked
}

func (m *callMockReturner) logCall() {
	m.timesInvoked.Lock()
	defer m.timesInvoked.Unlock()
	m.timesInvoked.value++
}

func (m *callMockReturner) numTimesInvoked() int {
	m.timesInvoked.Lock()
	defer m.timesInvoked.Unlock()
	return m.timesInvoked.value
}

func TypeAssertError(err interface{}) error {
	if err == nil {
		return nil
	}
	return err.(error)
}
