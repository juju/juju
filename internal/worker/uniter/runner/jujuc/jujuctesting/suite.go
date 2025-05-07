// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuctesting

import (
	"time"

	"github.com/juju/tc"
	"github.com/juju/testing"

	"github.com/juju/juju/core/application"
	"github.com/juju/juju/internal/charm"
)

// ContextSuite is the base suite for testing jujuc.Context-related code.
type ContextSuite struct {
	Stub *testing.Stub
	Unit string
}

func (s *ContextSuite) SetUpTest(c *tc.C) {
	s.Stub = &testing.Stub{}
	s.Unit = "u/0"
}

// NewInfo builds a ContextInfo with basic default data.
func (s *ContextSuite) NewInfo() *ContextInfo {
	var info ContextInfo

	timestamp := time.Date(2200, time.November, 05, 15, 29, 12, 30, time.UTC)
	gsStatus := application.GoalStateStatus{
		Status: "active",
		Since:  &timestamp,
	}

	info.Unit.Name = s.Unit
	info.ConfigSettings = charm.Settings{
		"empty":               nil,
		"monsters":            false,
		"spline-reticulation": 45.0,
		"title":               "My Title",
		"username":            "admin001",
	}
	info.GoalState = application.GoalState{
		Units: application.UnitsGoalState{
			"mysql/0": gsStatus,
		},
		Relations: map[string]application.UnitsGoalState{
			"db": {
				"mysql/0": gsStatus,
			},
			"server": {
				"wordpress/0": gsStatus,
			},
		},
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
