// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"

	"github.com/juju/errors"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/backups"
	httptesting "github.com/juju/juju/api/http/testing"
	"github.com/juju/juju/apiserver/params"
)

type downloadSuite struct {
	baseSuite
	httpClient httptesting.FakeClient
}

var _ = gc.Suite(&downloadSuite{})

func (s *downloadSuite) setResponse(c *gc.C, status int, data []byte, ctype string) {
	resp := http.Response{
		StatusCode: status,
		Header:     make(http.Header),
	}

	resp.Header.Set("Content-Type", ctype)
	resp.Body = ioutil.NopCloser(bytes.NewBuffer(data))

	s.httpClient.Response = &resp
	backups.SetHTTP(s.client, &s.httpClient)
}

func (s *downloadSuite) setSuccess(c *gc.C, data string) {
	body := []byte(data)
	s.setResponse(c, http.StatusOK, body, "application/octet-stream")
}

func (s *downloadSuite) setFailure(c *gc.C, msg string, status int) {
	if status < 0 {
		status = http.StatusInternalServerError
	}

	failure := params.Error{
		Message: msg,
	}
	data, err := json.Marshal(&failure)
	c.Assert(err, gc.IsNil)

	s.setResponse(c, status, data, "application/json")
}

func (s *downloadSuite) setError(c *gc.C, msg string, status int) {
	if status < 0 {
		status = http.StatusInternalServerError
	}

	data := []byte(msg)
	s.setResponse(c, status, data, "application/octet-stream")
}

func (s *downloadSuite) TestSuccessfulRequest(c *gc.C) {
	s.setSuccess(c, "<compressed archive data>")

	resultArchive, err := s.client.Download("spam")
	c.Assert(err, gc.IsNil)

	resultData, err := ioutil.ReadAll(resultArchive)
	c.Assert(err, gc.IsNil)
	c.Check(string(resultData), gc.Equals, "<compressed archive data>")
}

func (s *downloadSuite) TestFailedRequest(c *gc.C) {
	s.setFailure(c, "something went wrong!", -1)

	_, err := s.client.Download("spam")

	c.Check(errors.Cause(err), gc.FitsTypeOf, &params.Error{})
	c.Check(err, gc.ErrorMatches, "something went wrong!")
}

func (s *downloadSuite) TestErrorRequest(c *gc.C) {
	s.setError(c, "something went wrong!", -1)

	_, err := s.client.Download("spam")

	c.Check(errors.Cause(err), gc.FitsTypeOf, &params.Error{})
	c.Check(err, gc.ErrorMatches, "something went wrong!")
}
