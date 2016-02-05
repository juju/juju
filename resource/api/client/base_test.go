// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client_test

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"

	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/resource"
	"github.com/juju/juju/resource/api"
)

type BaseSuite struct {
	testing.IsolationSuite

	stub     *testing.Stub
	facade   *stubFacade
	response *api.UploadResult
}

func (s *BaseSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.stub = &testing.Stub{}
	s.facade = newStubFacade(c, s.stub)
	s.response = &api.UploadResult{}
}

func (s *BaseSuite) Do(req *http.Request, body io.ReadSeeker, resp interface{}) error {
	s.stub.AddCall("Do", req, body, resp)
	if err := s.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	result, ok := resp.(*api.UploadResult)
	if !ok {
		msg := fmt.Sprintf("bad response type %T, expected api.UploadResult", resp)
		return errors.NewNotValid(nil, msg)
	}

	*result = *s.response
	return nil
}

func newResourceResult(c *gc.C, serviceID string, names ...string) ([]resource.Resource, api.ResourcesResult) {
	var resources []resource.Resource
	var apiResult api.ResourcesResult
	for _, name := range names {
		data := name + "...spamspamspam"
		res, apiRes := newResource(c, name, "a-user", data)
		resources = append(resources, res)
		apiResult.Resources = append(apiResult.Resources, apiRes)
	}
	return resources, apiResult
}

func newResource(c *gc.C, name, username, data string) (resource.Resource, api.Resource) {
	fp, err := charmresource.GenerateFingerprint(strings.NewReader(data))
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

type stubFacade struct {
	basetesting.StubFacadeCaller

	apiResults map[string]api.ResourcesResult
	pendingIDs []string
}

func newStubFacade(c *gc.C, stub *testing.Stub) *stubFacade {
	s := &stubFacade{
		StubFacadeCaller: basetesting.StubFacadeCaller{
			Stub: stub,
		},
		apiResults: make(map[string]api.ResourcesResult),
	}

	s.FacadeCallFn = func(_ string, args, response interface{}) error {
		switch typedResponse := response.(type) {
		case *api.ResourcesResults:
			typedArgs, ok := args.(*api.ListResourcesArgs)
			c.Assert(ok, jc.IsTrue)

			for _, e := range typedArgs.Entities {
				tag, err := names.ParseTag(e.Tag)
				c.Assert(err, jc.ErrorIsNil)
				service := tag.Id()

				apiResult, ok := s.apiResults[service]
				if !ok {
					apiResult.Error = &params.Error{
						Message: fmt.Sprintf("service %q not found", service),
						Code:    params.CodeNotFound,
					}
				}
				typedResponse.Results = append(typedResponse.Results, apiResult)
			}
		case *api.AddPendingResourcesResult:
			typedResponse.PendingIDs = s.pendingIDs
		default:
			c.Errorf("bad type %T", response)
		}
		return nil
	}

	return s
}

func (s *stubFacade) Close() error {
	s.Stub.AddCall("Close")
	if err := s.Stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	return nil
}
