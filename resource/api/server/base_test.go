// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package server_test

import (
	"io"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"

	"github.com/juju/juju/resource"
	"github.com/juju/juju/resource/api"
	"github.com/juju/juju/resource/resourcetesting"
)

type BaseSuite struct {
	testing.IsolationSuite

	stub *testing.Stub
	data *stubDataStore
}

func (s *BaseSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.stub = &testing.Stub{}
	s.data = &stubDataStore{stub: s.stub}
}

func newResource(c *gc.C, name, username, data string) (resource.Resource, api.Resource) {
	opened := resourcetesting.NewResource(c, nil, name, "a-service", data)
	res := opened.Resource
	res.Username = username
	if username == "" {
		res.Timestamp = time.Time{}
	}

	apiRes := api.Resource{
		CharmResource: api.CharmResource{
			Name:        name,
			Type:        "file",
			Path:        res.Path,
			Origin:      "upload",
			Revision:    0,
			Fingerprint: res.Fingerprint.Bytes(),
			Size:        res.Size,
		},
		ID:        res.ID,
		ServiceID: res.ServiceID,
		Username:  username,
		Timestamp: res.Timestamp,
	}

	return res, apiRes
}

type stubDataStore struct {
	stub *testing.Stub

	ReturnListResources         resource.ServiceResources
	ReturnAddPendingResource    string
	ReturnGetResource           resource.Resource
	ReturnGetPendingResource    resource.Resource
	ReturnSetResource           resource.Resource
	ReturnUpdatePendingResource resource.Resource
	ReturnUnits                 []names.UnitTag
}

func (s *stubDataStore) ListResources(service string) (resource.ServiceResources, error) {
	s.stub.AddCall("ListResources", service)
	if err := s.stub.NextErr(); err != nil {
		return resource.ServiceResources{}, errors.Trace(err)
	}

	return s.ReturnListResources, nil
}

func (s *stubDataStore) AddPendingResource(service, userID string, chRes charmresource.Resource, r io.Reader) (string, error) {
	s.stub.AddCall("AddPendingResource", service, userID, chRes, r)
	if err := s.stub.NextErr(); err != nil {
		return "", errors.Trace(err)
	}

	return s.ReturnAddPendingResource, nil
}

func (s *stubDataStore) GetResource(service, name string) (resource.Resource, error) {
	s.stub.AddCall("GetResource", service, name)
	if err := s.stub.NextErr(); err != nil {
		return resource.Resource{}, errors.Trace(err)
	}

	return s.ReturnGetResource, nil
}

func (s *stubDataStore) GetPendingResource(service, name, pendingID string) (resource.Resource, error) {
	s.stub.AddCall("GetPendingResource", service, name, pendingID)
	if err := s.stub.NextErr(); err != nil {
		return resource.Resource{}, errors.Trace(err)
	}

	return s.ReturnGetPendingResource, nil
}

func (s *stubDataStore) SetResource(serviceID, userID string, res charmresource.Resource, r io.Reader) (resource.Resource, error) {
	s.stub.AddCall("SetResource", serviceID, userID, res, r)
	if err := s.stub.NextErr(); err != nil {
		return resource.Resource{}, errors.Trace(err)
	}

	return s.ReturnSetResource, nil
}

func (s *stubDataStore) UpdatePendingResource(serviceID, pendingID, userID string, res charmresource.Resource, r io.Reader) (resource.Resource, error) {
	s.stub.AddCall("UpdatePendingResource", serviceID, pendingID, userID, res, r)
	if err := s.stub.NextErr(); err != nil {
		return resource.Resource{}, errors.Trace(err)
	}

	return s.ReturnUpdatePendingResource, nil
}

func (s *stubDataStore) Units(serviceID string) ([]names.UnitTag, error) {
	s.stub.AddCall("Units", serviceID)
	if err := s.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return s.ReturnUnits, nil
}
