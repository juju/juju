// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client_test

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/resource"
	"github.com/juju/juju/resource/resourcetesting"
)

type BaseSuite struct {
	testing.IsolationSuite

	stub     *testing.Stub
	facade   *stubFacade
	response *params.UploadResult
}

func (s *BaseSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.stub = &testing.Stub{}
	s.facade = newStubFacade(c, s.stub)
	s.response = &params.UploadResult{}
}

func (s *BaseSuite) Do(ctx context.Context, req *http.Request, resp interface{}) error {
	s.stub.AddCall("Do", req, resp)
	if err := s.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	result, ok := resp.(*params.UploadResult)
	if !ok {
		msg := fmt.Sprintf("bad response type %T, expected params.UploadResult", resp)
		return errors.NewNotValid(nil, msg)
	}

	*result = *s.response
	return nil
}

func newResourceResult(c *gc.C, applicationID string, names ...string) ([]resource.Resource, params.ResourcesResult) {
	var resources []resource.Resource
	var apiResult params.ResourcesResult
	for _, name := range names {
		data := name + "...spamspamspam"
		res, apiRes := newResource(c, name, "a-user", data)
		resources = append(resources, res)
		apiResult.Resources = append(apiResult.Resources, apiRes)
	}
	return resources, apiResult
}

func newResource(c *gc.C, name, username, data string) (resource.Resource, params.Resource) {
	opened := resourcetesting.NewResource(c, nil, name, "a-application", data)
	res := opened.Resource
	res.Revision = 1
	res.Username = username
	if username == "" {
		// Note that resourcetesting.NewResource() returns a resources
		// with a username and timestamp set. So if the username was
		// "un-set" then we have to also unset the timestamp.
		res.Timestamp = time.Time{}
	}

	apiRes := params.Resource{
		CharmResource: params.CharmResource{
			Name:        name,
			Description: name + " description",
			Type:        "file",
			Path:        res.Path,
			Origin:      "upload",
			Revision:    1,
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

type stubFacade struct {
	basetesting.StubFacadeCaller

	apiResults map[string]params.ResourcesResult
	pendingIDs []string
}

func newStubFacade(c *gc.C, stub *testing.Stub) *stubFacade {
	s := &stubFacade{
		StubFacadeCaller: basetesting.StubFacadeCaller{
			Stub: stub,
		},
		apiResults: make(map[string]params.ResourcesResult),
	}

	s.FacadeCallFn = func(_ string, args, response interface{}) error {
		switch typedResponse := response.(type) {
		case *params.ResourcesResults:
			typedArgs, ok := args.(*params.ListResourcesArgs)
			c.Assert(ok, jc.IsTrue)

			for _, e := range typedArgs.Entities {
				tag, err := names.ParseTag(e.Tag)
				c.Assert(err, jc.ErrorIsNil)
				service := tag.Id()

				apiResult, ok := s.apiResults[service]
				if !ok {
					apiResult.Error = &params.Error{
						Message: fmt.Sprintf("application %q not found", service),
						Code:    params.CodeNotFound,
					}
				}
				typedResponse.Results = append(typedResponse.Results, apiResult)
			}
		case *params.AddPendingResourcesResult:
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
