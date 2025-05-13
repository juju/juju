// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"reflect"
	"time"

	"github.com/juju/tc"
)

// NotifyAsserterC gives helper functions for making assertions about how a
// channel operates (whether we get a receive event or not, whether it is
// closed, etc.)
type NotifyAsserterC struct {
	// C is a gocheck C structure for doing assertions
	C *tc.C
	// Chan is the channel we want to receive on
	Chan <-chan struct{}
	// Precond will be called before waiting on the channel, can be nil
	Precond func()
}

// AssertReceive will ensure that we get an event on the channel and the
// channel is not closed.
func (a *NotifyAsserterC) AssertReceive() {
	if a.Precond != nil {
		a.Precond()
	}
	select {
	case _, ok := <-a.Chan:
		a.C.Assert(ok, tc.IsTrue)
	case <-time.After(LongWait):
		a.C.Fatalf("timed out waiting for channel message")
	}
}

// AssertOneReceive checks that we have exactly one message, and no more
func (a *NotifyAsserterC) AssertOneReceive() {
	a.AssertReceive()
	a.AssertNoReceive()
}

// AssertClosed ensures that we get a closed event on the channel
func (a *NotifyAsserterC) AssertClosed() {
	if a.Precond != nil {
		a.Precond()
	}
	select {
	case _, ok := <-a.Chan:
		a.C.Assert(ok, tc.IsFalse)
	case <-time.After(LongWait):
		a.C.Fatalf("timed out waiting for channel to close")
	}
}

// Assert that we fail to receive on the channel after a short wait.
func (a *NotifyAsserterC) AssertNoReceive() {
	select {
	case <-a.Chan:
		a.C.Fatalf("unexpected receive")
	case <-time.After(ShortWait):
	}
}

// ContentAsserterC is like NotifyAsserterC in that it checks the behavior of a
// channel. The difference is that we expect actual content on the channel, so
// callers need to put that into and out of an 'interface{}'
type ContentAsserterC struct {
	// C is a gocheck C structure for doing assertions
	C *tc.C
	// Chan is the channel we want to receive on
	Chan interface{}
	// Precond will be called before waiting on the channel, can be nil
	Precond func()
}

// recv waits to receive a value on the channe for the given
// time. It returns the value received, if any, whether it
// was received ok (the channel was not closed) and
// whether the receive timed out.
func (a *ContentAsserterC) recv(timeout time.Duration) (val interface{}, ok, timedOut bool) {
	if a.Precond != nil {
		a.Precond()
	}
	which, v, ok := reflect.Select([]reflect.SelectCase{{
		Dir:  reflect.SelectRecv,
		Chan: reflect.ValueOf(a.Chan),
	}, {
		Dir:  reflect.SelectRecv,
		Chan: reflect.ValueOf(time.After(timeout)),
	}})
	switch which {
	case 0:
		a.C.Assert(ok, tc.IsTrue)
		return v.Interface(), ok, false
	case 1:
		return nil, false, true
	}
	panic("unreachable")
}

// AssertReceive will ensure that we get an event on the channel and the
// channel is not closed. It will return the content received
func (a *ContentAsserterC) AssertReceive() interface{} {
	v, ok, timedOut := a.recv(LongWait)
	if timedOut {
		a.C.Fatalf("timed out waiting for channel message")
	}
	a.C.Assert(ok, tc.IsTrue)
	return v
}

// AssertOneReceive checks that we have exactly one message, and no more
func (a *ContentAsserterC) AssertOneReceive() interface{} {
	res := a.AssertReceive()
	a.AssertNoReceive()
	return res
}

// AssertOneValue checks that exactly 1 message was sent, and that the content DeepEquals the value.
// It also returns the value in case further inspection is desired.
func (a *ContentAsserterC) AssertOneValue(val interface{}) interface{} {
	res := a.AssertReceive()
	a.C.Assert(val, tc.DeepEquals, res)
	a.AssertNoReceive()
	return res
}

// AssertClosed ensures that we get a closed event on the channel
func (a *ContentAsserterC) AssertClosed() {
	_, ok, timedOut := a.recv(LongWait)
	if timedOut {
		a.C.Fatalf("timed out waiting for channel to close")
	}
	a.C.Assert(ok, tc.IsFalse)
}

// Assert that we fail to receive on the channel after a short wait.
func (a *ContentAsserterC) AssertNoReceive() {
	content, _, timedOut := a.recv(ShortWait)
	if timedOut {
		return
	}
	a.C.Fatalf("unexpected receive: %#v", content)
}
