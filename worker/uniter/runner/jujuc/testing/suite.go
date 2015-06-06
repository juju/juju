// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

// The base suite for testing jujuc.Context-related code.
type ContextSuite struct {
	Stub   *testing.Stub
	Info   *ContextInfo
	Helper *ContextHelper
	Hctx   *Context
}

func (s *ContextSuite) SetUpTest(c *gc.C) {
	s.Stub = &testing.Stub{}
	s.Info = &ContextInfo{}
	s.Helper = s.NewHelper(s.Info)
	s.Hctx = s.Helper.Context()
}

// NewHookContext builds a bare-bones Context.
func (s *ContextSuite) NewHookContext() jujuc.Context {
	return s.NewHelper(nil).Context()
}

// NewHelper build a ContextHelper for the provided info.
func (s *ContextSuite) NewHelper(info *ContextInfo) *ContextHelper {
	return NewContextHelper(s.Stub, info)
}

// HookContext returns a unit hook context for the given unit info.
func (s *ContextSuite) HookContext(unit string, settings charm.Settings) *ContextHelper {
	info := &ContextInfo{}
	info.Name = unit
	info.ConfigSettings = settings
	return s.NewHelper(info)
}
