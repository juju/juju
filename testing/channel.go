// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"time"

	gc "launchpad.net/gocheck"

	jc "launchpad.net/juju-core/testing/checkers"
)

// NotifyAsserterC gives helper functions for making assertions about how a
// channel operates (whether we get a receive event or not, whether it is
// closed, etc.)
type NotifyAsserterC struct {
	// C is a gocheck C structure for doing assertions
	C *gc.C
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
		a.C.Assert(ok, jc.IsTrue)
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
		a.C.Assert(ok, jc.IsFalse)
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
	C *gc.C
	// Chan is the channel we want to receive on
	Chan <-chan interface{}
	// Precond will be called before waiting on the channel, can be nil
	Precond func()
}

// AssertReceive will ensure that we get an event on the channel and the
// channel is not closed. It will return the content received
func (a *ContentAsserterC) AssertReceive() interface{} {
	if a.Precond != nil {
		a.Precond()
	}
	select {
	case content, ok := <-a.Chan:
		a.C.Assert(ok, jc.IsTrue)
		return content
	case <-time.After(LongWait):
		a.C.Fatalf("timed out waiting for channel message")
	}
	return nil
}

// AssertOneReceive checks that we have exactly one message, and no more
func (a *ContentAsserterC) AssertOneReceive() interface{} {
	res := a.AssertReceive()
	a.AssertNoReceive()
	return res
}

// AssertClosed ensures that we get a closed event on the channel
func (a *ContentAsserterC) AssertClosed() {
	if a.Precond != nil {
		a.Precond()
	}
	select {
	case _, ok := <-a.Chan:
		a.C.Assert(ok, jc.IsFalse)
	case <-time.After(LongWait):
		a.C.Fatalf("timed out waiting for channel to close")
	}
}

// Assert that we fail to receive on the channel after a short wait.
func (a *ContentAsserterC) AssertNoReceive() {
	select {
	case content := <-a.Chan:
		a.C.Fatalf("unexpected receive: %#v", content)
	case <-time.After(ShortWait):
	}
}
