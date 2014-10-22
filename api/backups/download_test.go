// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"io/ioutil"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/backups"
)

type downloadSuite struct {
	baseSuite
}

var _ = gc.Suite(&downloadSuite{})

func (s *downloadSuite) TestSuccessfulRequest(c *gc.C) {
	backups.SetHTTP(s.client, &fakeHTTPCaller{Data: "<compressed archive data>"})

	resultArchive, err := s.client.Download("spam")
	c.Assert(err, gc.IsNil)

	resultData, err := ioutil.ReadAll(resultArchive)
	c.Assert(err, gc.IsNil)
	c.Check(string(resultData), gc.Equals, "<compressed archive data>")
}
