// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resourceshookcontext_test

import (
	"io"

	"github.com/juju/errors"
	"github.com/juju/testing"

	"github.com/juju/juju/core/resources"
)

type stubUnitDataStore struct {
	*testing.Stub

	ReturnOpenResource  resources.Opened
	ReturnGetResource   resources.Resource
	ReturnListResources resources.ApplicationResources
}

func (s *stubUnitDataStore) OpenResource(name string) (resources.Resource, io.ReadCloser, error) {
	s.AddCall("OpenResource", name)
	if err := s.NextErr(); err != nil {
		return resources.Resource{}, nil, errors.Trace(err)
	}

	return s.ReturnOpenResource.Resource, s.ReturnOpenResource.ReadCloser, nil
}

func (s *stubUnitDataStore) GetResource(name string) (resources.Resource, error) {
	s.AddCall("GetResource", name)
	if err := s.NextErr(); err != nil {
		return resources.Resource{}, errors.Trace(err)
	}

	return s.ReturnGetResource, nil
}

func (s *stubUnitDataStore) ListResources() (resources.ApplicationResources, error) {
	s.AddCall("ListResources")
	if err := s.NextErr(); err != nil {
		return resources.ApplicationResources{}, errors.Trace(err)
	}

	return s.ReturnListResources, nil
}
