// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package server_test

import (
	"io"
	"io/ioutil"
	"time"

	"github.com/juju/errors"
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"
	"gopkg.in/macaroon.v1"

	"github.com/juju/juju/resource"
	"github.com/juju/juju/resource/api"
	"github.com/juju/juju/resource/api/server"
	"github.com/juju/juju/resource/resourcetesting"
)

type BaseSuite struct {
	testing.IsolationSuite

	stub     *testing.Stub
	data     *stubDataStore
	csClient *stubCSClient
}

func (s *BaseSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.stub = &testing.Stub{}
	s.data = &stubDataStore{stub: s.stub}
	s.csClient = &stubCSClient{Stub: s.stub}
}

func (s *BaseSuite) newCSClient(cURL *charm.URL, csMac *macaroon.Macaroon) (server.CharmStore, error) {
	s.stub.AddCall("newCSClient", cURL, csMac)
	if err := s.stub.NextErr(); err != nil {
		return nil, err
	}

	return s.csClient, nil
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

type stubCSClient struct {
	*testing.Stub

	ReturnListResources [][]charmresource.Resource
	ReturnGetResource   *charmresource.Resource
}

func (s *stubCSClient) ListResources(cURLs []*charm.URL, channel string) ([][]charmresource.Resource, error) {
	s.AddCall("ListResources", cURLs, channel)
	if err := s.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return s.ReturnListResources, nil
}

func (s *stubCSClient) GetResource(cURL *charm.URL, resourceName string, revision int) (charmresource.Resource, io.ReadCloser, error) {
	s.AddCall("GetResource", cURL, resourceName, revision)
	if err := s.NextErr(); err != nil {
		return charmresource.Resource{}, nil, errors.Trace(err)
	}

	if s.ReturnGetResource == nil {
		return charmresource.Resource{}, nil, errors.NotFoundf("resource %q", resourceName)
	}
	return *s.ReturnGetResource, ioutil.NopCloser(nil), nil
}
