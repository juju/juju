// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"io/ioutil"
	"strings"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state/backups"
)

type downloadSuite struct {
	baseSuite
}

var _ = gc.Suite(&downloadSuite{})

func (s *downloadSuite) TestSuccessfulRequest(c *gc.C) {
	store := backups.NewStorage(s.State)
	defer store.Close()
	backupsState := backups.NewBackups(store)

	r := strings.NewReader("<compressed archive data>")
	meta, err := backups.NewMetadataState(s.State, "0")
	c.Assert(err, jc.ErrorIsNil)
	// The Add method requires the length to be set
	// otherwise the content is assumed to have length 0.
	meta.Raw.Size = int64(r.Len())
	id, err := backupsState.Add(r, meta)
	c.Assert(err, jc.ErrorIsNil)
	resultArchive, err := s.client.Download(id)
	c.Assert(err, jc.ErrorIsNil)

	resultData, err := ioutil.ReadAll(resultArchive)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(string(resultData), gc.Equals, "<compressed archive data>")
}

func (s *downloadSuite) TestFailedRequest(c *gc.C) {
	resultArchive, err := s.client.Download("unknown")
	c.Assert(err, gc.ErrorMatches, `GET https://.*/model/.*/backups: backup metadata "unknown" not found`)
	c.Assert(err, jc.Satisfies, params.IsCodeNotFound)
	c.Assert(resultArchive, gc.Equals, nil)
}
