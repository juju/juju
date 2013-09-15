// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	gc "launchpad.net/gocheck"
)

// CleanupSuite adds the ability to add cleanup functions that are called
// during either test tear down or suite tear down depending on the method
// called.
type CleanupSuite struct {
	testStack  []func()
	suiteStack []func()
}

func (s *CleanupSuite) SetUpSuite(c *gc.C) {
	s.suiteStack = nil
}

func (s *CleanupSuite) TearDownSuite(c *gc.C) {
	s.callStack(s.suiteStack)
}

func (s *CleanupSuite) SetUpTest(c *gc.C) {
	s.testStack = nil
}

func (s *CleanupSuite) TearDownTest(c *gc.C) {
	s.callStack(s.testStack)
}

func (s *CleanupSuite) callStack(stack []func()) {
	for i := len(stack) - 1; i >= 0; i-- {
		stack[i]()
	}
}

func (s *CleanupSuite) AddCleanup(cleanup func()) {
	s.testStack = append(s.testStack, cleanup)
}

func (s *CleanupSuite) AddSuiteCleanup(cleanup func()) {
	s.suiteStack = append(s.suiteStack, cleanup)
}
