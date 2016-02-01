// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package server_test

import (
	"io"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"

	"github.com/juju/juju/resource"
	"github.com/juju/juju/resource/api"
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
	fp, err := charmresource.GenerateFingerprint(strings.NewReader(data))
	c.Assert(err, jc.ErrorIsNil)
	var size int64
	var now time.Time
	if data != "" {
		size = int64(len(data))
		now = time.Now().UTC()
	}
	res := resource.Resource{
		Resource: charmresource.Resource{
			Meta: charmresource.Meta{
				Name: name,
				Type: charmresource.TypeFile,
				Path: name + ".tgz",
			},
			Origin:      charmresource.OriginUpload,
			Revision:    0,
			Fingerprint: fp,
			Size:        size,
		},
		Username:  username,
		Timestamp: now,
	}
	err = res.Validate()
	c.Assert(err, jc.ErrorIsNil)

	apiRes := api.Resource{
		CharmResource: api.CharmResource{
			Name:        name,
			Type:        "file",
			Path:        name + ".tgz",
			Origin:      "upload",
			Revision:    0,
			Fingerprint: fp.Bytes(),
			Size:        size,
		},
		Username:  username,
		Timestamp: now,
	}

	return res, apiRes
}

type stubDataStore struct {
	stub *testing.Stub

	ReturnListResources resource.ServiceResources
	ReturnGetResource   resource.Resource
}

func (s *stubDataStore) ListResources(service string) (resource.ServiceResources, error) {
	s.stub.AddCall("ListResources", service)
	if err := s.stub.NextErr(); err != nil {
		return resource.ServiceResources{}, errors.Trace(err)
	}

	return s.ReturnListResources, nil
}

func (s *stubDataStore) GetResource(service, name string) (resource.Resource, error) {
	s.stub.AddCall("GetResource", service, name)
	if err := s.stub.NextErr(); err != nil {
		return resource.Resource{}, errors.Trace(err)
	}

	return s.ReturnGetResource, nil
}

func (s *stubDataStore) SetResource(serviceID, userID string, res charmresource.Resource, r io.Reader) error {
	s.stub.AddCall("SetResource", serviceID, userID, res, r)
	if err := s.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (s *stubDataStore) SetUnitResource(id, unitID, serviceID string, res resource.Resource) error {
	s.stub.AddCall("SetUnitResource", id, unitID, serviceID, res)
	if err := s.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	return nil
}
