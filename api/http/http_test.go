// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package http_test

import (
	"context"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/golang/mock/gomock"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/httprequest.v1"

	basemocks "github.com/juju/juju/api/base/mocks"
	apihttp "github.com/juju/juju/api/http"
	"github.com/juju/juju/api/http/mocks"
)

type httpSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&httpSuite{})

func (s *httpSuite) TestOpenURI(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	apiCaller := basemocks.NewMockAPICaller(ctrl)
	mockHttp := mocks.NewMockHTTPClient(ctrl)
	reqClient := &httprequest.Client{
		BaseURL: "http://somewhere.invalid",
		Doer:    mockHttp,
	}
	apiCaller.EXPECT().Context().Return(context.TODO())
	apiCaller.EXPECT().HTTPClient().Return(reqClient, nil)
	resp := &http.Response{
		StatusCode: 200,
		Header:     make(http.Header),
		Body:       ioutil.NopCloser(strings.NewReader("2.6.6-ubuntu-amd64")),
	}
	resp.Header.Add("Content-Type", "application/json")
	mockHttp.EXPECT().Do(gomock.Any()).DoAndReturn(func(r *http.Request) (*http.Response, error) {
		return resp, nil
	})

	reader, err := apihttp.OpenURI(apiCaller, "/tools/2.6.6-ubuntu-amd64", nil)
	c.Assert(err, jc.ErrorIsNil)
	defer reader.Close()

	// The fake tools content will be the version number.
	content, err := ioutil.ReadAll(reader)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(content), gc.Equals, "2.6.6-ubuntu-amd64")
}
