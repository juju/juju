// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	"gopkg.in/juju/charm.v6-unstable"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"
)

type stubCharmStore struct {
	stub *testing.Stub

	ReturnListResources [][]charmresource.Resource
}

func (s *stubCharmStore) ListResources(charmURLs []charm.URL) ([][]charmresource.Resource, error) {
	s.stub.AddCall("ListResources", charmURLs)
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
