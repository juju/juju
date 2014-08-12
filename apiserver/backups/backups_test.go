// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"io"

	"github.com/juju/errors"
	"github.com/juju/names"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/apiserver/backups"
	"github.com/juju/juju/apiserver/common"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state/backups/db"
	"github.com/juju/juju/state/backups/metadata"
)

type fakeImpl struct {
	meta    *metadata.Metadata
	archive io.ReadCloser
	err     error
}

func (i *fakeImpl) Create(db.ConnInfo, metadata.Origin, string) (*metadata.Metadata, error) {
	if i.err != nil {
		return nil, i.err
	}
	return i.meta, nil
}

func (i *fakeImpl) Get(string) (*metadata.Metadata, io.ReadCloser, error) {
	if i.err != nil {
		return nil, nil, i.err
	}
	return i.meta, i.archive, nil
}

type backupsSuite struct {
	testing.JujuConnSuite
	resources  *common.Resources
	authorizer *apiservertesting.FakeAuthorizer
	api        *backups.BackupsAPI
	meta       *metadata.Metadata
}

func (s *backupsSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.resources = common.NewResources()
	tag := names.NewUserTag("spam")
	s.authorizer = &apiservertesting.FakeAuthorizer{Tag: tag}
	var err error
	s.api, err = backups.NewBackupsAPI(s.State, s.resources, s.authorizer)
	c.Assert(err, gc.IsNil)
	origin := metadata.NewOrigin("", "", "")
	s.meta = metadata.NewMetadata(*origin, "", nil)
}

var _ = gc.Suite(&backupsSuite{})

func (s *backupsSuite) setImpl(
	c *gc.C, meta *metadata.Metadata, err string,
) *fakeImpl {
	impl := fakeImpl{
		meta: meta,
	}
	if err != "" {
		impl.err = errors.Errorf(err)
	}
	backups.SetImpl(s.api, &impl)
	return &impl
}

func (s *backupsSuite) TestRegistered(c *gc.C) {
	_, err := common.Facades.GetType("Backups", 0)
	c.Check(err, gc.IsNil)
}

func (s *backupsSuite) TestNewBackupsAPIOkay(c *gc.C) {
	api, err := backups.NewBackupsAPI(s.State, s.resources, s.authorizer)
	c.Assert(err, gc.IsNil)
	st, backupsImpl := backups.APIValues(api)

	c.Check(st, gc.Equals, s.State)
	c.Check(backupsImpl, gc.NotNil) // XXX Need better tests.
}

func (s *backupsSuite) TestNewBackupsAPINotAuthorized(c *gc.C) {
	s.authorizer.Tag = names.NewServiceTag("eggs")
	_, err := backups.NewBackupsAPI(s.State, s.resources, s.authorizer)

	c.Check(err, gc.Equals, common.ErrPerm)
}
