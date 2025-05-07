// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource_test

import (
	"context"
	"io"

	"github.com/juju/errors"

	jujuresource "github.com/juju/juju/cmd/juju/resource"
	"github.com/juju/juju/core/resource"
	charmresource "github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/testhelpers"
)

type stubCharmStore struct {
	stub *testhelpers.Stub

	ReturnListResources [][]charmresource.Resource
}

func (s *stubCharmStore) ListResources(ctx context.Context, charms []jujuresource.CharmID) ([][]charmresource.Resource, error) {
	s.stub.AddCall("ListResources", charms)
	if err := s.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return s.ReturnListResources, nil
}

type stubAPIClient struct {
	stub *testhelpers.Stub

	resources resource.ApplicationResources
}

func (s *stubAPIClient) Upload(_ context.Context, application, name, filename, pendingID string, resource io.ReadSeeker) error {
	s.stub.AddCall("Upload", application, name, filename, pendingID, resource)
	if err := s.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (s *stubAPIClient) ListResources(ctx context.Context, applications []string) ([]resource.ApplicationResources, error) {
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
	stub *testhelpers.Stub
}

func (s *stubFile) Close() error {
	s.stub.AddCall("FileClose")
	return errors.Trace(s.stub.NextErr())
}
