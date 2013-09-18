// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	gc "launchpad.net/gocheck"
)

type CleanupFunc func(*gc.C)
type cleanupStack []CleanupFunc

// CleanupSuite adds the ability to add cleanup functions that are called
// during either test tear down or suite tear down depending on the method
// called.
type CleanupSuite struct {
	testStack  cleanupStack
	suiteStack cleanupStack
}

func (s *CleanupSuite) SetUpSuite(c *gc.C) {
	s.suiteStack = nil
}

func (s *CleanupSuite) TearDownSuite(c *gc.C) {
	s.callStack(c, s.suiteStack)
}

func (s *CleanupSuite) SetUpTest(c *gc.C) {
	s.testStack = nil
}

func (s *CleanupSuite) TearDownTest(c *gc.C) {
	s.callStack(c, s.testStack)
}

func (s *CleanupSuite) callStack(c *gc.C, stack cleanupStack) {
	for i := len(stack) - 1; i >= 0; i-- {
		stack[i](c)
	}
}

func (s *CleanupSuite) AddCleanup(cleanup CleanupFunc) {
	s.testStack = append(s.testStack, cleanup)
}

func (s *CleanupSuite) AddSuiteCleanup(cleanup CleanupFunc) {
	s.suiteStack = append(s.suiteStack, cleanup)
}
