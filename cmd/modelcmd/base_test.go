// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelcmd_test

import (
	"io/ioutil"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/jujuclient"
	coretesting "github.com/juju/juju/testing"
)

type BaseCommandSuite struct {
	testing.IsolationSuite
	store *jujuclient.MemStore
}

var _ = gc.Suite(&BaseCommandSuite{})

func (s *BaseCommandSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.store = jujuclient.NewMemStore()
	s.store.CurrentControllerName = "foo"
	s.store.Controllers["foo"] = jujuclient.ControllerDetails{
		APIEndpoints: []string{"testing.invalid:1234"},
	}
	s.store.Models["foo"] = &jujuclient.ControllerModels{
		Models: map[string]jujuclient.ModelDetails{
			"admin/badmodel":  {ModelUUID: "deadbeef", ModelType: model.IAAS},
			"admin/goodmodel": {ModelUUID: "deadbeef2", ModelType: model.IAAS},
		},
		CurrentModel: "admin/badmodel",
	}
	s.store.Accounts["foo"] = jujuclient.AccountDetails{
		User: "bar", Password: "hunter2",
	}
}

func (s *BaseCommandSuite) assertUnknownModel(c *gc.C, baseCmd *modelcmd.ModelCommandBase, current, expectedCurrent string) {
	s.store.Models["foo"].CurrentModel = current
	apiOpen := func(*api.Info, api.DialOpts) (api.Connection, error) {
		return nil, errors.Trace(&params.Error{Code: params.CodeModelNotFound, Message: "model deaddeaf not found"})
	}
	baseCmd.SetClientStore(s.store)
	baseCmd.SetAPIOpen(apiOpen)
	modelcmd.InitContexts(&cmd.Context{Stderr: ioutil.Discard}, baseCmd)
	modelcmd.SetRunStarted(baseCmd)
	baseCmd.SetModelName("foo:admin/badmodel", false)
	conn, err := baseCmd.NewAPIRoot()
	c.Assert(conn, gc.IsNil)
	msg := strings.Replace(err.Error(), "\n", "", -1)
	c.Assert(msg, gc.Equals, `model "admin/badmodel" has been removed from the controller, run 'juju models' and switch to one of them.`)
	c.Assert(s.store.Models["foo"].Models, gc.HasLen, 1)
	c.Assert(s.store.Models["foo"].Models["admin/goodmodel"], gc.DeepEquals,
		jujuclient.ModelDetails{ModelUUID: "deadbeef2", ModelType: model.IAAS})
	c.Assert(s.store.Models["foo"].CurrentModel, gc.Equals, expectedCurrent)
}

func (s *BaseCommandSuite) TestUnknownModel(c *gc.C) {
	s.assertUnknownModel(c, new(modelcmd.ModelCommandBase), "admin/badmodel", "admin/badmodel")
}

func (s *BaseCommandSuite) TestUnknownUncachedModel(c *gc.C) {
	baseCmd := new(modelcmd.ModelCommandBase)
	baseCmd.CanClearCurrentModel = false
	baseCmd.RemoveModelFromClientStore(s.store, "foo", "admin/nonexistent")
	// expecting silence in the logs since this model has never been cached.
	c.Assert(c.GetTestLog(), gc.DeepEquals, "")
}

func (s *BaseCommandSuite) TestUnknownModelCanRemoveCachedCurrent(c *gc.C) {
	baseCmd := new(modelcmd.ModelCommandBase)
	baseCmd.CanClearCurrentModel = true
	s.assertUnknownModel(c, baseCmd, "admin/badmodel", "")
}

func (s *BaseCommandSuite) TestUnknownModelNotCurrent(c *gc.C) {
	s.assertUnknownModel(c, new(modelcmd.ModelCommandBase), "admin/goodmodel", "admin/goodmodel")
}

func (s *BaseCommandSuite) TestUnknownModelNotCurrentCanRemoveCachedCurrent(c *gc.C) {
	baseCmd := new(modelcmd.ModelCommandBase)
	baseCmd.CanClearCurrentModel = true
	s.assertUnknownModel(c, baseCmd, "admin/goodmodel", "admin/goodmodel")
}

type NewGetBootstrapConfigParamsFuncSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&NewGetBootstrapConfigParamsFuncSuite{})

func (NewGetBootstrapConfigParamsFuncSuite) TestDetectCredentials(c *gc.C) {
	clientStore := jujuclient.NewMemStore()
	clientStore.Controllers["foo"] = jujuclient.ControllerDetails{}
	clientStore.BootstrapConfig["foo"] = jujuclient.BootstrapConfig{
		Cloud:               "cloud",
		CloudType:           "cloud-type",
		ControllerModelUUID: coretesting.ModelTag.Id(),
		Config: map[string]interface{}{
			"name": "foo",
			"type": "cloud-type",
		},
	}
	var registry mockProviderRegistry

	f := modelcmd.NewGetBootstrapConfigParamsFunc(
		cmdtesting.Context(c),
		clientStore,
		&registry,
	)
	_, params, err := f("foo")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(params.Cloud.Credential.Label, gc.Equals, "finalized")
}

func (NewGetBootstrapConfigParamsFuncSuite) TestCloudCACert(c *gc.C) {
	fakeCert := coretesting.CACert
	clientStore := jujuclient.NewMemStore()
	clientStore.Controllers["foo"] = jujuclient.ControllerDetails{}
	clientStore.BootstrapConfig["foo"] = jujuclient.BootstrapConfig{
		Cloud:               "cloud",
		CloudType:           "cloud-type",
		ControllerModelUUID: coretesting.ModelTag.Id(),
		Config: map[string]interface{}{
			"name": "foo",
			"type": "cloud-type",
		},
		CloudCACertificates: []string{fakeCert},
	}
	var registry mockProviderRegistry

	f := modelcmd.NewGetBootstrapConfigParamsFunc(
		cmdtesting.Context(c),
		clientStore,
		&registry,
	)
	_, params, err := f("foo")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(params.Cloud.CACertificates, jc.SameContents, []string{fakeCert})
}

type mockProviderRegistry struct {
	environs.ProviderRegistry
}

func (r *mockProviderRegistry) Provider(t string) (environs.EnvironProvider, error) {
	return &mockEnvironProvider{}, nil
}

type mockEnvironProvider struct {
	environs.EnvironProvider
}

func (p *mockEnvironProvider) DetectCredentials() (*cloud.CloudCredential, error) {
	return cloud.NewEmptyCloudCredential(), nil
}

func (p *mockEnvironProvider) FinalizeCredential(
	_ environs.FinalizeCredentialContext,
	args environs.FinalizeCredentialParams,
) (*cloud.Credential, error) {
	out := args.Credential
	out.Label = "finalized"
	return &out, nil
}

func (p *mockEnvironProvider) CredentialSchemas() map[cloud.AuthType]cloud.CredentialSchema {
	return map[cloud.AuthType]cloud.CredentialSchema{cloud.EmptyAuthType: {}}
}
