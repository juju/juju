// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watchertest

import "github.com/juju/tc"

// A TestReporter is something that can be used to report test failures.  It
// is satisfied by the standard library's *testing.T.
type TestReporter interface {
	Errorf(format string, args ...any)
	Fatalf(format string, args ...any)
}

// TestHelper is a TestReporter that has the Helper method.  It is satisfied
// by the standard library's *testing.T.
type TestHelper interface {
	TestReporter
	Helper()
}

// Idler is an interface that ensures that the change stream is idle.
type Idler interface {
	AssertChangeStreamIdle(c *tc.C)
}

// Harness is a test harness for testing watchers.
// The harness is created to ensure that for every watcher, they have a
// predictable lifecycle. This means that every watcher must adhere to the
// following:
//
//  1. Empty an initial event.
//  2. The change stream should become idle.
//  3. Run the setup for the test.
//  4. The change stream should become idle.
//  5. Assert the changes.
//  6. The watcher should not emit any more changes.
//  7. The change stream should be idle, or become idle.
//
// Steps from 3 to 5 are repeated for each test added to the harness.
// Once the tests are run, the harness ensures that there are no more changes
// before checking that the change stream is idle.
//
// By ensuring that the change stream is idle, we can guarantee that the
//
//  1. The watcher has processed all the changes.
//  2. There are no more changes to be processed.
//
// This isolation technique allows us to test the watcher in a predictable
// manner.
type Harness[T any] struct {
	watcher WatcherC[T]
	idler   Idler
	tests   []harnessTest[T]
}

// NewHarness creates a new Harness. The idler is used to ensure that the
// change stream is idle.
// The watcher is used to assert the changes against. Normally this should be
// a NotifyWatcherC or a StringsWatcherC.
func NewHarness[T any](idler Idler, watcher WatcherC[T]) *Harness[T] {
	h := &Harness[T]{
		watcher: watcher,
		idler:   idler,
	}
	return h
}

// AddTest adds the setup for the test and also the assertion for the setup.
// This is split into two functions to allow for the checking of the
// assert change stream idle call in between the setup and the assertion.
// Run must be called after all the tests have been added.
func (h *Harness[T]) AddTest(setup func(*tc.C), assert func(WatcherC[T])) {
	h.tests = append(h.tests, harnessTest[T]{
		setup:  setup,
		assert: assert,
	})
}

// Run runs all the tests added to the harness.
func (h *Harness[T]) Run(c *tc.C, initial ...T) {
	if len(h.tests) == 0 {
		c.Fatalf("no tests")
	}

	// Ensure that the initial event is sent by the watcher.
	h.watcher.Check(SliceAssert[T](initial...))
	h.idler.AssertChangeStreamIdle(c)

	for i, test := range h.tests {
		c.Logf("running test %d", i)

		// Assert the changes.
		test.setup(c)
		h.idler.AssertChangeStreamIdle(c)
		test.assert(h.watcher)
	}

	// Ensure that the watcher doesn't emit any more changes.

	h.watcher.AssertNoChange()
	h.idler.AssertChangeStreamIdle(c)

	// Now ensure that the watcher is also killed cleanly.

	h.watcher.AssertKilled()
}

type harnessTest[T any] struct {
	setup  func(*tc.C)
	assert func(WatcherC[T])
}
