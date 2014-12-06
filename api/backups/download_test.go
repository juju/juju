// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"io/ioutil"
	"net/http"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	apiserverhttp "github.com/juju/juju/apiserver/http"
	"github.com/juju/juju/apiserver/params"
)

type downloadSuite struct {
	httpSuite
}

var _ = gc.Suite(&downloadSuite{})

func (s *downloadSuite) setSuccess(c *gc.C, data string) {
	body := []byte(data)
	s.setResponse(c, http.StatusOK, body, apiserverhttp.CTypeRaw)
}

func (s *downloadSuite) TestSuccessfulRequest(c *gc.C) {
	s.setSuccess(c, "<compressed archive data>")

	resultArchive, err := s.client.Download("spam")
	c.Assert(err, jc.ErrorIsNil)

	resultData, err := ioutil.ReadAll(resultArchive)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(string(resultData), gc.Equals, "<compressed archive data>")
}

func (s *downloadSuite) TestFailedRequest(c *gc.C) {
	s.setFailure(c, "something went wrong!", http.StatusInternalServerError)

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
