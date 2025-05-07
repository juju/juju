// Copyright 2013, 2014 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package testing

import (
	"os/exec"

	"github.com/juju/tc"
)

// CleanupSuite adds the ability to add cleanup functions that are called
// during either test tear down or suite tear down depending on the method
// called.
type CleanupSuite struct {
	testStack    []func(*tc.C)
	suiteStack   []func(*tc.C)
	origSuite    *CleanupSuite
	testsStarted bool
	inTest       bool
	tornDown     bool
}

func (s *CleanupSuite) SetUpSuite(c *tc.C) {
	s.suiteStack = nil
	s.testStack = nil
	s.origSuite = s
	s.testsStarted = false
	s.inTest = false
	s.tornDown = false
}

func (s *CleanupSuite) TearDownSuite(c *tc.C) {
	s.callStack(c, s.suiteStack)
	s.suiteStack = nil
	s.origSuite = nil
	s.tornDown = true
}

func (s *CleanupSuite) SetUpTest(c *tc.C) {
	s.testStack = nil
	s.testsStarted = true
	s.inTest = true
}

func (s *CleanupSuite) TearDownTest(c *tc.C) {
	s.callStack(c, s.testStack)
	s.testStack = nil
	s.inTest = false
}

func (s *CleanupSuite) callStack(c *tc.C, stack []func(*tc.C)) {
	for i := len(stack) - 1; i >= 0; i-- {
		stack[i](c)
	}
}

// AddCleanup pushes the cleanup function onto the stack of functions to be
// called during TearDownTest or TearDownSuite. TearDownTest will be used if
// SetUpTest has already been called, else we will use TearDownSuite
func (s *CleanupSuite) AddCleanup(cleanup func(*tc.C)) {
	if s.origSuite == nil {
		// This is either called before SetUpSuite or after
		// TearDownSuite. Either way, we can't really trust that we're
		// going to call Cleanup correctly.
		if s.tornDown {
			panic("unsafe to call AddCleanup after TearDownSuite")
		} else {
			panic("unsafe to call AddCleanup before SetUpSuite")
		}
	}
	if s != s.origSuite {
		// If you write a test like:
		// func (s MySuite) TestFoo(c *gc.C) {
		//   s.AddCleanup(foo)
		// }
		// The AddCleanup call is unsafe because it modifes
		// s.origSuite but that object disappears once TestFoo
		// returns. So you have to use:
		// func (s *MySuite) TestFoo(c *gc.C) if you want the Cleanup
		// funcs.
		panic("unsafe to call AddCleanup from non pointer receiver test")
	}
	if !s.inTest {
		if s.testsStarted {
			// This indicates that we are not currently in a test
			// (inTest is false), but that we have already run a
			// test for this test suite (testStarted is true).
			// Making a Suite-level change here means that only
			// some of the tests in the suite will see the change,
			// which means it *isn't* a Suite (applies to all
			// tests) level change.
			panic("unsafe to call AddCleanup after a test has been torn down" +
				" before a new test has been set up" +
				" (Suite level changes only make sense before first test is run)")
		}
		// We either haven't called SetUpTest or we've already called
		// TearDownTest, consider this a Suite level cleanup.
		s.suiteStack = append(s.suiteStack, cleanup)
		return
	}
	s.testStack = append(s.testStack, cleanup)
}

// PatchEnvironment sets the environment variable 'name' the the value passed
// in. The old value is saved and returned to the original value at test tear
// down time using a cleanup function.
func (s *CleanupSuite) PatchEnvironment(name, value string) {
	restore := PatchEnvironment(name, value)
	s.AddCleanup(func(*tc.C) { restore() })
}

// PatchEnvPathPrepend prepends the given path to the environment $PATH and restores the
// original path on test teardown.
func (s *CleanupSuite) PatchEnvPathPrepend(dir string) {
	restore := PatchEnvPathPrepend(dir)
	s.AddCleanup(func(*tc.C) { restore() })
}

// PatchValue sets the 'dest' variable the the value passed in. The old value
// is saved and returned to the original value at test tear down time using a
// cleanup function. The value must be assignable to the element type of the
// destination.
func (s *CleanupSuite) PatchValue(dest, value interface{}) {
	restore := PatchValue(dest, value)
	s.AddCleanup(func(*tc.C) { restore() })
}

// HookCommandOutput calls the package function of the same name to mock out
// the result of a particular comand execution, and will call the restore
// function on test teardown.
func (s *CleanupSuite) HookCommandOutput(
	outputFunc *func(cmd *exec.Cmd) ([]byte, error),
	output []byte,
	err error,
) <-chan *exec.Cmd {
	result, restore := HookCommandOutput(outputFunc, output, err)
	s.AddCleanup(func(*tc.C) { restore() })
	return result
}
