// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"bytes"
	"io/ioutil"

	gc "launchpad.net/gocheck"

	"github.com/juju/juju/apiserver/params"
)

func (s *backupsSuite) TestListOkay(c *gc.C) {
	impl := s.setBackups(c, s.meta, "")
	impl.archive = ioutil.NopCloser(bytes.NewBufferString("spamspamspam"))
	args := params.BackupsListArgs{}
	result, err := s.api.List(args)
	c.Assert(err, gc.IsNil)
	item := params.BackupsMetadataResult{}
	item.UpdateFromMetadata(s.meta)
	expected := params.BackupsListResult{
		List: []params.BackupsMetadataResult{item},
	}

	c.Check(result, gc.DeepEquals, expected)
}

func (s *backupsSuite) TestListError(c *gc.C) {
	s.setBackups(c, nil, "failed!")
	args := params.BackupsListArgs{}
	_, err := s.api.List(args)

	c.Check(err, gc.ErrorMatches, "failed!")
}
