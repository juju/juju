// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"io"
	"io/ioutil"

	"github.com/juju/errors"
	"github.com/juju/names"
	gc "gopkg.in/check.v1"

	backupsAPI "github.com/juju/juju/apiserver/backups"
	"github.com/juju/juju/apiserver/common"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/backups"
	"github.com/juju/juju/state/backups/db"
)

type fakeBackups struct {
	meta    *backups.Metadata
	archive io.ReadCloser
	err     error
}

func (i *fakeBackups) Create(backups.Paths, db.Info, backups.Origin, string) (*backups.Metadata, error) {
	if i.err != nil {
		return nil, errors.Trace(i.err)
	}
	return i.meta, nil
}

func (i *fakeBackups) Get(string) (*backups.Metadata, io.ReadCloser, error) {
	if i.err != nil {
		return nil, nil, errors.Trace(i.err)
	}
	return i.meta, i.archive, nil
}

func (i *fakeBackups) List() ([]backups.Metadata, error) {
	if i.err != nil {
		return nil, errors.Trace(i.err)
	}
	return []backups.Metadata{*i.meta}, nil
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
	api        *backupsAPI.API
	meta       *backups.Metadata
}

var _ = gc.Suite(&backupsSuite{})

func (s *backupsSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.resources = common.NewResources()
	s.resources.RegisterNamed("dataDir", common.StringResource("/var/lib/juju"))
	tag := names.NewLocalUserTag("spam")
	s.authorizer = &apiservertesting.FakeAuthorizer{Tag: tag}
	var err error
	s.api, err = backupsAPI.NewAPI(s.State, s.resources, s.authorizer)
	c.Assert(err, gc.IsNil)
	s.meta = s.newMeta("")
}

func (s *backupsSuite) newMeta(notes string) *backups.Metadata {
	origin := backups.NewOrigin("<env ID>", "<machine ID>", "<hostname>")
	return backups.NewMetadata(*origin, notes, nil)
}

func (s *backupsSuite) setBackups(c *gc.C, meta *backups.Metadata, err string) *fakeBackups {
	fake := fakeBackups{
		meta: meta,
	}
	if err != "" {
		fake.err = errors.Errorf(err)
	}
	s.PatchValue(backupsAPI.NewBackups,
		func(*state.State) (backups.Backups, io.Closer) {
			return &fake, ioutil.NopCloser(nil)
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
