// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"

	"github.com/juju/juju/resource"
)

type stubClient struct {
	stub *testing.Stub

	ReturnListSpecs [][]resource.Spec
}

func (s *stubClient) ListSpecs(serviceIDs ...string) ([][]resource.Spec, error) {
	s.stub.AddCall("ListSpecs", serviceIDs)
	if err := s.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return s.ReturnListSpecs, nil
}

func (s *stubClient) Close() error {
	s.stub.AddCall("Close")
	if err := s.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	return nil
}
