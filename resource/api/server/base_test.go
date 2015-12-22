// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package server_test

import (
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
	fp, err := charmresource.GenerateFingerprint([]byte(data))
	c.Assert(err, jc.ErrorIsNil)
	var now time.Time
	if username != "" {
		now = time.Now()
	}
	res := resource.Resource{
		Resource: charmresource.Resource{
			Meta: charmresource.Meta{
				Name: name,
				Type: charmresource.TypeFile,
				Path: name + ".tgz",
			},
			Origin:      charmresource.OriginUpload,
			Revision:    1,
			Fingerprint: fp,
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
			Revision:    1,
			Fingerprint: fp.Bytes(),
		},
		Username:  username,
		Timestamp: now,
	}

	return res, apiRes
}

type stubDataStore struct {
	stub *testing.Stub

	ReturnListResources []resource.Resource
}

func (s *stubDataStore) ListResources(service string) ([]resource.Resource, error) {
	s.stub.AddCall("ListResources", service)
	if err := s.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return s.ReturnListResources, nil
}
