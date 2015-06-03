// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"
)

// The base suite for testing jujuc.Context-related code.
type ContextSuite struct {
	Stub *testing.Stub
}

func (s *ContextSuite) SetUpTest(c *gc.C) {
	s.Stub = &testing.Stub{}
}

// GetHookContext returns a new hook context.
func (s *ContextSuite) GetHookContext(c *gc.C, info *ContextInfo) *Context {
	return NewContext(s.Stub, info)
}
