// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charmrepo.v3"

	"github.com/juju/juju/api/annotations"
	"github.com/juju/juju/api/application"
	"github.com/juju/juju/api/charms"
	"github.com/juju/juju/api/modelconfig"
	"github.com/juju/juju/cmd/modelcmd"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testcharms"
	"github.com/juju/juju/testing"
)

type RemoveApplicationSuite struct {
	jujutesting.RepoSuite
	testing.CmdBlockHelper
}

var _ = gc.Suite(&RemoveApplicationSuite{})

func (s *RemoveApplicationSuite) SetUpTest(c *gc.C) {
	s.RepoSuite.SetUpTest(c)
	s.CmdBlockHelper = testing.NewCmdBlockHelper(s.APIState)
	c.Assert(s.CmdBlockHelper, gc.NotNil)
	s.AddCleanup(func(*gc.C) { s.CmdBlockHelper.Close() })
}

func runRemoveApplication(c *gc.C, args ...string) (*cmd.Context, error) {
	return cmdtesting.RunCommand(c, NewRemoveApplicationCommand(), args...)
}

func (s *RemoveApplicationSuite) setupTestApplication(c *gc.C) {
	// Destroy an application that exists.
	ch := testcharms.Repo.CharmArchivePath(s.CharmsPath, "multi-series")
	err := runDeploy(c, ch, "multi-series")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *RemoveApplicationSuite) TestLocalApplication(c *gc.C) {
	s.setupTestApplication(c)
	ctx, err := runRemoveApplication(c, "multi-series")
	c.Assert(err, jc.ErrorIsNil)
	stderr := cmdtesting.Stderr(ctx)
	c.Assert(stderr, gc.Equals, "removing application multi-series\n")
	multiSeries, err := s.State.Application("multi-series")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(multiSeries.Life(), gc.Equals, state.Dying)
}

func (s *RemoveApplicationSuite) TestDetachStorage(c *gc.C) {
	s.testStorageRemoval(c, false)
}

func (s *RemoveApplicationSuite) TestDestroyStorage(c *gc.C) {
	s.testStorageRemoval(c, true)
}

func (s *RemoveApplicationSuite) testStorageRemoval(c *gc.C, destroy bool) {
	ch := testcharms.Repo.CharmArchivePath(s.CharmsPath, "storage-filesystem-multi-series")
	err := runDeploy(c, ch, "storage-filesystem-multi-series", "-n2", "--storage", "data=2,modelscoped")
	c.Assert(err, jc.ErrorIsNil)

	// Materialise the storage by assigning units to machines.
	for _, id := range []string{"storage-filesystem-multi-series/0", "storage-filesystem-multi-series/1"} {
		u, err := s.State.Unit(id)
		c.Assert(err, jc.ErrorIsNil)
		err = s.State.AssignUnit(u, state.AssignCleanEmpty)
		c.Assert(err, jc.ErrorIsNil)
	}

	args := []string{"storage-filesystem-multi-series"}
	action := "detach"
	if destroy {
		args = append(args, "--destroy-storage")
		action = "remove"
	}
	ctx, err := runRemoveApplication(c, args...)
	c.Assert(err, jc.ErrorIsNil)
	stderr := cmdtesting.Stderr(ctx)
	c.Assert(stderr, gc.Equals, fmt.Sprintf(`
removing application storage-filesystem-multi-series
- will %[1]s storage data/0
- will %[1]s storage data/1
- will %[1]s storage data/2
- will %[1]s storage data/3
`[1:], action))
}

func (s *RemoveApplicationSuite) TestRemoveLocalMetered(c *gc.C) {
	ch := testcharms.Repo.CharmArchivePath(s.CharmsPath, "metered-multi-series")
	deploy := NewDeployCommand()
	_, err := cmdtesting.RunCommand(c, deploy, ch)
	c.Assert(err, jc.ErrorIsNil)
	_, err = runRemoveApplication(c, "metered-multi-series")
	c.Assert(err, jc.ErrorIsNil)
	multiSeries, err := s.State.Application("metered-multi-series")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(multiSeries.Life(), gc.Equals, state.Dying)
}

func (s *RemoveApplicationSuite) TestBlockRemoveApplication(c *gc.C) {
	s.setupTestApplication(c)

	// block operation
	s.BlockRemoveObject(c, "TestBlockRemoveApplication")
	_, err := runRemoveApplication(c, "multi-series")
	s.AssertBlocked(c, err, ".*TestBlockRemoveApplication.*")
	multiSeries, err := s.State.Application("multi-series")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(multiSeries.Life(), gc.Equals, state.Alive)
}

func (s *RemoveApplicationSuite) TestFailure(c *gc.C) {
	// Destroy an application that does not exist.
	ctx, err := runRemoveApplication(c, "gargleblaster")
	c.Assert(err, gc.Equals, cmd.ErrSilent)

	stderr := cmdtesting.Stderr(ctx)
	c.Assert(stderr, gc.Equals, `
removing application gargleblaster failed: application "gargleblaster" not found
`[1:])
}

func (s *RemoveApplicationSuite) TestInvalidArgs(c *gc.C) {
	_, err := runRemoveApplication(c)
	c.Assert(err, gc.ErrorMatches, `no application specified`)
	_, err = runRemoveApplication(c, "invalid:name")
	c.Assert(err, gc.ErrorMatches, `invalid application name "invalid:name"`)
}

type RemoveCharmStoreCharmsSuite struct {
	charmStoreSuite
	ctx *cmd.Context
}

var _ = gc.Suite(&RemoveCharmStoreCharmsSuite{})

func (s *RemoveCharmStoreCharmsSuite) SetUpTest(c *gc.C) {
	s.charmStoreSuite.SetUpTest(c)

	s.ctx = cmdtesting.Context(c)

	testcharms.UploadCharm(c, s.client, "cs:quantal/metered-1", "metered")
	deployCmd := &DeployCommand{}
	cmd := modelcmd.Wrap(deployCmd)
	deployCmd.NewAPIRoot = func() (DeployAPI, error) {
		apiRoot, err := deployCmd.ModelCommandBase.NewAPIRoot()
		if err != nil {
			return nil, errors.Trace(err)
		}
		bakeryClient, err := deployCmd.BakeryClient()
		if err != nil {
			return nil, errors.Trace(err)
		}
		cstoreClient := newCharmStoreClient(bakeryClient).WithChannel(deployCmd.Channel)
		return &deployAPIAdapter{
			Connection:        apiRoot,
			apiClient:         &apiClient{Client: apiRoot.Client()},
			charmsClient:      &charmsClient{Client: charms.NewClient(apiRoot)},
			applicationClient: &applicationClient{Client: application.NewClient(apiRoot)},
			modelConfigClient: &modelConfigClient{Client: modelconfig.NewClient(apiRoot)},
			charmstoreClient:  &charmstoreClient{Client: cstoreClient},
			annotationsClient: &annotationsClient{Client: annotations.NewClient(apiRoot)},
			charmRepoClient:   &charmRepoClient{CharmStore: charmrepo.NewCharmStoreFromClient(cstoreClient)},
		}, nil
	}

	_, err := cmdtesting.RunCommand(c, cmd, "cs:quantal/metered-1")
	c.Assert(err, jc.ErrorIsNil)

}
