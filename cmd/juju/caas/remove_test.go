// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caas_test

import (
	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/juju/caas"
)

type removeCAASSuite struct {
	jujutesting.IsolationSuite
	fakeCloudAPI        *fakeRemoveCloudAPI
	store               *fakeCloudMetadataStore
	fileCredentialStore *fakeCredentialStore
}

var _ = gc.Suite(&removeCAASSuite{})

type fakeRemoveCloudAPI struct {
	caas.RemoveCloudAPI
	jujutesting.Stub
}

func (api *fakeRemoveCloudAPI) RemoveCloud(cloud string) error {
	api.AddCall("RemoveCloud", cloud)
	return api.NextErr()
}

func (api *fakeRemoveCloudAPI) Close() error {
	return nil
}

func (s *removeCAASSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.fakeCloudAPI = &fakeRemoveCloudAPI{}
	s.fileCredentialStore = &fakeCredentialStore{}

	var logger loggo.Logger
	s.store = &fakeCloudMetadataStore{CallMocker: jujutesting.NewCallMocker(logger)}

	k8sCloud := cloud.Cloud{Name: "myk8s", Type: "kubernetes"}
	initialCloudMap := map[string]cloud.Cloud{"myk8s": k8sCloud}

	s.store.Call("PersonalCloudMetadata").Returns(initialCloudMap, nil)
	s.store.Call("WritePersonalCloudMetadata", map[string]cloud.Cloud{}).Returns(nil)
}

func (s *removeCAASSuite) makeCommand() cmd.Command {
	removecmd := caas.NewRemoveCAASCommandForTest(
		s.store,
		s.fileCredentialStore,
		NewMockClientStore(),
		func() (caas.RemoveCloudAPI, error) {
			return s.fakeCloudAPI, nil
		},
	)
	return removecmd
}

func (s *removeCAASSuite) runCommand(c *gc.C, cmd cmd.Command, args ...string) (*cmd.Context, error) {
	return cmdtesting.RunCommand(c, cmd, args...)
}

func (s *removeCAASSuite) TestExtraArg(c *gc.C) {
	cmd := s.makeCommand()
	_, err := s.runCommand(c, cmd, "k8sname", "extra")
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["extra"\]`)
}

func (s *removeCAASSuite) TestMissingName(c *gc.C) {
	cmd := s.makeCommand()
	_, err := s.runCommand(c, cmd)
	c.Assert(err, gc.ErrorMatches, `missing k8s name.`)
}

func (s *removeCAASSuite) TestRemove(c *gc.C) {
	cmd := s.makeCommand()
	_, err := s.runCommand(c, cmd, "myk8s")
	c.Assert(err, jc.ErrorIsNil)

	s.fakeCloudAPI.CheckCallNames(c, "RemoveCloud")
	s.fakeCloudAPI.CheckCall(c, 0, "RemoveCloud", "myk8s")

	s.store.CheckCallNames(c, "PersonalCloudMetadata", "WritePersonalCloudMetadata")
	s.store.CheckCall(c, 1, "WritePersonalCloudMetadata", map[string]cloud.Cloud{})

	s.fileCredentialStore.CheckCallNames(c, "UpdateCredential")
	s.fileCredentialStore.CheckCall(c, 0, "UpdateCredential", "myk8s", cloud.CloudCredential{})
}

func (s *removeCAASSuite) TestRemoveNotInController(c *gc.C) {
	s.fakeCloudAPI.SetErrors(errors.NotFoundf("cloud"))
	cmd := s.makeCommand()
	_, err := s.runCommand(c, cmd, "myk8s")
	c.Assert(err, gc.ErrorMatches, "cannot remove k8s cloud from controller.*")

	s.store.CheckNoCalls(c)
	s.fileCredentialStore.CheckNoCalls(c)
}

func (s *removeCAASSuite) TestRemoveNotInLocal(c *gc.C) {
	cmd := s.makeCommand()
	_, err := s.runCommand(c, cmd, "yourk8s")
	c.Assert(err, jc.ErrorIsNil)

	s.fakeCloudAPI.CheckCallNames(c, "RemoveCloud")
	s.fakeCloudAPI.CheckCall(c, 0, "RemoveCloud", "yourk8s")

	s.store.CheckCallNames(c, "PersonalCloudMetadata")
	s.fileCredentialStore.CheckCall(c, 0, "UpdateCredential", "yourk8s", cloud.CloudCredential{})
}
