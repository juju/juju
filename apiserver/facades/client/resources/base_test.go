// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources

import (
	"io"
	"time"

	"github.com/juju/charm/v8"
	charmresource "github.com/juju/charm/v8/resource"
	"github.com/juju/errors"
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/charmstore"
	"github.com/juju/juju/core/resources"
	resourcetesting "github.com/juju/juju/core/resources/testing"
	"github.com/juju/juju/rpc/params"
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

func (s *BaseSuite) newCSClient() (CharmStore, error) {
	s.stub.AddCall("newCSClient")
	if err := s.stub.NextErr(); err != nil {
		return nil, err
	}
	return s.csClient, nil
}

func (s *BaseSuite) newCSFactory() func(_ *charm.URL) (NewCharmRepository, error) {
	return func(_ *charm.URL) (NewCharmRepository, error) {
		return newCharmStoreClient(s.csClient), nil
	}
}

func (s *BaseSuite) newLocalFactory() func(_ *charm.URL) (NewCharmRepository, error) {
	return func(_ *charm.URL) (NewCharmRepository, error) {
		return &localClient{}, nil
	}
}

func newResource(c *gc.C, name, username, data string) (resources.Resource, params.Resource) {
	opened := resourcetesting.NewResource(c, nil, name, "a-application", data)
	res := opened.Resource
	res.Username = username
	if username == "" {
		res.Timestamp = time.Time{}
	}

	apiRes := params.Resource{
		CharmResource: params.CharmResource{
			Name:        name,
			Description: name + " description",
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

	ReturnListResources         resources.ApplicationResources
	ReturnAddPendingResource    string
	ReturnGetResource           resources.Resource
	ReturnGetPendingResource    resources.Resource
	ReturnSetResource           resources.Resource
	ReturnUpdatePendingResource resources.Resource
}

func (s *stubDataStore) OpenResource(application, name string) (resources.Resource, io.ReadCloser, error) {
	s.stub.AddCall("OpenResource", application, name)
	if err := s.stub.NextErr(); err != nil {
		return resources.Resource{}, nil, errors.Trace(err)
	}
	return s.ReturnGetResource, nil, nil
}

func (s *stubDataStore) ListResources(application string) (resources.ApplicationResources, error) {
	s.stub.AddCall("ListResources", application)
	if err := s.stub.NextErr(); err != nil {
		return resources.ApplicationResources{}, errors.Trace(err)
	}

	return s.ReturnListResources, nil
}

func (s *stubDataStore) AddPendingResource(application, userID string, chRes charmresource.Resource) (string, error) {
	s.stub.AddCall("AddPendingResource", application, userID, chRes)
	if err := s.stub.NextErr(); err != nil {
		return "", errors.Trace(err)
	}

	return s.ReturnAddPendingResource, nil
}

func (s *stubDataStore) GetResource(application, name string) (resources.Resource, error) {
	s.stub.AddCall("GetResource", application, name)
	if err := s.stub.NextErr(); err != nil {
		return resources.Resource{}, errors.Trace(err)
	}

	return s.ReturnGetResource, nil
}

func (s *stubDataStore) GetPendingResource(application, name, pendingID string) (resources.Resource, error) {
	s.stub.AddCall("GetPendingResource", application, name, pendingID)
	if err := s.stub.NextErr(); err != nil {
		return resources.Resource{}, errors.Trace(err)
	}

	return s.ReturnGetPendingResource, nil
}

func (s *stubDataStore) SetResource(applicationID, userID string, res charmresource.Resource, r io.Reader) (resources.Resource, error) {
	s.stub.AddCall("SetResource", applicationID, userID, res, r)
	if err := s.stub.NextErr(); err != nil {
		return resources.Resource{}, errors.Trace(err)
	}

	return s.ReturnSetResource, nil
}

func (s *stubDataStore) UpdatePendingResource(applicationID, pendingID, userID string, res charmresource.Resource, r io.Reader) (resources.Resource, error) {
	s.stub.AddCall("UpdatePendingResource", applicationID, pendingID, userID, res, r)
	if err := s.stub.NextErr(); err != nil {
		return resources.Resource{}, errors.Trace(err)
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

type stubFactory struct {
	*testing.Stub
	ReturnResources []charmresource.Resource
}

func (s *stubFactory) ResolveResources(resources []charmresource.Resource, charmID CharmID) ([]charmresource.Resource, error) {
	s.AddCall("ResolveResources", resources, charmID)

	if err := s.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}
	return s.ReturnResources, nil
}
