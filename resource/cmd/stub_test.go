// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd

import (
	"io"

	"github.com/juju/errors"
	"github.com/juju/testing"
	"gopkg.in/juju/charm.v6-unstable"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"
)

type stubCharmStore struct {
	stub *testing.Stub

	ReturnListResources [][]charmresource.Resource
}

func (s *stubCharmStore) Connect() (CharmResourceLister, error) {
	s.stub.AddCall("Connect")
	if err := s.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return s, nil
}

func (s *stubCharmStore) ListResources(charmURLs []*charm.URL) ([][]charmresource.Resource, error) {
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

type stubAPIClient struct {
	stub *testing.Stub
}

func (s *stubAPIClient) Upload(service, name string, resource io.ReadSeeker) error {
	s.stub.AddCall("Upload", service, name, resource)
	if err := s.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	return nil
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
