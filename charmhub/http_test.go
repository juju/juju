// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"bytes"
	"context"
	"net/http"

	"github.com/golang/mock/gomock"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type APIRequesterSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&APIRequesterSuite{})

func (s *APIRequesterSuite) TestDo(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	req := MustNewRequest(c, "http://api.foo.bar")

	mockTransport := NewMockTransport(ctrl)
	mockTransport.EXPECT().Do(req).Return(emptyResponse(), nil)

	requester := NewAPIRequester(mockTransport)
	resp, err := requester.Do(req)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(resp.StatusCode, gc.Equals, http.StatusOK)
}

func (s *APIRequesterSuite) TestDoWithFailure(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	req := MustNewRequest(c, "http://api.foo.bar")

	mockTransport := NewMockTransport(ctrl)
	mockTransport.EXPECT().Do(req).Return(emptyResponse(), errors.Errorf("boom"))

	requester := NewAPIRequester(mockTransport)
	_, err := requester.Do(req)
	c.Assert(err, gc.Not(jc.ErrorIsNil))
}

func (s *APIRequesterSuite) TestDoWithInvalidContentType(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	req := MustNewRequest(c, "http://api.foo.bar")

	mockTransport := NewMockTransport(ctrl)
	mockTransport.EXPECT().Do(req).Return(invalidContentTypeResponse(), nil)

	requester := NewAPIRequester(mockTransport)
	_, err := requester.Do(req)
	c.Assert(err, gc.Not(jc.ErrorIsNil))
}

func (s *APIRequesterSuite) TestDoWithNotFoundResponse(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	req := MustNewRequest(c, "http://api.foo.bar")

	mockTransport := NewMockTransport(ctrl)
	mockTransport.EXPECT().Do(req).Return(notFoundResponse(), nil)

	requester := NewAPIRequester(mockTransport)
	resp, err := requester.Do(req)
	c.Assert(err, gc.Not(jc.ErrorIsNil))
	c.Assert(err, gc.ErrorMatches, "not-found")
	c.Assert(resp.StatusCode, gc.Equals, http.StatusNotFound)
}

type RESTSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&RESTSuite{})

func (s *RESTSuite) TestGet(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	var recievedURL string

	mockTransport := NewMockTransport(ctrl)
	mockTransport.EXPECT().Do(gomock.Any()).Do(func(req *http.Request) {
		recievedURL = req.URL.String()
	}).Return(emptyResponse(), nil)

	base := MustMakePath(c, "http://api.foo.bar")

	client := NewHTTPRESTClient(mockTransport, nil)

	var result interface{}
	err := client.Get(context.TODO(), base, &result)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(recievedURL, gc.Equals, "http://api.foo.bar")
}

func (s *RESTSuite) TestGetWithInvalidContext(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockTransport := NewMockTransport(ctrl)
	client := NewHTTPRESTClient(mockTransport, nil)

	base := MustMakePath(c, "http://api.foo.bar")

	var result interface{}
	err := client.Get(nil, base, &result)
	c.Assert(err, gc.Not(jc.ErrorIsNil))
}

func (s *RESTSuite) TestGetWithFailure(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockTransport := NewMockTransport(ctrl)
	mockTransport.EXPECT().Do(gomock.Any()).Return(emptyResponse(), errors.Errorf("boom"))

	client := NewHTTPRESTClient(mockTransport, nil)

	base := MustMakePath(c, "http://api.foo.bar")

	var result interface{}
	err := client.Get(context.TODO(), base, &result)
	c.Assert(err, gc.Not(jc.ErrorIsNil))
}

func (s *RESTSuite) TestGetWithUnmarshalFailure(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockTransport := NewMockTransport(ctrl)
	mockTransport.EXPECT().Do(gomock.Any()).Return(invalidResponse(), nil)

	client := NewHTTPRESTClient(mockTransport, nil)

	base := MustMakePath(c, "http://api.foo.bar")

	var result interface{}
	err := client.Get(context.TODO(), base, &result)
	c.Assert(err, gc.Not(jc.ErrorIsNil))
}

func emptyResponse() *http.Response {
	return &http.Response{
		Header:     MakeContentTypeHeader("application/json"),
		StatusCode: http.StatusOK,
		Body:       MakeNopCloser(bytes.NewBufferString("{}")),
	}
}

func invalidResponse() *http.Response {
	return &http.Response{
		Header:     MakeContentTypeHeader("application/json"),
		StatusCode: http.StatusOK,
		Body:       MakeNopCloser(bytes.NewBufferString("/\\!")),
	}
}

func invalidContentTypeResponse() *http.Response {
	return &http.Response{
		Header:     MakeContentTypeHeader("text/plain"),
		StatusCode: http.StatusNotFound,
		Body:       MakeNopCloser(bytes.NewBufferString("")),
	}
}

func notFoundResponse() *http.Response {
	return &http.Response{
		Header:     MakeContentTypeHeader("application/json"),
		StatusCode: http.StatusNotFound,
		Body: MakeNopCloser(bytes.NewBufferString(`
{
	"code":"404",
	"message":"not-found"
}
		`)),
	}
}
