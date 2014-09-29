// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"bytes"
	"io/ioutil"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/backups"
	"github.com/juju/juju/apiserver/params"
)

type downloadSuite struct {
	baseSuite
}

var _ = gc.Suite(&downloadSuite{})

func (s *downloadSuite) TestInfo(c *gc.C) {
	archive := bytes.NewBufferString("<compressed archive data>")
	cleanup := backups.PatchClientFacadeCall(s.client,
		func(req string, paramsIn interface{}, resp interface{}) error {
			c.Check(req, gc.Equals, "DownloadDirect")

			c.Assert(paramsIn, gc.FitsTypeOf, params.BackupsDownloadArgs{})
			p := paramsIn.(params.BackupsDownloadArgs)
			c.Check(p.ID, gc.Equals, "spam")

			if result, ok := resp.(*params.BackupsDownloadResult); ok {
				result.ID = p.ID
				result.Archive = ioutil.NopCloser(archive)
			} else {
				c.Fatalf("wrong output structure")
			}
			return nil
		},
	)
	defer cleanup()

	result, err := s.client.Download("spam")
	c.Assert(err, gc.IsNil)

	c.Check(result.ID, gc.Equals, "spam")

	data, err := ioutil.ReadAll(result.Archive)
	c.Assert(err, gc.IsNil)
	c.Check(string(data), gc.Equals, "<compressed archive data>")
}
