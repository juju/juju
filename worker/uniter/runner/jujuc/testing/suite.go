// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5"
)

// ContextSuite is the base suite for testing jujuc.Context-related code.
type ContextSuite struct {
	Stub *testing.Stub
	Unit string
}

func (s *ContextSuite) SetUpTest(c *gc.C) {
	s.Stub = &testing.Stub{}
	s.Unit = "u/0"
}

// NewInfo builds a ContextInfo with basic default data.
func (s *ContextSuite) NewInfo() *ContextInfo {
	var info ContextInfo
	info.Unit.Name = s.Unit
	info.ConfigSettings = charm.Settings{
		"empty":               nil,
		"monsters":            false,
		"spline-reticulation": 45.0,
		"title":               "My Title",
		"username":            "admin001",
	}
	info.AvailabilityZone = "us-east-1a"
	info.PublicAddress = "gimli.minecraft.testing.invalid"
	info.PrivateAddress = "192.168.0.99"
	return &info
}

// NewHookContext builds a jujuc.Context test double.
func (s *ContextSuite) NewHookContext() (*Context, *ContextInfo) {
	info := s.NewInfo()
	return NewContext(s.Stub, info), info
}
