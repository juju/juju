// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"io"
	"io/ioutil"

	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	backupsAPI "github.com/juju/juju/apiserver/backups"
	"github.com/juju/juju/apiserver/common"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/backups"
	backupstesting "github.com/juju/juju/state/backups/testing"
	"github.com/juju/juju/testing/factory"
)

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
	c.Assert(err, jc.ErrorIsNil)
	s.meta = backupstesting.NewMetadataStarted()
}

func (s *backupsSuite) setBackups(c *gc.C, meta *backups.Metadata, err string) *backupstesting.FakeBackups {
	fake := backupstesting.FakeBackups{
		Meta: meta,
	}
	if meta != nil {
		fake.MetaList = append(fake.MetaList, meta)
	}
	if err != "" {
		fake.Error = errors.Errorf(err)
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
	c.Check(err, jc.ErrorIsNil)
}

func (s *backupsSuite) TestNewAPIOkay(c *gc.C) {
	_, err := backupsAPI.NewAPI(s.State, s.resources, s.authorizer)
	c.Check(err, jc.ErrorIsNil)
}

func (s *backupsSuite) TestNewAPINotAuthorized(c *gc.C) {
	s.authorizer.Tag = names.NewServiceTag("eggs")
	_, err := backupsAPI.NewAPI(s.State, s.resources, s.authorizer)

	c.Check(errors.Cause(err), gc.Equals, common.ErrPerm)
}

func (s *backupsSuite) TestNewAPIHostedEnvironmentFails(c *gc.C) {
	otherState := factory.NewFactory(s.State).MakeEnvironment(c, nil)
	defer otherState.Close()
	_, err := backupsAPI.NewAPI(otherState, s.resources, s.authorizer)
	c.Check(err, gc.ErrorMatches, "backups are not supported for hosted environments")
}
