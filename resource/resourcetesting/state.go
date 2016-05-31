// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resourcetesting

import (
	"github.com/juju/testing"
	"gopkg.in/juju/charm.v6-unstable"
)

// StubUnit is a testing implementation of resource.Unit.
type StubUnit struct {
	*testing.Stub

	ReturnName        string
	ReturnServiceName string
	ReturnCharmURL    *charm.URL
}

// Name implements resource.Unit.
func (s *StubUnit) Name() string {
	s.AddCall("Name")
	s.NextErr() // Pop one off.

	return s.ReturnName
}

// ServiceName implements resource.Unit.
func (s *StubUnit) ServiceName() string {
	s.AddCall("ServiceName")
	s.NextErr() // Pop one off.

	return s.ReturnServiceName
}

// CharmURL implements resource.Unit.
func (s *StubUnit) CharmURL() (*charm.URL, bool) {
	s.AddCall("CharmURL")
	s.NextErr() // Pop one off.

	forceCharm := false
	return s.ReturnCharmURL, forceCharm
}
