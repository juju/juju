// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5"
)

// The base suite for testing jujuc.Context-related code.
type ContextSuite struct {
	Stub *testing.Stub
	Info *ContextInfo
	Ctx  *ContextWrapper
	Hctx *Context
}

func (s *ContextSuite) SetUpTest(c *gc.C) {
	s.Stub = &testing.Stub{}
	s.Info = NewContextInfo()
	s.Ctx = s.NewHookContext(s.Info)
	s.Hctx = s.Ctx.Context
}

// NewHookContext builds a bare-bones Context and wraps it.
func (s *ContextSuite) NewHookContext(info *ContextInfo) *ContextWrapper {
	return &ContextWrapper{
		Context: NewContext(s.Stub, info),
	}
}

// HookContext returns a unit hook context for the given unit info.
func (s *ContextSuite) HookContext(unit string, settings charm.Settings) *ContextWrapper {
	info := NewContextInfo()
	info.Name = unit
	info.ConfigSettings = settings
	return s.NewHookContext(info)
}
