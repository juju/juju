// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd

import (
	"io"

	"github.com/juju/errors"
	"github.com/juju/juju/resource"
	"github.com/juju/testing"
	"gopkg.in/juju/charm.v6-unstable"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"
)

type stubCharmStore struct {
	stub *testing.Stub

	ReturnListResources [][]charmresource.Resource
	ReturnResources     []resource.Resource
}

func (s *stubCharmStore) ListResources(charmURLs []charm.URL) ([][]charmresource.Resource, error) {
	s.stub.AddCall("ListResources", charmURLs)
	if err := s.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return s.ReturnListResources, nil
}

func (s *stubCharmStore) Upload(service, name string, resource io.Reader) error {
	s.stub.AddCall("Upload", service, name, resource)
	err := s.stub.NextErr()
	return errors.Trace(err)
}

func (s *stubCharmStore) ShowService(service string) ([]resource.Resource, error) {
	s.stub.AddCall("ShowService", service)
	if err := s.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}
	return s.ReturnResources, nil
}

func (s *stubCharmStore) Close() error {
	s.stub.AddCall("Close")
	if err := s.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	return nil
}
