// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	gc "gopkg.in/check.v1"
)

type resourcesUploadSuite struct {
}

var _ = gc.Suite(&resourcesUploadSuite{})

func (s *resourcesUploadSuite) TestStub(c *gc.C) {
	c.Skip("This suite is missing tests for the following scenarios:\n" +
		"- Returns a http.StatusMethodNotAllowed, when no resource provided to GET/PUT\n" +
		"- Sending a POST req requires authorization via unit or application only.\n" +
		"- Rejects an unknown model with http.StatusNotFound.\n" +
		"- Upload of an application resource.\n" +
		"- Upload of an unit resource.\n" +
		"- Test argument validation\n" +
		"- Test fails when model not importing.")
}
