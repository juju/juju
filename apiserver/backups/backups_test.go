// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"io"

	"github.com/juju/errors"
	"github.com/juju/names"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/backups"
	"github.com/juju/juju/apiserver/common"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state/backups/db"
	"github.com/juju/juju/state/backups/files"
	"github.com/juju/juju/state/backups/metadata"
)

type fakeBackups struct {
	meta    *metadata.Metadata
	archive io.ReadCloser
	err     error
}

func (i *fakeBackups) Create(files.Paths, db.ConnInfo, metadata.Origin, string) (*metadata.Metadata, error) {
	if i.err != nil {
		return nil, errors.Trace(i.err)
	}
	return i.meta, nil
}

func (i *fakeBackups) Get(string) (*metadata.Metadata, io.ReadCloser, error) {
	if i.err != nil {
		return nil, nil, errors.Trace(i.err)
	}
	return i.meta, i.archive, nil
}

func (i *fakeBackups) List() ([]metadata.Metadata, error) {
	if i.err != nil {
		return nil, errors.Trace(i.err)
	}
	return []metadata.Metadata{*i.meta}, nil
}

func (i *fakeBackups) Remove(string) error {
	if i.err != nil {
		return errors.Trace(i.err)
	}
	return nil
}

type backupsSuite struct {
	testing.JujuConnSuite
	resources  *common.Resources
	authorizer *apiservertesting.FakeAuthorizer
	api        *backups.API
	meta       *metadata.Metadata
}

var _ = gc.Suite(&backupsSuite{})

func (s *backupsSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.resources = common.NewResources()
	s.resources.RegisterNamed("dataDir", common.StringResource("/var/lib/juju"))
	tag := names.NewLocalUserTag("spam")
	s.authorizer = &apiservertesting.FakeAuthorizer{Tag: tag}
	var err error
	s.api, err = backups.NewAPI(s.State, s.resources, s.authorizer)
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
	backups.SetBackups(s.api, &fake)
	return &fake
}

func (s *backupsSuite) TestRegistered(c *gc.C) {
	_, err := common.Facades.GetType("Backups", 0)
	c.Check(err, gc.IsNil)
}

func (s *backupsSuite) TestNewAPIOkay(c *gc.C) {
	api, err := backups.NewAPI(s.State, s.resources, s.authorizer)
	c.Assert(err, gc.IsNil)
	st, backupsImpl := backups.APIValues(api)

	c.Check(st, gc.Equals, s.State)
	c.Check(backupsImpl, gc.NotNil)
}

func (s *backupsSuite) TestNewAPINotAuthorized(c *gc.C) {
	s.authorizer.Tag = names.NewServiceTag("eggs")
	_, err := backups.NewAPI(s.State, s.resources, s.authorizer)

	c.Check(errors.Cause(err), gc.Equals, common.ErrPerm)
}
