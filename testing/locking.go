// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"errors"
	"sync"
	"time"
)

// TestLockingFunction verifies that a function obeys a given lock.
//
// Use this as a building block in your own tests for proper locking.
// Parameters are a lock that you expect your function to block on, and
// the function that you want to test for proper locking on that lock.
//
// This helper attempts to verify that the function both obtains and releases
// the lock.  It will panic if the function fails to do either.
// TODO: Support generic sync.Locker instead of just Mutex.
// TODO: This could be a gocheck checker.
// TODO(rog): make this work reliably even for functions that take longer
// than a few µs to execute.
func TestLockingFunction(lock *sync.Mutex, function func()) {
	// We record two events that must happen in the right order.
	// Buffer the channel so that we don't get hung up during attempts
	// to push the events in.
	events := make(chan string, 2)
	// Synchronization channel, to make sure that the function starts
	// trying to run at the point where we're going to make it block.
	proceed := make(chan bool, 1)

	goroutine := func() {
		proceed <- true
		function()
		events <- "complete function"
	}

	lock.Lock()
	go goroutine()
	// Make the goroutine start now.  It should block inside "function()."
	// (It's fine, technically even better, if the goroutine started right
	// away, and this channel is buffered specifically so that it can.)
	<-proceed

	// Give a misbehaved function plenty of rope to hang itself.  We don't
	// want it to block for a microsecond, hand control back to here so we
	// think it's stuck on the lock, and then later continue on its merry
	// lockless journey to finish last, as expected but for the wrong
	// reason.
	for counter := 0; counter < 10; counter++ {
		// TODO: In Go 1.1, use runtime.GoSched instead.
		time.Sleep(0)
	}

	// Unblock the goroutine.
	events <- "release lock"
	lock.Unlock()

	// Now that we've released the lock, the function is unblocked.  Read
	// the 2 events.  (This will wait until the function has completed.)
	firstEvent := <-events
	secondEvent := <-events
	if firstEvent != "release lock" || secondEvent != "complete function" {
		panic(errors.New("function did not obey lock"))
	}

	// Also, the function must have released the lock.
	blankLock := sync.Mutex{}
	if *lock != blankLock {
		panic(errors.New("function did not release lock"))
	}
}
