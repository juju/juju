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

	apihttp "github.com/juju/juju/v3/api/http"
	"github.com/juju/juju/v3/api/http/mocks"
)

type httpSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&httpSuite{})

type uriMatcher struct {
	expectedURL string
}

func (m uriMatcher) Matches(x interface{}) bool {
	req, ok := x.(*http.Request)
	if !ok {
		return false
	}
	if req.URL.String() != m.expectedURL {
		return false
	}
	return true
}

func (uriMatcher) String() string {
	return "matches charm upload requests"
}

func (s *httpSuite) TestOpenURI(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockHttp := mocks.NewMockHTTPDoer(ctrl)
	resultResp := &http.Response{
		StatusCode: 200,
		Header:     make(http.Header),
		Body:       ioutil.NopCloser(strings.NewReader("2.6.6-ubuntu-amd64")),
	}
	resultResp.Header.Add("Content-Type", "application/json")
	ctx := context.TODO()
	mockHttp.EXPECT().Do(ctx, &uriMatcher{"/tools/2.6.6-ubuntu-amd64"}, gomock.Any()).DoAndReturn(func(_ context.Context, _ *http.Request, resp interface{}) error {
		out := resp.(**http.Response)
		*out = resultResp
		return nil
	})

	reader, err := apihttp.OpenURI(ctx, mockHttp, "/tools/2.6.6-ubuntu-amd64", nil)
	c.Assert(err, jc.ErrorIsNil)
	defer reader.Close()

	// The fake tools content will be the version number.
	content, err := ioutil.ReadAll(reader)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(content), gc.Equals, "2.6.6-ubuntu-amd64")
}
