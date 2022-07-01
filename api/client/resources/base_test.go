// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources_test

import (
	"time"

	"github.com/golang/mock/gomock"
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/v2/api/base/mocks"
	"github.com/juju/juju/v2/api/client/resources"
	httpmocks "github.com/juju/juju/v2/api/http/mocks"
	coreresources "github.com/juju/juju/v2/core/resources"
	resourcetesting "github.com/juju/juju/v2/core/resources/testing"
	"github.com/juju/juju/v2/rpc/params"
)

type BaseSuite struct {
	testing.IsolationSuite

	facade     *mocks.MockFacadeCaller
	apiCaller  *mocks.MockAPICallCloser
	httpClient *httpmocks.MockHTTPDoer
	client     *resources.Client
}

func (s *BaseSuite) TearDownTest(c *gc.C) {
	s.facade = nil
	s.IsolationSuite.TearDownTest(c)
}

func (s *BaseSuite) setUpMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.httpClient = httpmocks.NewMockHTTPDoer(ctrl)
	s.apiCaller = mocks.NewMockAPICallCloser(ctrl)
	s.apiCaller.EXPECT().BestFacadeVersion(gomock.Any()).Return(3).AnyTimes()

	s.facade = mocks.NewMockFacadeCaller(ctrl)
	s.facade.EXPECT().RawAPICaller().Return(s.apiCaller).AnyTimes()
	s.facade.EXPECT().BestAPIVersion().Return(2).AnyTimes()
	s.client = resources.NewClientForTest(s.facade, s.httpClient)
	return ctrl
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
