// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package server_test

import (
	"io"

	"github.com/juju/errors"
	"github.com/juju/testing"

	"github.com/juju/juju/resource"
)

type stubUnitDataStore struct {
	*testing.Stub

	ReturnOpenResource  resource.Opened
	ReturnListResources []resource.Resource
}

func (s *stubUnitDataStore) OpenResource(name string) (resource.Resource, io.ReadCloser, error) {
	s.AddCall("OpenResource", name)
	if err := s.NextErr(); err != nil {
		return resource.Resource{}, nil, errors.Trace(err)
	}

	return s.ReturnOpenResource.Resource, s.ReturnOpenResource.ReadCloser, nil
}

func (s *stubUnitDataStore) ListResources() ([]resource.Resource, error) {
	s.AddCall("ListResources")
	if err := s.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return s.ReturnListResources, nil
}
