// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"

	"github.com/juju/juju/resource"
)

type stubCharmStore struct {
	stub *testing.Stub

	ReturnListResources [][]resource.Info
}

func (s *stubCharmStore) ListResources(charmIDs []string) ([][]resource.Info, error) {
	s.stub.AddCall("ListResources", charmIDs)
	if err := s.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return s.ReturnListResources, nil
}

func (s *stubCharmStore) Close() error {
	s.stub.AddCall("Close")
	if err := s.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	return nil
}
