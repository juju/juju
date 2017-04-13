// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charmrepo.v2-unstable"
	"gopkg.in/juju/names.v2"

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
	ch := testcharms.Repo.CharmArchivePath(s.CharmsPath, "riak")
	err := runDeploy(c, ch, "riak", "--series", "quantal")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *RemoveApplicationSuite) TestLocalApplication(c *gc.C) {
	s.setupTestApplication(c)
	ctx, err := runRemoveApplication(c, "riak")
	c.Assert(err, jc.ErrorIsNil)
	stderr := cmdtesting.Stderr(ctx)
	c.Assert(stderr, gc.Equals, "removing application riak\n")
	riak, err := s.State.Application("riak")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(riak.Life(), gc.Equals, state.Dying)
}

func (s *RemoveApplicationSuite) TestInformStorageRemoved(c *gc.C) {
	ch := testcharms.Repo.CharmArchivePath(s.CharmsPath, "storage-filesystem")
	err := runDeploy(c, ch, "storage-filesystem", "--series", "quantal", "-n2", "--storage", "data=2,rootfs")
	c.Assert(err, jc.ErrorIsNil)

	ctx, err := runRemoveApplication(c, "storage-filesystem")
	c.Assert(err, jc.ErrorIsNil)
	stderr := cmdtesting.Stderr(ctx)
	c.Assert(stderr, gc.Equals, `
removing application storage-filesystem
- will remove storage data/0
- will remove storage data/1
- will remove storage data/2
- will remove storage data/3
`[1:])
}

func (s *RemoveApplicationSuite) TestRemoteApplication(c *gc.C) {
	_, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name:        "remote-app",
		SourceModel: names.NewModelTag("test"),
		Token:       "token",
	})
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.RemoteApplication("remote-app")
	c.Assert(err, jc.ErrorIsNil)

	ctx, err := runRemoveApplication(c, "remote-app")
	c.Assert(err, jc.ErrorIsNil)
	stderr := cmdtesting.Stderr(ctx)
	c.Assert(stderr, gc.Equals, "removing application remote-app\n")

	// Removed immediately since there are no units.
	_, err = s.State.RemoteApplication("remote-app")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *RemoveApplicationSuite) TestRemoveLocalMetered(c *gc.C) {
	ch := testcharms.Repo.CharmArchivePath(s.CharmsPath, "metered")
	deploy := NewDefaultDeployCommand()
	_, err := cmdtesting.RunCommand(c, deploy, ch, "--series", "quantal")
	c.Assert(err, jc.ErrorIsNil)
	_, err = runRemoveApplication(c, "metered")
	c.Assert(err, jc.ErrorIsNil)
	riak, err := s.State.Application("metered")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(riak.Life(), gc.Equals, state.Dying)
}

func (s *RemoveApplicationSuite) TestBlockRemoveService(c *gc.C) {
	s.setupTestApplication(c)

	// block operation
	s.BlockRemoveObject(c, "TestBlockRemoveService")
	_, err := runRemoveApplication(c, "riak")
	s.AssertBlocked(c, err, ".*TestBlockRemoveService.*")
	riak, err := s.State.Application("riak")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(riak.Life(), gc.Equals, state.Alive)
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
