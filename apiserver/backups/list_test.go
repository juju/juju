// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"bytes"
	"io/ioutil"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/backups"
	"github.com/juju/juju/apiserver/params"
)

func (s *backupsSuite) TestListOkay(c *gc.C) {
	impl := s.setBackups(c, s.meta, "")
	impl.Archive = ioutil.NopCloser(bytes.NewBufferString("spamspamspam"))
	args := params.BackupsListArgs{}
	result, err := s.api.List(args)
	c.Assert(err, jc.ErrorIsNil)

	item := backups.ResultFromMetadata(s.meta)
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
