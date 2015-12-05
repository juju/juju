// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"

	"github.com/juju/juju/resource"
	"github.com/juju/juju/resource/state"
)

type stubRawState struct {
	stub *testing.Stub

	ReturnPersistence state.Persistence
}

func (s *stubRawState) Persistence() (state.Persistence, error) {
	s.stub.AddCall("Persistence")
	if err := s.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return s.ReturnPersistence, nil
}

type stubPersistence struct {
	stub *testing.Stub

	ReturnListResources []resource.Resource
}

func (s *stubPersistence) ListResources(serviceID string) ([]resource.Resource, error) {
	s.stub.AddCall("ListResources", serviceID)
	if err := s.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return s.ReturnListResources, nil
}
