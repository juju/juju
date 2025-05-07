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
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"
	"github.com/kr/pretty"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/api/base/mocks"
	"github.com/juju/juju/api/client/resources"
	apicharm "github.com/juju/juju/api/common/charm"
	httpmocks "github.com/juju/juju/api/http/mocks"
	corebase "github.com/juju/juju/core/base"
	coreresources "github.com/juju/juju/core/resource"
	resourcetesting "github.com/juju/juju/core/resource/testing"
	charmresource "github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/uuid"
	"github.com/juju/juju/rpc/params"
)

var _ = tc.Suite(&UploadSuite{})

type UploadSuite struct {
	mockHTTPClient   *httpmocks.MockHTTPDoer
	mockAPICaller    *mocks.MockAPICallCloser
	mockFacadeCaller *mocks.MockFacadeCaller
	client           *resources.Client
}

func (s *UploadSuite) setup(c *tc.C) *gomock.Controller {
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

func (s *UploadSuite) TestUpload(c *tc.C) {
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
	c   *tc.C
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

func (s *UploadSuite) TestUploadBadApplication(c *tc.C) {
	defer s.setup(c).Finish()

	err := s.client.Upload(context.Background(), "???", "spam", "file.zip", "", nil)
	c.Check(err, tc.ErrorMatches, `.*invalid application.*`)
}

func (s *UploadSuite) TestUploadFailed(c *tc.C) {
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
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *UploadSuite) TestAddPendingResources(c *tc.C) {
	defer s.setup(c).Finish()

	res, apiResult := newResourceResult(c, "spam")
	addArgs := newAddPendingResourcesArgsV2(apiResult)
	uuid, err := uuid.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	expected := []string{uuid.String()}
	result := new(params.AddPendingResourcesResult)
	results := params.AddPendingResourcesResult{
		PendingIDs: expected,
	}
	s.mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "AddPendingResources", &addArgs, result).SetArg(3, results).Return(nil)

	cURL := "ch:spam"
	pendingIDs, err := s.client.AddPendingResources(context.Background(),
		resources.AddPendingResourcesArgs{
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

func (s *UploadSuite) TestUploadPendingResource(c *tc.C) {
	defer s.setup(c).Finish()

	res, apiResult := newResourceResult(c, "spam")
	addArgs := newAddPendingResourcesArgsV2(apiResult)
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
	s.mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "AddPendingResources", &addArgs, gomock.Any()).SetArg(3, results).Return(nil)
	s.mockHTTPClient.EXPECT().Do(ctx, reqMatcher{c, req}, gomock.Any())

	uploadArgs := newUploadPendingResourceArgs(res, data)
	uploadID, err := s.client.UploadPendingResource(ctx, uploadArgs)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(uploadID, tc.Equals, expected)
}

func (s *UploadSuite) TestUploadPendingResourceNoFile(c *tc.C) {
	defer s.setup(c).Finish()

	res, apiResult := newResourceResult(c, "spam")
	addArgs := newAddPendingResourcesArgsV2(apiResult)
	uuid, err := uuid.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	expected := uuid.String()
	results := params.AddPendingResourcesResult{
		PendingIDs: []string{expected},
	}
	s.mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "AddPendingResources", &addArgs, gomock.Any()).SetArg(3, results).Return(nil)

	uploadArgs := newUploadPendingResourceArgsNoData(res)
	uploadID, err := s.client.UploadPendingResource(context.Background(), uploadArgs)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(uploadID, tc.Equals, expected)
}

func (s *UploadSuite) TestUploadPendingResourceBadApplication(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	_, err := s.client.UploadPendingResource(context.Background(), resources.UploadPendingResourceArgs{})
	c.Assert(err, tc.ErrorMatches, `.*invalid application.*`)
}

func (s *UploadSuite) TestUploadPendingResourceFailed(c *tc.C) {
	defer s.setup(c).Finish()

	res, apiResult := newResourceResult(c, "spam")
	addArgs := newAddPendingResourcesArgsV2(apiResult)
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
	s.mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "AddPendingResources", &addArgs, gomock.Any()).SetArg(3, results).Return(nil)
	s.mockHTTPClient.EXPECT().Do(ctx, reqMatcher{c, req}, gomock.Any()).Return(errors.New("boom"))

	uploadArgs := newUploadPendingResourceArgs(res, data)
	_, err = s.client.UploadPendingResource(ctx, uploadArgs)
	c.Assert(err, tc.ErrorMatches, "boom")
}

func newAddPendingResourcesArgsV2(apiResult params.ResourcesResult) params.AddPendingResourcesArgsV2 {
	return params.AddPendingResourcesArgsV2{
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
}

func newUploadPendingResourceArgs(res []coreresources.Resource, data string) resources.UploadPendingResourceArgs {
	args := newUploadPendingResourceArgsNoData(res)
	args.Reader = strings.NewReader(data)
	return args
}

func newUploadPendingResourceArgsNoData(res []coreresources.Resource) resources.UploadPendingResourceArgs {
	return resources.UploadPendingResourceArgs{
		ApplicationID: "a-application",
		CharmID: resources.CharmID{
			URL: "ch:spam",
			Origin: apicharm.Origin{
				Source:       apicharm.OriginCharmHub,
				ID:           "id",
				Risk:         "stable",
				Base:         corebase.MakeDefaultBase("ubuntu", "22.04"),
				Architecture: "arm64",
			},
		},
		Resource: res[0].Resource,
		Filename: "file.zip",
	}
}

func newResourceResult(c *tc.C, names ...string) ([]coreresources.Resource, params.ResourcesResult) {
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

func newResource(c *tc.C, name, username, data string) (coreresources.Resource, params.Resource) {
	opened := resourcetesting.NewResource(c, nil, name, "a-application", data)
	res := opened.Resource
	res.Revision = 1
	res.RetrievedBy = username
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
		UUID:            res.UUID.String(),
		ApplicationName: res.ApplicationName,
		Username:        username,
		Timestamp:       res.Timestamp,
	}

	return res, apiRes
}
