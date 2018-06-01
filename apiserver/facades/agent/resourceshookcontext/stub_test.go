// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resourceshookcontext_test

import (
	"io"

	"github.com/juju/errors"
	"github.com/juju/testing"

	"github.com/juju/juju/resource"
)

type stubUnitDataStore struct {
	*testing.Stub

	ReturnOpenResource  resource.Opened
	ReturnGetResource   resource.Resource
	ReturnListResources resource.ApplicationResources
}

func (s *stubUnitDataStore) OpenResource(name string) (resource.Resource, io.ReadCloser, error) {
	s.AddCall("OpenResource", name)
	if err := s.NextErr(); err != nil {
		return resource.Resource{}, nil, errors.Trace(err)
	}

	return s.ReturnOpenResource.Resource, s.ReturnOpenResource.ReadCloser, nil
}

func (s *stubUnitDataStore) GetResource(name string) (resource.Resource, error) {
	s.AddCall("GetResource", name)
	if err := s.NextErr(); err != nil {
		return resource.Resource{}, errors.Trace(err)
	}

	return s.ReturnGetResource, nil
}

func (s *stubUnitDataStore) ListResources() (resource.ApplicationResources, error) {
	s.AddCall("ListResources")
	if err := s.NextErr(); err != nil {
		return resource.ApplicationResources{}, errors.Trace(err)
	}

	return s.ReturnListResources, nil
}
