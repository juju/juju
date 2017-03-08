// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelcmd_test

import (
	"strings"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	coretesting "github.com/juju/juju/testing"
)

type BaseCommandSuite struct {
	coretesting.FakeJujuXDGDataHomeSuite
	store *jujuclienttesting.MemStore
}

var _ = gc.Suite(&BaseCommandSuite{})

func (s *BaseCommandSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)

	s.store = jujuclienttesting.NewMemStore()
	s.store.CurrentControllerName = "foo"
	s.store.Controllers["foo"] = jujuclient.ControllerDetails{
		APIEndpoints: []string{"testing.invalid:1234"},
	}
	s.store.Models["foo"] = &jujuclient.ControllerModels{
		Models: map[string]jujuclient.ModelDetails{
			"admin/badmodel":  {"deadbeef"},
			"admin/goodmodel": {"deadbeef2"},
		},
		CurrentModel: "admin/badmodel",
	}
	s.store.Accounts["foo"] = jujuclient.AccountDetails{
		User: "bar", Password: "hunter2",
	}
}

func (s *BaseCommandSuite) assertUnknownModel(c *gc.C, current, expectedCurrent string) {
	s.store.Models["foo"].CurrentModel = current
	apiOpen := func(*api.Info, api.DialOpts) (api.Connection, error) {
		return nil, errors.Trace(&params.Error{Code: params.CodeModelNotFound, Message: "model deaddeaf not found"})
	}
	cmd := modelcmd.NewModelCommandBase(s.store, "foo", "admin/badmodel")
	cmd.SetAPIOpen(apiOpen)
	conn, err := cmd.NewAPIRoot()
	c.Assert(conn, gc.IsNil)
	msg := strings.Replace(err.Error(), "\n", "", -1)
	c.Assert(msg, gc.Equals, `model "admin/badmodel" has been removed from the controller, run 'juju models' and switch to one of them.There are 1 accessible models on controller "foo".`)
	c.Assert(s.store.Models["foo"].Models, gc.HasLen, 1)
	c.Assert(s.store.Models["foo"].Models["admin/goodmodel"], gc.DeepEquals, jujuclient.ModelDetails{"deadbeef2"})
	c.Assert(s.store.Models["foo"].CurrentModel, gc.Equals, expectedCurrent)
}

func (s *BaseCommandSuite) TestUnknownModel(c *gc.C) {
	s.assertUnknownModel(c, "admin/badmodel", "")
}

func (s *BaseCommandSuite) TestUnknownModelNotCurrent(c *gc.C) {
	s.assertUnknownModel(c, "admin/goodmodel", "admin/goodmodel")
}

type NewGetBootstrapConfigParamsFuncSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&NewGetBootstrapConfigParamsFuncSuite{})

func (NewGetBootstrapConfigParamsFuncSuite) TestDetectCredentials(c *gc.C) {
	clientStore := jujuclienttesting.NewMemStore()
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
		coretesting.Context(c),
		clientStore,
		&registry,
	)
	_, params, err := f("foo")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(params.Cloud.Credential.Label, gc.Equals, "finalized")
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
