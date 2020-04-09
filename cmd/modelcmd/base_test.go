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
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/pki"
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
	baseCmd.SetModelIdentifier("foo:admin/badmodel", false)
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

func (s *BaseCommandSuite) TestMigratedModelErrorHandling(c *gc.C) {
	var callCount int
	apiOpen := func(*api.Info, api.DialOpts) (api.Connection, error) {
		var alias string
		if callCount > 0 {
			alias = "brand-new-controller"
		}
		callCount++

		nhp, _ := network.ParseMachineHostPort("1.2.3.4:5555")
		redirErr := &api.RedirectError{
			CACert:          coretesting.CACert,
			Servers:         []network.MachineHostPorts{{*nhp}},
			ControllerAlias: alias,
		}
		return nil, errors.Trace(redirErr)
	}

	baseCmd := new(modelcmd.ModelCommandBase)
	baseCmd.SetClientStore(s.store)
	baseCmd.SetAPIOpen(apiOpen)
	modelcmd.InitContexts(&cmd.Context{Stderr: ioutil.Discard}, baseCmd)
	modelcmd.SetRunStarted(baseCmd)

	c.Assert(baseCmd.SetModelIdentifier("foo:admin/badmodel", false), jc.ErrorIsNil)

	fingerprint, _, err := pki.Fingerprint([]byte(coretesting.CACert))
	c.Assert(err, jc.ErrorIsNil)

	specs := []struct {
		descr   string
		expErr  string
		setupFn func()
	}{
		{
			descr: "model migration document does not contain a controller alias (simulate an older controller)",
			expErr: `Model "admin/badmodel" has been migrated to another controller.
To access it run one of the following commands (you can replace the -c argument with your own preferred controller name):
  'juju login 1.2.3.4:5555 -c new-controller'

New controller fingerprint [` + fingerprint + `]`,
		},
		{
			descr: "model migration document contains a controller alias",
			expErr: `Model "admin/badmodel" has been migrated to another controller.
To access it run one of the following commands (you can replace the -c argument with your own preferred controller name):
  'juju login 1.2.3.4:5555 -c brand-new-controller'

New controller fingerprint [` + fingerprint + `]`,
		},
		{
			descr: "model migration docuemnt contains a controller alias but we already know the controller locally by a different name",
			expErr: `Model "admin/badmodel" has been migrated to controller "bar".
To access it run 'juju switch bar:admin/badmodel'.`,
			setupFn: func() {
				// Model has been migrated to a controller which does exist in the local
				// cache. This can happen if we have 2 users A and B (on separate machines)
				// and:
				// - both A and B have SRC and DST controllers in their local cache
				// - A migrates the model from SRC -> DST
				// - B tries to invoke any model-related command on SRC for the migrated model.
				s.store.Controllers["bar"] = jujuclient.ControllerDetails{
					APIEndpoints: []string{"1.2.3.4:5555"},
				}
			},
		},
	}

	for specIndex, spec := range specs {
		c.Logf("test %d: %s", specIndex, spec.descr)

		if spec.setupFn != nil {
			spec.setupFn()
		}

		_, err := baseCmd.NewAPIRoot()
		c.Assert(err, gc.Not(gc.IsNil))
		c.Assert(err.Error(), gc.Equals, spec.expErr)
	}
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
