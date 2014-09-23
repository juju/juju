// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	stdtesting "testing"

	gc "launchpad.net/gocheck"

	"github.com/juju/juju/api/backups"
	"github.com/juju/juju/apiserver/params"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state/backups/metadata"
	"github.com/juju/juju/testing"
)

func TestAll(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}

type baseBackupsSuite struct {
	jujutesting.JujuConnSuite

	origin *metadata.Origin
	meta   *metadata.Metadata
	client *backups.Client
}

func (s *baseBackupsSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	s.origin = metadata.NewOrigin("eggs", "0", "main-host")
	s.meta = metadata.NewMetadata(*s.origin, "", nil)
	s.meta.SetID("spam")
	s.meta.Finish(10, "ham", "", nil)
	s.meta.SetStored()

	s.client = backups.NewClient(s.APIState)
}

func (s *baseBackupsSuite) checkMetadataResult(
	c *gc.C, result *params.BackupsMetadataResult,
	meta *metadata.Metadata, notes string,
) {
	c.Check(result.ID, gc.Equals, meta.ID())
	c.Check(result.Started, gc.Equals, meta.Started())
	c.Check(result.Finished, gc.Equals, *meta.Finished())
	c.Check(result.Checksum, gc.Equals, meta.Checksum())
	c.Check(result.ChecksumFormat, gc.Equals, meta.ChecksumFormat())
	c.Check(result.Size, gc.Equals, meta.Size())
	c.Check(result.Stored, gc.Equals, meta.Stored())
	c.Check(result.Notes, gc.Equals, notes)

	origin := meta.Origin()
	c.Check(result.Environment, gc.Equals, origin.Environment())
	c.Check(result.Machine, gc.Equals, origin.Machine())
	c.Check(result.Hostname, gc.Equals, origin.Hostname())
	c.Check(result.Version, gc.Equals, origin.Version())
}

func (s *baseBackupsSuite) metadataResult() *params.BackupsMetadataResult {
	result := &params.BackupsMetadataResult{}
	result.UpdateFromMetadata(s.meta)
	return result
}
