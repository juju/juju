// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource_test

import (
	"io"

	charmresource "github.com/juju/charm/v7/resource"
	"github.com/juju/errors"
	"github.com/juju/juju/resource"
	"github.com/juju/testing"

	"github.com/juju/juju/charmstore"
)

type stubCharmStore struct {
	stub *testing.Stub

	ReturnListResources [][]charmresource.Resource
}

func (s *stubCharmStore) ListResources(charms []charmstore.CharmID) ([][]charmresource.Resource, error) {
	s.stub.AddCall("ListResources", charms)
	if err := s.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return s.ReturnListResources, nil
}

type stubAPIClient struct {
	stub *testing.Stub

	resources resource.ApplicationResources
}

func (s *stubAPIClient) Upload(application, name, filename string, resource io.ReadSeeker) error {
	s.stub.AddCall("Upload", application, name, filename, resource)
	if err := s.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (s *stubAPIClient) ListResources(applications []string) ([]resource.ApplicationResources, error) {
	s.stub.AddCall("ListResources", applications)
	if err := s.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}
	return []resource.ApplicationResources{s.resources}, nil
}

func (s *stubAPIClient) Close() error {
	s.stub.AddCall("Close")
	if err := s.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	return nil
}

type stubFile struct {
	// No one actually tries to read from this during tests.
	io.ReadSeeker
	stub *testing.Stub
}

func (s *stubFile) Close() error {
	s.stub.AddCall("FileClose")
	return errors.Trace(s.stub.NextErr())
}
