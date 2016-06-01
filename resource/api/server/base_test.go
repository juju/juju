// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package server_test

import (
	"io"
	"time"

	"github.com/juju/errors"
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"

	"github.com/juju/juju/charmstore"
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

func (s *BaseSuite) newCSClient() (server.CharmStore, error) {
	s.stub.AddCall("newCSClient")
	if err := s.stub.NextErr(); err != nil {
		return nil, err
	}

	return s.csClient, nil
}

func newResource(c *gc.C, name, username, data string) (resource.Resource, api.Resource) {
	opened := resourcetesting.NewResource(c, nil, name, "a-application", data)
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
		ID:            res.ID,
		ApplicationID: res.ApplicationID,
		Username:      username,
		Timestamp:     res.Timestamp,
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

func (s *stubDataStore) SetResource(applicationID, userID string, res charmresource.Resource, r io.Reader) (resource.Resource, error) {
	s.stub.AddCall("SetResource", applicationID, userID, res, r)
	if err := s.stub.NextErr(); err != nil {
		return resource.Resource{}, errors.Trace(err)
	}

	return s.ReturnSetResource, nil
}

func (s *stubDataStore) UpdatePendingResource(applicationID, pendingID, userID string, res charmresource.Resource, r io.Reader) (resource.Resource, error) {
	s.stub.AddCall("UpdatePendingResource", applicationID, pendingID, userID, res, r)
	if err := s.stub.NextErr(); err != nil {
		return resource.Resource{}, errors.Trace(err)
	}

	return s.ReturnUpdatePendingResource, nil
}

type stubCSClient struct {
	*testing.Stub

	ReturnListResources [][]charmresource.Resource
	ReturnResourceInfo  *charmresource.Resource
}

func (s *stubCSClient) ListResources(charms []charmstore.CharmID) ([][]charmresource.Resource, error) {
	s.AddCall("ListResources", charms)
	if err := s.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return s.ReturnListResources, nil
}

func (s *stubCSClient) ResourceInfo(req charmstore.ResourceRequest) (charmresource.Resource, error) {
	s.AddCall("ResourceInfo", req)
	if err := s.NextErr(); err != nil {
		return charmresource.Resource{}, errors.Trace(err)
	}

	if s.ReturnResourceInfo == nil {
		return charmresource.Resource{}, errors.NotFoundf("resource %q", req.Name)
	}
	return *s.ReturnResourceInfo, nil
}
