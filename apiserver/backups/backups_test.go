// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/utils/filestorage"
	gc "launchpad.net/gocheck"

	backupsAPI "github.com/juju/juju/apiserver/backups"
	"github.com/juju/juju/apiserver/common"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state/backups"
	"github.com/juju/juju/state/backups/metadata"
)

type backupsSuite struct {
	testing.JujuConnSuite
	resources  *common.Resources
	authorizer *apiservertesting.FakeAuthorizer
	api        *backupsAPI.API
	meta       *metadata.Metadata
}

var _ = gc.Suite(&backupsSuite{})

func (s *backupsSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.resources = common.NewResources()
	tag := names.NewLocalUserTag("spam")
	s.authorizer = &apiservertesting.FakeAuthorizer{Tag: tag}
	var err error
	s.api, err = backupsAPI.NewAPI(s.State, s.resources, s.authorizer)
	c.Assert(err, gc.IsNil)
	s.meta = s.newMeta("")
}

func (s *backupsSuite) newMeta(notes string) *metadata.Metadata {
	origin := metadata.NewOrigin("<env ID>", "<machine ID>", "<hostname>")
	return metadata.NewMetadata(*origin, notes, nil)
}

func (s *backupsSuite) setBackups(c *gc.C, meta *metadata.Metadata, err string) *fakeBackups {
	fake := fakeBackups{
		meta: meta,
	}
	if err != "" {
		fake.err = errors.Errorf(err)
	}
	s.PatchValue(backupsAPI.NewBackups,
		func(filestorage.FileStorage) backups.Backups {
			return &fake
		},
	)
	return &fake
}

func (s *backupsSuite) TestRegistered(c *gc.C) {
	_, err := common.Facades.GetType("Backups", 0)
	c.Check(err, gc.IsNil)
}

func (s *backupsSuite) TestNewAPIOkay(c *gc.C) {
	_, err := backupsAPI.NewAPI(s.State, s.resources, s.authorizer)
	c.Check(err, gc.IsNil)
}

func (s *backupsSuite) TestNewAPINotAuthorized(c *gc.C) {
	s.authorizer.Tag = names.NewServiceTag("eggs")
	_, err := backupsAPI.NewAPI(s.State, s.resources, s.authorizer)

	c.Check(errors.Cause(err), gc.Equals, common.ErrPerm)
}
