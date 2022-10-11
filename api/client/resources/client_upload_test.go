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

	"github.com/golang/mock/gomock"
	"github.com/juju/charm/v9"
	charmresource "github.com/juju/charm/v9/resource"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	"github.com/kr/pretty"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/client/resources"
	apicharm "github.com/juju/juju/api/common/charm"
	"github.com/juju/juju/core/series"
	"github.com/juju/juju/rpc/params"
)

var _ = gc.Suite(&UploadSuite{})

type UploadSuite struct {
	BaseSuite
}

func (s *UploadSuite) TestUpload(c *gc.C) {
	defer s.setUpMocks(c).Finish()

	ctx := context.TODO()
	s.apiCaller.EXPECT().Context().Return(ctx)

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

	s.httpClient.EXPECT().Do(ctx, reqMatcher{c, req}, gomock.Any())

	err = s.client.Upload("a-application", "spam", "foo.zip", strings.NewReader(data))
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
	defer s.setUpMocks(c).Finish()

	err := s.client.Upload("???", "spam", "file.zip", nil)
	c.Check(err, gc.ErrorMatches, `.*invalid application.*`)
}

func (s *UploadSuite) TestUploadFailed(c *gc.C) {
	defer s.setUpMocks(c).Finish()

	ctx := context.TODO()
	s.apiCaller.EXPECT().Context().Return(ctx)

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

	s.httpClient.EXPECT().Do(ctx, reqMatcher{c, req}, gomock.Any()).Return(errors.New("boom"))

	err = s.client.Upload("a-application", "spam", "foo.zip", strings.NewReader(data))
	c.Assert(err, gc.ErrorMatches, "boom")
}

func (s *UploadSuite) TestAddPendingResources(c *gc.C) {
	defer s.setUpMocks(c).Finish()

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
	uuid, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	expected := []string{uuid.String()}
	resultParams := params.AddPendingResourcesResult{
		PendingIDs: expected,
	}
	s.facade.EXPECT().FacadeCall("AddPendingResources", &args, gomock.Any()).SetArg(2, resultParams).Return(nil)

	cURL := charm.MustParseURL("ch:spam")
	pendingIDs, err := s.client.AddPendingResources(resources.AddPendingResourcesArgs{
		ApplicationID: "a-application",
		CharmID: resources.CharmID{
			URL: cURL,
			Origin: apicharm.Origin{
				Source:       apicharm.OriginCharmHub,
				ID:           "id",
				Risk:         "stable",
				Base:         series.MakeDefaultBase("ubuntu", "22.04"),
				Architecture: "arm64",
			},
		},
		Resources: []charmresource.Resource{res[0].Resource},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(pendingIDs, jc.DeepEquals, expected)
}

func (s *UploadSuite) TestUploadPendingResource(c *gc.C) {
	defer s.setUpMocks(c).Finish()

	res, apiResult := newResourceResult(c, "spam")
	args := params.AddPendingResourcesArgsV2{
		Entity:    params.Entity{Tag: "application-a-application"},
		Resources: []params.CharmResource{apiResult.Resources[0].CharmResource},
	}
	uuid, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	expected := uuid.String()
	resultParams := params.AddPendingResourcesResult{
		PendingIDs: []string{expected},
	}
	s.facade.EXPECT().FacadeCall("AddPendingResources", &args, gomock.Any()).SetArg(2, resultParams).Return(nil)

	ctx := context.TODO()
	s.apiCaller.EXPECT().Context().Return(ctx)

	data := "<data>"
	fp, err := charmresource.GenerateFingerprint(strings.NewReader(data))
	c.Assert(err, jc.ErrorIsNil)
	req, err := http.NewRequest("PUT", "/applications/a-application/resources/spam", strings.NewReader(data))
	c.Assert(err, jc.ErrorIsNil)
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Content-SHA384", fp.String())
	req.Header.Set("Content-Length", fmt.Sprint(len(data)))
	req.ContentLength = int64(len(data))
	req.URL.RawQuery = "pendingid=" + expected
	req.Header.Set("Content-Disposition", "form-data; filename=file.zip")

	s.httpClient.EXPECT().Do(ctx, reqMatcher{c, req}, gomock.Any())

	uploadID, err := s.client.UploadPendingResource("a-application", res[0].Resource, "file.zip", strings.NewReader(data))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(uploadID, gc.Equals, expected)
}

func (s *UploadSuite) TestUploadPendingResourceNoFile(c *gc.C) {
	defer s.setUpMocks(c).Finish()

	res, apiResult := newResourceResult(c, "spam")
	args := params.AddPendingResourcesArgsV2{
		Entity:    params.Entity{Tag: "application-a-application"},
		Resources: []params.CharmResource{apiResult.Resources[0].CharmResource},
	}
	uuid, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	expected := uuid.String()
	resultParams := params.AddPendingResourcesResult{
		PendingIDs: []string{expected},
	}
	s.facade.EXPECT().FacadeCall("AddPendingResources", &args, gomock.Any()).SetArg(2, resultParams).Return(nil)

	uploadID, err := s.client.UploadPendingResource("a-application", res[0].Resource, "file.zip", nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(uploadID, gc.Equals, expected)
}

func (s *UploadSuite) TestUploadPendingResourceBadApplication(c *gc.C) {
	defer s.setUpMocks(c).Finish()

	res, _ := newResourceResult(c, "spam")
	_, err := s.client.UploadPendingResource("???", res[0].Resource, "file.zip", nil)
	c.Assert(err, gc.ErrorMatches, `.*invalid application.*`)
}

func (s *UploadSuite) TestUploadPendingResourceFailed(c *gc.C) {
	defer s.setUpMocks(c).Finish()

	res, apiResult := newResourceResult(c, "spam")
	args := params.AddPendingResourcesArgsV2{
		Entity:    params.Entity{Tag: "application-a-application"},
		Resources: []params.CharmResource{apiResult.Resources[0].CharmResource},
	}
	uuid, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	expected := uuid.String()
	resultParams := params.AddPendingResourcesResult{
		PendingIDs: []string{expected},
	}
	s.facade.EXPECT().FacadeCall("AddPendingResources", &args, gomock.Any()).SetArg(2, resultParams).Return(nil)

	ctx := context.TODO()
	s.apiCaller.EXPECT().Context().Return(ctx)

	data := "<data>"
	fp, err := charmresource.GenerateFingerprint(strings.NewReader(data))
	c.Assert(err, jc.ErrorIsNil)
	req, err := http.NewRequest("PUT", "/applications/a-application/resources/spam", strings.NewReader(data))
	c.Assert(err, jc.ErrorIsNil)
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Content-SHA384", fp.String())
	req.Header.Set("Content-Length", fmt.Sprint(len(data)))
	req.ContentLength = int64(len(data))
	req.URL.RawQuery = "pendingid=" + expected
	req.Header.Set("Content-Disposition", "form-data; filename=file.zip")

	s.httpClient.EXPECT().Do(ctx, reqMatcher{c, req}, gomock.Any()).Return(errors.New("boom"))

	_, err = s.client.UploadPendingResource("a-application", res[0].Resource, "file.zip", strings.NewReader(data))
	c.Assert(err, gc.ErrorMatches, "boom")
}
