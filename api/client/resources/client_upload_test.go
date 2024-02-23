// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"time"

	"github.com/juju/errors"
	charmresource "github.com/juju/juju/charm/resource"
	jc "github.com/juju/testing/checkers"
	"github.com/kr/pretty"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base/mocks"
	"github.com/juju/juju/api/client/resources"
	apicharm "github.com/juju/juju/api/common/charm"
	httpmocks "github.com/juju/juju/api/http/mocks"
	corebase "github.com/juju/juju/core/base"
	coreresources "github.com/juju/juju/core/resources"
	resourcetesting "github.com/juju/juju/core/resources/testing"
	"github.com/juju/juju/internal/uuid"
	"github.com/juju/juju/rpc/params"
)

var _ = gc.Suite(&UploadSuite{})

type UploadSuite struct {
	mockHTTPClient   *httpmocks.MockHTTPDoer
	mockAPICaller    *mocks.MockAPICallCloser
	mockFacadeCaller *mocks.MockFacadeCaller
	client           *resources.Client
}

func (s *UploadSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.mockHTTPClient = httpmocks.NewMockHTTPDoer(ctrl)
	s.mockAPICaller = mocks.NewMockAPICallCloser(ctrl)
	s.mockAPICaller.EXPECT().BestFacadeVersion(gomock.Any()).Return(3).AnyTimes()

	s.mockFacadeCaller = mocks.NewMockFacadeCaller(ctrl)
	s.mockFacadeCaller.EXPECT().RawAPICaller().Return(s.mockAPICaller).AnyTimes()
	s.mockFacadeCaller.EXPECT().BestAPIVersion().Return(2).AnyTimes()
	s.client = resources.NewClientForTest(s.mockFacadeCaller, s.mockHTTPClient)
	return ctrl
}

func (s *UploadSuite) TestUpload(c *gc.C) {
	defer s.setup(c).Finish()

	ctx := context.Background()

	data := "<data>"
	fp, err := charmresource.GenerateFingerprint(strings.NewReader(data))
	c.Assert(err, jc.ErrorIsNil)
	req, err := http.NewRequest("PUT", "/applications/a-application/resources/spam", strings.NewReader(data))
	c.Assert(err, jc.ErrorIsNil)
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Content-SHA384", fp.String())
	req.Header.Set("Content-Length", fmt.Sprint(len(data)))
	req.Header.Set("Content-Disposition", "form-data; filename=foo.zip")
	req.ContentLength = int64(len(data))

	s.mockHTTPClient.EXPECT().Do(ctx, reqMatcher{c, req}, gomock.Any())

	err = s.client.Upload(context.Background(), "a-application", "spam", "foo.zip", "", strings.NewReader(data))
	c.Assert(err, jc.ErrorIsNil)
}

type reqMatcher struct {
	c   *gc.C
	req *http.Request
}

func (m reqMatcher) Matches(x interface{}) bool {
	obtained, ok := x.(*http.Request)
	if !ok {
		return false
	}
	obtainedCopy := *obtained
	obtainedBody, err := io.ReadAll(obtainedCopy.Body)
	m.c.Assert(err, jc.ErrorIsNil)
	obtainedCopy.Body = nil
	obtainedCopy.GetBody = nil

	reqCopy := *m.req
	reqBody, err := io.ReadAll(reqCopy.Body)
	m.c.Assert(err, jc.ErrorIsNil)
	reqCopy.Body = nil
	reqCopy.GetBody = nil
	if string(obtainedBody) != string(reqBody) {
		return false
	}
	return reflect.DeepEqual(reqCopy, obtainedCopy)
}

func (m reqMatcher) String() string {
	return pretty.Sprint(m.req)
}

func (s *UploadSuite) TestUploadBadApplication(c *gc.C) {
	defer s.setup(c).Finish()

	err := s.client.Upload(context.Background(), "???", "spam", "file.zip", "", nil)
	c.Check(err, gc.ErrorMatches, `.*invalid application.*`)
}

func (s *UploadSuite) TestUploadFailed(c *gc.C) {
	defer s.setup(c).Finish()

	data := "<data>"
	fp, err := charmresource.GenerateFingerprint(strings.NewReader(data))
	c.Assert(err, jc.ErrorIsNil)
	req, err := http.NewRequest("PUT", "/applications/a-application/resources/spam", strings.NewReader(data))
	c.Assert(err, jc.ErrorIsNil)
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Content-SHA384", fp.String())
	req.Header.Set("Content-Length", fmt.Sprint(len(data)))
	req.Header.Set("Content-Disposition", "form-data; filename=foo.zip")
	req.ContentLength = int64(len(data))

	ctx := context.Background()
	s.mockHTTPClient.EXPECT().Do(ctx, reqMatcher{c, req}, gomock.Any()).Return(errors.New("boom"))
	err = s.client.Upload(ctx, "a-application", "spam", "foo.zip", "", strings.NewReader(data))
	c.Assert(err, gc.ErrorMatches, "boom")
}

func (s *UploadSuite) TestAddPendingResources(c *gc.C) {
	defer s.setup(c).Finish()

	res, apiResult := newResourceResult(c, "spam")
	args := params.AddPendingResourcesArgsV2{
		Entity: params.Entity{Tag: "application-a-application"},
		URL:    "ch:spam",
		CharmOrigin: params.CharmOrigin{
			Source:       "charm-hub",
			ID:           "id",
			Risk:         "stable",
			Base:         params.Base{Name: "ubuntu", Channel: "22.04/stable"},
			Architecture: "arm64",
		},
		Resources: []params.CharmResource{apiResult.Resources[0].CharmResource},
	}
	uuid, err := uuid.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	expected := []string{uuid.String()}
	result := new(params.AddPendingResourcesResult)
	results := params.AddPendingResourcesResult{
		PendingIDs: expected,
	}
	s.mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "AddPendingResources", &args, result).SetArg(3, results).Return(nil)

	cURL := "ch:spam"
	pendingIDs, err := s.client.AddPendingResources(resources.AddPendingResourcesArgs{
		ApplicationID: "a-application",
		CharmID: resources.CharmID{
			URL: cURL,
			Origin: apicharm.Origin{
				Source:       apicharm.OriginCharmHub,
				ID:           "id",
				Risk:         "stable",
				Base:         corebase.MakeDefaultBase("ubuntu", "22.04"),
				Architecture: "arm64",
			},
		},
		Resources: []charmresource.Resource{res[0].Resource},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(pendingIDs, jc.DeepEquals, expected)
}

func (s *UploadSuite) TestUploadPendingResource(c *gc.C) {
	defer s.setup(c).Finish()

	res, apiResult := newResourceResult(c, "spam")
	args := params.AddPendingResourcesArgsV2{
		Entity:    params.Entity{Tag: "application-a-application"},
		Resources: []params.CharmResource{apiResult.Resources[0].CharmResource},
	}
	uuid, err := uuid.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	expected := uuid.String()
	results := params.AddPendingResourcesResult{
		PendingIDs: []string{expected},
	}
	data := "<data>"
	fp, err := charmresource.GenerateFingerprint(strings.NewReader(data))
	c.Assert(err, jc.ErrorIsNil)

	url := fmt.Sprintf("/applications/a-application/resources/spam?pendingid=%v", expected)
	req, err := http.NewRequest("PUT", url, strings.NewReader(data))
	c.Assert(err, jc.ErrorIsNil)
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Content-SHA384", fp.String())
	req.Header.Set("Content-Length", fmt.Sprint(len(data)))
	req.ContentLength = int64(len(data))
	req.Header.Set("Content-Disposition", "form-data; filename=file.zip")

	ctx := context.Background()
	s.mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "AddPendingResources", &args, gomock.Any()).SetArg(3, results).Return(nil)
	s.mockHTTPClient.EXPECT().Do(ctx, reqMatcher{c, req}, gomock.Any())

	uploadID, err := s.client.UploadPendingResource(ctx, "a-application", res[0].Resource, "file.zip", strings.NewReader(data))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(uploadID, gc.Equals, expected)
}

func (s *UploadSuite) TestUploadPendingResourceNoFile(c *gc.C) {
	defer s.setup(c).Finish()

	res, apiResult := newResourceResult(c, "spam")
	args := params.AddPendingResourcesArgsV2{
		Entity:    params.Entity{Tag: "application-a-application"},
		Resources: []params.CharmResource{apiResult.Resources[0].CharmResource},
	}
	uuid, err := uuid.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	expected := uuid.String()
	results := params.AddPendingResourcesResult{
		PendingIDs: []string{expected},
	}
	s.mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "AddPendingResources", &args, gomock.Any()).SetArg(3, results).Return(nil)

	uploadID, err := s.client.UploadPendingResource(context.Background(), "a-application", res[0].Resource, "file.zip", nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(uploadID, gc.Equals, expected)
}

func (s *UploadSuite) TestUploadPendingResourceBadApplication(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	res, _ := newResourceResult(c, "spam")
	_, err := s.client.UploadPendingResource(context.Background(), "???", res[0].Resource, "file.zip", nil)
	c.Assert(err, gc.ErrorMatches, `.*invalid application.*`)
}

func (s *UploadSuite) TestUploadPendingResourceFailed(c *gc.C) {
	defer s.setup(c).Finish()

	res, apiResult := newResourceResult(c, "spam")
	args := params.AddPendingResourcesArgsV2{
		Entity:    params.Entity{Tag: "application-a-application"},
		Resources: []params.CharmResource{apiResult.Resources[0].CharmResource},
	}
	uuid, err := uuid.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	expected := uuid.String()
	results := params.AddPendingResourcesResult{
		PendingIDs: []string{expected},
	}
	data := "<data>"
	fp, err := charmresource.GenerateFingerprint(strings.NewReader(data))
	c.Assert(err, jc.ErrorIsNil)
	url := fmt.Sprintf("/applications/a-application/resources/spam?pendingid=%v", expected)
	req, err := http.NewRequest("PUT", url, strings.NewReader(data))
	c.Assert(err, jc.ErrorIsNil)
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Content-SHA384", fp.String())
	req.Header.Set("Content-Length", fmt.Sprint(len(data)))
	req.ContentLength = int64(len(data))
	req.Header.Set("Content-Disposition", "form-data; filename=file.zip")

	ctx := context.Background()
	s.mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "AddPendingResources", &args, gomock.Any()).SetArg(3, results).Return(nil)
	s.mockHTTPClient.EXPECT().Do(ctx, reqMatcher{c, req}, gomock.Any()).Return(errors.New("boom"))

	_, err = s.client.UploadPendingResource(ctx, "a-application", res[0].Resource, "file.zip", strings.NewReader(data))
	c.Assert(err, gc.ErrorMatches, "boom")
}

func newResourceResult(c *gc.C, names ...string) ([]coreresources.Resource, params.ResourcesResult) {
	var res []coreresources.Resource
	var apiResult params.ResourcesResult
	for _, name := range names {
		data := name + "...spamspamspam"
		newRes, apiRes := newResource(c, name, "a-user", data)
		res = append(res, newRes)
		apiResult.Resources = append(apiResult.Resources, apiRes)
	}
	return res, apiResult
}

func newResource(c *gc.C, name, username, data string) (coreresources.Resource, params.Resource) {
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
