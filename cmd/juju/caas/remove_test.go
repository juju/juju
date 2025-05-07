// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caas_test

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
	"github.com/juju/tc"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/juju/caas"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/jujuclient"
)

type fakeCredentialStore struct {
	jujutesting.Stub
	*jujuclient.MemStore
}

func (fcs *fakeCredentialStore) CredentialForCloud(string) (*cloud.CloudCredential, error) {
	fcs.AddCall("CredentialForCloud")
	return &cloud.CloudCredential{}, nil
}

func (fcs *fakeCredentialStore) AllCredentials() (map[string]cloud.CloudCredential, error) {
	fcs.AddCall("AllCredentials")
	return map[string]cloud.CloudCredential{}, nil
}

func (fcs *fakeCredentialStore) UpdateCredential(cloudName string, details cloud.CloudCredential) error {
	fcs.AddCall("UpdateCredential", cloudName, details)
	return nil
}

type removeCAASSuite struct {
	jujutesting.IsolationSuite
	fakeCloudAPI       *fakeRemoveCloudAPI
	cloudMetadataStore *fakeCloudMetadataStore
	store              *fakeCredentialStore
}

var _ = tc.Suite(&removeCAASSuite{})

type fakeRemoveCloudAPI struct {
	caas.RemoveCloudAPI
	jujutesting.Stub
}

func (api *fakeRemoveCloudAPI) RemoveCloud(ctx context.Context, cloud string) error {
	api.AddCall("RemoveCloud", cloud)
	return api.NextErr()
}

func (api *fakeRemoveCloudAPI) Close() error {
	return nil
}

func (s *removeCAASSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.fakeCloudAPI = &fakeRemoveCloudAPI{}
	s.store = &fakeCredentialStore{
		MemStore: NewMockClientStore(),
	}

	var logger loggo.Logger
	s.cloudMetadataStore = &fakeCloudMetadataStore{CallMocker: jujutesting.NewCallMocker(logger)}

	k8sCloud := cloud.Cloud{Name: "myk8s", Type: "kubernetes"}
	initialCloudMap := map[string]cloud.Cloud{"myk8s": k8sCloud}

	s.cloudMetadataStore.Call("PersonalCloudMetadata").Returns(initialCloudMap, nil)
	s.cloudMetadataStore.Call("WritePersonalCloudMetadata", map[string]cloud.Cloud{}).Returns(nil)
}

func (s *removeCAASSuite) makeCommand() cmd.Command {
	removecmd := caas.NewRemoveCAASCommandForTest(
		s.cloudMetadataStore,
		s.store,
		s.store,
		func(ctx context.Context) (caas.RemoveCloudAPI, error) {
			return s.fakeCloudAPI, nil
		},
	)
	return removecmd
}

func (s *removeCAASSuite) runCommand(c *tc.C, cmd cmd.Command, args ...string) (*cmd.Context, error) {
	return cmdtesting.RunCommand(c, cmd, args...)
}

func (s *removeCAASSuite) TestExtraArg(c *tc.C) {
	command := s.makeCommand()
	_, err := s.runCommand(c, command, "k8sname", "extra")
	c.Assert(err, tc.ErrorMatches, `unrecognized args: \["extra"\]`)
}

func (s *removeCAASSuite) TestMissingName(c *tc.C) {
	command := s.makeCommand()
	_, err := s.runCommand(c, command)
	c.Assert(err, tc.ErrorMatches, `missing k8s cloud name.`)
}

func (s *removeCAASSuite) TestRemove(c *tc.C) {
	command := s.makeCommand()
	_, err := s.runCommand(c, command, "myk8s", "-c", "foo", "--client")
	c.Assert(err, jc.ErrorIsNil)

	s.fakeCloudAPI.CheckCallNames(c, "RemoveCloud")
	s.fakeCloudAPI.CheckCall(c, 0, "RemoveCloud", "myk8s")

	s.cloudMetadataStore.CheckCallNames(c, "PersonalCloudMetadata", "PersonalCloudMetadata", "WritePersonalCloudMetadata")
	s.cloudMetadataStore.CheckCall(c, 2, "WritePersonalCloudMetadata", map[string]cloud.Cloud{})

	s.store.CheckCallNames(c, "CredentialForCloud", "UpdateCredential")
	s.store.CheckCall(c, 1, "UpdateCredential", "myk8s", cloud.CloudCredential{})
}

func (s *removeCAASSuite) TestRemoveControllerOnly(c *tc.C) {
	command := s.makeCommand()
	_, err := s.runCommand(c, command, "myk8s", "-c", "foo")
	c.Assert(err, jc.ErrorIsNil)

	// controller side operations
	s.fakeCloudAPI.CheckCallNames(c, "RemoveCloud")
	s.fakeCloudAPI.CheckCall(c, 0, "RemoveCloud", "myk8s")

	// client side operations
	s.cloudMetadataStore.CheckNoCalls(c)
	s.store.CheckNoCalls(c)
}

func (s *removeCAASSuite) TestRemoveClientOnly(c *tc.C) {
	command := s.makeCommand()
	_, err := s.runCommand(c, command, "myk8s", "--client")
	c.Assert(err, jc.ErrorIsNil)

	// controller side operations
	s.fakeCloudAPI.CheckNoCalls(c)

	// client side operations
	s.cloudMetadataStore.CheckCallNames(c, "PersonalCloudMetadata", "WritePersonalCloudMetadata")
	s.cloudMetadataStore.CheckCall(c, 1, "WritePersonalCloudMetadata", map[string]cloud.Cloud{})
	s.store.CheckCallNames(c, "UpdateCredential")
	s.store.CheckCall(c, 0, "UpdateCredential", "myk8s", cloud.CloudCredential{})
}

func (s *removeCAASSuite) TestRemoveNotInController(c *tc.C) {
	s.fakeCloudAPI.SetErrors(errors.NotFoundf("cloud"))
	command := s.makeCommand()
	_, err := s.runCommand(c, command, "myk8s", "-c", "foo")
	c.Assert(err, tc.ErrorMatches, "cannot remove k8s cloud from controller.*")
	s.store.CheckNoCalls(c)
}

func (s *removeCAASSuite) TestRemoveNotInLocal(c *tc.C) {
	command := s.makeCommand()
	_, err := s.runCommand(c, command, "yourk8s", "-c", "foo", "--client")
	c.Assert(err, jc.ErrorIsNil)

	s.fakeCloudAPI.CheckCallNames(c, "RemoveCloud")
	s.fakeCloudAPI.CheckCall(c, 0, "RemoveCloud", "yourk8s")

	s.cloudMetadataStore.CheckCallNames(c, "PersonalCloudMetadata", "PersonalCloudMetadata")
	s.store.CheckCall(c, 0, "UpdateCredential", "yourk8s", cloud.CloudCredential{})
}
