// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"testing"
	"time"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/backups"
	"github.com/juju/juju/apiserver/params"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state/backups/metadata"
	backupstesting "github.com/juju/juju/state/backups/testing"
	coretesting "github.com/juju/juju/testing"
)

func TestAll(t *testing.T) {
	coretesting.MgoTestPackage(t)
}

type baseSuite struct {
	jujutesting.JujuConnSuite
	backupstesting.BaseSuite
	client *backups.Client
}

func (s *baseSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.JujuConnSuite.SetUpTest(c)
	s.client = backups.NewClient(s.APIState)
}

func (s *baseSuite) metadataResult() *params.BackupsMetadataResult {
	result := &params.BackupsMetadataResult{}
	result.UpdateFromMetadata(s.Meta)
	return result
}

func (s *baseSuite) checkMetadataResult(
	c *gc.C, result *params.BackupsMetadataResult, meta *metadata.Metadata,
) {
	var finished, stored time.Time
	if meta.Finished != nil {
		finished = *meta.Finished
	}
	if meta.Stored() != nil {
		stored = *(meta.Stored())
	}

	c.Check(result.ID, gc.Equals, meta.ID())
	c.Check(result.Started, gc.Equals, meta.Started)
	c.Check(result.Finished, gc.Equals, finished)
	c.Check(result.Checksum, gc.Equals, meta.Checksum())
	c.Check(result.ChecksumFormat, gc.Equals, meta.ChecksumFormat())
	c.Check(result.Size, gc.Equals, meta.Size())
	c.Check(result.Stored, gc.Equals, stored)
	c.Check(result.Notes, gc.Equals, meta.Notes)

	c.Check(result.Environment, gc.Equals, meta.Origin.Environment)
	c.Check(result.Machine, gc.Equals, meta.Origin.Machine)
	c.Check(result.Hostname, gc.Equals, meta.Origin.Hostname)
	c.Check(result.Version, gc.Equals, meta.Origin.Version)
}
