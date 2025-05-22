// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package http_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	stdtesting "testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	apihttp "github.com/juju/juju/api/http"
	"github.com/juju/juju/api/http/mocks"
	"github.com/juju/juju/internal/testhelpers"
)

type httpSuite struct {
	testhelpers.IsolationSuite
}

func TestHttpSuite(t *stdtesting.T) {
	tc.Run(t, &httpSuite{})
}

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

func (s *httpSuite) TestOpenURI(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockHttp := mocks.NewMockHTTPDoer(ctrl)
	resultResp := &http.Response{
		StatusCode: 200,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader("2.6.6-ubuntu-amd64")),
	}
	resultResp.Header.Add("Content-Type", "application/json")
	mockHttp.EXPECT().Do(gomock.Any(), &uriMatcher{"/tools/2.6.6-ubuntu-amd64"}, gomock.Any()).DoAndReturn(func(_ context.Context, _ *http.Request, resp interface{}) error {
		out := resp.(**http.Response)
		*out = resultResp
		return nil
	})

	reader, err := apihttp.OpenURI(c.Context(), mockHttp, "/tools/2.6.6-ubuntu-amd64", nil)
	c.Assert(err, tc.ErrorIsNil)
	defer reader.Close()

	// The fake tools content will be the version number.
	content, err := io.ReadAll(reader)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(string(content), tc.Equals, "2.6.6-ubuntu-amd64")
}
