// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for info.

package model_test

import (
	"context"
	stdtesting "testing"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/api"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/juju/model"
	"github.com/juju/juju/core/life"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/pki"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/rpc/params"
)

func TestShowCommandSuite(t *stdtesting.T) {
	tc.Run(t, &ShowCommandSuite{})
}

type ShowCommandSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	fake            fakeModelShowClient
	store           *jujuclient.MemStore
	expectedOutput  attrs
	expectedDisplay string
}

func (s *ShowCommandSuite) SetUpTest(c *tc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	lastConnection := time.Date(2015, 3, 20, 0, 0, 0, 0, time.UTC)
	statusSince := time.Date(2016, 4, 5, 0, 0, 0, 0, time.UTC)
	migrationStart := time.Date(2016, 4, 6, 0, 10, 0, 0, time.UTC)
	migrationEnd := time.Date(2016, 4, 7, 0, 0, 15, 0, time.UTC)

	users := []params.ModelUserInfo{{
		UserName:       "admin",
		LastConnection: &lastConnection,
		Access:         "write",
	}, {
		UserName:    "bob",
		DisplayName: "Bob",
		Access:      "read",
	}}

	s.fake = fakeModelShowClient{
		info: params.ModelInfo{
			Name:           "mymodel",
			Qualifier:      "production",
			UUID:           testing.ModelTag.Id(),
			Type:           "iaas",
			ControllerUUID: "1ca2293b-fdb9-4299-97d6-55583bb39364",
			IsController:   false,
			CloudTag:       "cloud-some-cloud",
			CloudRegion:    "some-region",
			ProviderType:   "openstack",
			Life:           life.Alive,
			Status: params.EntityStatus{
				Status: status.Active,
				Since:  &statusSince,
			},
			Users: users,
			Migration: &params.ModelMigrationStatus{
				Status: "obfuscating Quigley matrix",
				Start:  &migrationStart,
				End:    &migrationEnd,
			},
		},
	}

	s.expectedOutput = attrs{
		"mymodel": attrs{
			"name":            "production/mymodel",
			"short-name":      "mymodel",
			"model-uuid":      "deadbeef-0bad-400d-8000-4b1d0d06f00d",
			"model-type":      "iaas",
			"controller-uuid": "1ca2293b-fdb9-4299-97d6-55583bb39364",
			"controller-name": "testing",
			"is-controller":   false,
			"cloud":           "some-cloud",
			"region":          "some-region",
			"type":            "openstack",
			"life":            "alive",
			"status": attrs{
				"current":         "active",
				"since":           "2016-04-05",
				"migration":       "obfuscating Quigley matrix",
				"migration-start": "2016-04-06",
				"migration-end":   "2016-04-07",
			},
			"users": attrs{
				"admin": attrs{
					"access":          "write",
					"last-connection": "2015-03-20",
				},
				"bob": attrs{
					"display-name":    "Bob",
					"access":          "read",
					"last-connection": "never connected",
				},
			},
		},
	}

	s.store = jujuclient.NewMemStore()
	s.store.CurrentControllerName = "testing"
	s.store.Controllers["testing"] = jujuclient.ControllerDetails{}
	s.store.Accounts["testing"] = jujuclient.AccountDetails{
		User: "admin",
	}
	err := s.store.UpdateModel("testing", "admin/mymodel", jujuclient.ModelDetails{
		ModelUUID: testing.ModelTag.Id(),
		ModelType: coremodel.IAAS,
	})
	c.Assert(err, tc.ErrorIsNil)
	s.store.Models["testing"].CurrentModel = "admin/mymodel"
}

func (s *ShowCommandSuite) TestShow(c *tc.C) {
	_, err := cmdtesting.RunCommand(c, s.newShowCommand())
	c.Assert(err, tc.ErrorIsNil)
	s.fake.CheckCalls(c, []testhelpers.StubCall{
		{"ModelInfo", []interface{}{[]names.ModelTag{testing.ModelTag}}},
		{"Close", nil},
	})
}

func (s *ShowCommandSuite) TestShowWithPartModelUUID(c *tc.C) {
	_, err := cmdtesting.RunCommand(c, s.newShowCommand(), "deadbeef")
	c.Assert(err, tc.ErrorIsNil)
	s.fake.CheckCalls(c, []testhelpers.StubCall{
		{"ModelInfo", []interface{}{[]names.ModelTag{testing.ModelTag}}},
		{"Close", nil},
	})
}

func (s *ShowCommandSuite) TestShowUnknownCallsRefresh(c *tc.C) {
	called := false
	refresh := func(context.Context, jujuclient.ClientStore, string) error {
		called = true
		return nil
	}
	_, err := cmdtesting.RunCommand(c, model.NewShowCommandForTest(&s.fake, refresh, s.store), "unknown")
	c.Check(called, tc.IsTrue)
	c.Check(err, tc.ErrorIs, errors.NotFound)
}

func (s *ShowCommandSuite) TestShowFormatYaml(c *tc.C) {
	ctx, err := cmdtesting.RunCommand(c, s.newShowCommand(), "--format", "yaml")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), tc.YAMLEquals, s.expectedOutput)
}

func (s *ShowCommandSuite) addCredentialToTestData(credentialValid *bool) {
	s.fake.info.CloudCredentialTag = "cloudcred-some-cloud_some-owner_some-credential"
	s.fake.info.CloudCredentialValidity = credentialValid

	modelOutput := s.expectedOutput["mymodel"].(attrs)
	modelOutput["credential"] = attrs{
		"name":           "some-credential",
		"owner":          "some-owner",
		"cloud":          "some-cloud",
		"validity-check": common.HumanReadableBoolPointer(credentialValid, "valid", "invalid"),
	}
}

func (s *ShowCommandSuite) TestShowWithCredentialFormatYaml(c *tc.C) {
	_true := true
	s.addCredentialToTestData(&_true)
	ctx, err := cmdtesting.RunCommand(c, s.newShowCommand(), "--format", "yaml")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), tc.YAMLEquals, s.expectedOutput)
}

func (s *ShowCommandSuite) TestShowFormatJson(c *tc.C) {
	ctx, err := cmdtesting.RunCommand(c, s.newShowCommand(), "--format", "json")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), tc.JSONEquals, s.expectedOutput)
}

func (s *ShowCommandSuite) TestShowWithCredentialFormatJson(c *tc.C) {
	_false := false
	s.addCredentialToTestData(&_false)
	ctx, err := cmdtesting.RunCommand(c, s.newShowCommand(), "--format", "json")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), tc.JSONEquals, s.expectedOutput)
}

func (s *ShowCommandSuite) TestUnrecognizedArg(c *tc.C) {
	_, err := cmdtesting.RunCommand(c, s.newShowCommand(), "admin", "whoops")
	c.Assert(err, tc.ErrorMatches, `unrecognized args: \["whoops"\]`)
}

func (s *ShowCommandSuite) addSecretBackendTestData() {
	s.fake.info.SecretBackends = []params.SecretBackendResult{{
		Result: params.SecretBackend{
			Name: "myvault",
		},
		NumSecrets: 666,
		Status:     "error",
		Message:    "vault is sealed",
	}}

	modelOutput := s.expectedOutput["mymodel"].(attrs)
	modelOutput["secret-backends"] = attrs{
		"myvault": attrs{
			"num-secrets": 666,
			"status":      "error",
			"message":     "vault is sealed",
		}}
}

func (s *ShowCommandSuite) TestShowWithSecretBackendFormatYaml(c *tc.C) {
	s.addSecretBackendTestData()
	ctx, err := cmdtesting.RunCommand(c, s.newShowCommand(), "--format", "yaml")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), tc.YAMLEquals, s.expectedOutput)
}

func (s *ShowCommandSuite) TestShowWithSecretBackendFormatJson(c *tc.C) {
	s.addSecretBackendTestData()
	ctx, err := cmdtesting.RunCommand(c, s.newShowCommand(), "--format", "json")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), tc.JSONEquals, s.expectedOutput)
}

func (s *ShowCommandSuite) TestShowBasicIncompleteModelsYaml(c *tc.C) {
	s.fake.infos = []params.ModelInfoResult{
		{Result: createBasicModelInfo()},
	}
	s.expectedDisplay = `
basic-model:
  name: prod/basic-model
  short-name: basic-model
  model-uuid: deadbeef-0bad-400d-8000-4b1d0d06f00d
  model-type: iaas
  controller-uuid: deadbeef-1bad-500d-9000-4b1d0d06f00d
  controller-name: testing
  is-controller: false
  cloud: altostratus
  region: mid-level
  life: dead
`[1:]
	s.assertShowOutput(c, "yaml")
}

func (s *ShowCommandSuite) TestShowBasicIncompleteModelsJson(c *tc.C) {
	s.fake.infos = []params.ModelInfoResult{
		{Result: createBasicModelInfo()},
	}
	s.expectedDisplay = "{\"basic-model\":" +
		"{\"name\":\"prod/basic-model\"," +
		"\"short-name\":\"basic-model\"," +
		"\"model-uuid\":\"deadbeef-0bad-400d-8000-4b1d0d06f00d\"," +
		"\"model-type\":\"iaas\"," +
		"\"controller-uuid\":\"deadbeef-1bad-500d-9000-4b1d0d06f00d\"," +
		"\"controller-name\":\"testing\"," +
		"\"is-controller\":false," +
		"\"cloud\":\"altostratus\"," +
		"\"region\":\"mid-level\"," +
		"\"life\":\"dead\"}}\n"
	s.assertShowOutput(c, "json")
}

func (s *ShowCommandSuite) TestShowBasicWithStatusIncompleteModelsYaml(c *tc.C) {
	s.fake.infos = []params.ModelInfoResult{
		{Result: createBasicModelInfoWithStatus()},
	}
	s.expectedDisplay = `
basic-model:
  name: prod/basic-model
  short-name: basic-model
  model-uuid: deadbeef-0bad-400d-8000-4b1d0d06f00d
  model-type: iaas
  controller-uuid: deadbeef-1bad-500d-9000-4b1d0d06f00d
  controller-name: testing
  is-controller: false
  cloud: altostratus
  region: mid-level
  life: dead
  status:
    current: busy
`[1:]
	s.assertShowOutput(c, "yaml")
}

func (s *ShowCommandSuite) TestShowBasicWithStatusIncompleteModelsJson(c *tc.C) {
	s.fake.infos = []params.ModelInfoResult{
		{Result: createBasicModelInfoWithStatus()},
	}
	s.expectedDisplay = "{\"basic-model\":" +
		"{\"name\":\"prod/basic-model\"," +
		"\"short-name\":\"basic-model\"," +
		"\"model-uuid\":\"deadbeef-0bad-400d-8000-4b1d0d06f00d\"," +
		"\"model-type\":\"iaas\"," +
		"\"controller-uuid\":\"deadbeef-1bad-500d-9000-4b1d0d06f00d\"," +
		"\"controller-name\":\"testing\"," +
		"\"is-controller\":false," +
		"\"cloud\":\"altostratus\"," +
		"\"region\":\"mid-level\"," +
		"\"life\":\"dead\"," +
		"\"status\":{\"current\":\"busy\"}}}\n"

	s.assertShowOutput(c, "json")
}

func (s *ShowCommandSuite) TestShowBasicWithMigrationIncompleteModelsYaml(c *tc.C) {
	basicAndMigrationStatusInfo := createBasicModelInfo()
	addMigrationStatusStatus(basicAndMigrationStatusInfo)
	s.fake.infos = []params.ModelInfoResult{
		{Result: basicAndMigrationStatusInfo},
	}
	s.expectedDisplay = `
basic-model:
  name: prod/basic-model
  short-name: basic-model
  model-uuid: deadbeef-0bad-400d-8000-4b1d0d06f00d
  model-type: iaas
  controller-uuid: deadbeef-1bad-500d-9000-4b1d0d06f00d
  controller-name: testing
  is-controller: false
  cloud: altostratus
  region: mid-level
  life: dead
  status:
    migration: importing
    migration-start: just now
`[1:]
	s.assertShowOutput(c, "yaml")
}

func (s *ShowCommandSuite) TestShowBasicWithMigrationIncompleteModelsJson(c *tc.C) {
	basicAndMigrationStatusInfo := createBasicModelInfo()
	addMigrationStatusStatus(basicAndMigrationStatusInfo)
	s.fake.infos = []params.ModelInfoResult{
		{Result: basicAndMigrationStatusInfo},
	}
	s.expectedDisplay = "{\"basic-model\":" +
		"{\"name\":\"prod/basic-model\"," +
		"\"short-name\":\"basic-model\"," +
		"\"model-uuid\":\"deadbeef-0bad-400d-8000-4b1d0d06f00d\"," +
		"\"model-type\":\"iaas\"," +
		"\"controller-uuid\":\"deadbeef-1bad-500d-9000-4b1d0d06f00d\"," +
		"\"controller-name\":\"testing\"," +
		"\"is-controller\":false," +
		"\"cloud\":\"altostratus\"," +
		"\"region\":\"mid-level\"," +
		"\"life\":\"dead\"," +
		"\"status\":{\"migration\":\"importing\",\"migration-start\":\"just now\"}}}\n"
	s.assertShowOutput(c, "json")
}

func (s *ShowCommandSuite) TestShowBasicWithStatusAndMigrationIncompleteModelsYaml(c *tc.C) {
	basicAndStatusAndMigrationInfo := createBasicModelInfoWithStatus()
	addMigrationStatusStatus(basicAndStatusAndMigrationInfo)
	s.fake.infos = []params.ModelInfoResult{
		{Result: basicAndStatusAndMigrationInfo},
	}
	s.expectedDisplay = `
basic-model:
  name: prod/basic-model
  short-name: basic-model
  model-uuid: deadbeef-0bad-400d-8000-4b1d0d06f00d
  model-type: iaas
  controller-uuid: deadbeef-1bad-500d-9000-4b1d0d06f00d
  controller-name: testing
  is-controller: false
  cloud: altostratus
  region: mid-level
  life: dead
  status:
    current: busy
    migration: importing
    migration-start: just now
`[1:]
	s.assertShowOutput(c, "yaml")
}

func (s *ShowCommandSuite) TestShowBasicWithStatusAndMigrationIncompleteModelsJson(c *tc.C) {
	basicAndStatusAndMigrationInfo := createBasicModelInfoWithStatus()
	addMigrationStatusStatus(basicAndStatusAndMigrationInfo)
	s.fake.infos = []params.ModelInfoResult{
		{Result: basicAndStatusAndMigrationInfo},
	}
	s.expectedDisplay = "{\"basic-model\":" +
		"{\"name\":\"prod/basic-model\"," +
		"\"short-name\":\"basic-model\"," +
		"\"model-uuid\":\"deadbeef-0bad-400d-8000-4b1d0d06f00d\"," +
		"\"model-type\":\"iaas\"," +
		"\"controller-uuid\":\"deadbeef-1bad-500d-9000-4b1d0d06f00d\"," +
		"\"controller-name\":\"testing\"," +
		"\"is-controller\":false," +
		"\"cloud\":\"altostratus\"," +
		"\"region\":\"mid-level\"," +
		"\"life\":\"dead\"," +
		"\"status\":{\"current\":\"busy\",\"migration\":\"importing\",\"migration-start\":\"just now\"}}}\n"

	s.assertShowOutput(c, "json")
}

func (s *ShowCommandSuite) TestShowBasicWithProviderIncompleteModelsYaml(c *tc.C) {
	basicAndProviderTypeInfo := createBasicModelInfo()
	basicAndProviderTypeInfo.ProviderType = "aws"
	s.fake.infos = []params.ModelInfoResult{
		{Result: basicAndProviderTypeInfo},
	}
	s.expectedDisplay = `
basic-model:
  name: prod/basic-model
  short-name: basic-model
  model-uuid: deadbeef-0bad-400d-8000-4b1d0d06f00d
  model-type: iaas
  controller-uuid: deadbeef-1bad-500d-9000-4b1d0d06f00d
  controller-name: testing
  is-controller: false
  cloud: altostratus
  region: mid-level
  type: aws
  life: dead
`[1:]
	s.assertShowOutput(c, "yaml")
}

func (s *ShowCommandSuite) TestShowBasicWithProviderIncompleteModelsJson(c *tc.C) {
	basicAndProviderTypeInfo := createBasicModelInfo()
	basicAndProviderTypeInfo.ProviderType = "aws"
	s.fake.infos = []params.ModelInfoResult{
		{Result: basicAndProviderTypeInfo},
	}
	s.expectedDisplay = "{\"basic-model\":" +
		"{\"name\":\"prod/basic-model\"," +
		"\"short-name\":\"basic-model\"," +
		"\"model-uuid\":\"deadbeef-0bad-400d-8000-4b1d0d06f00d\"," +
		"\"model-type\":\"iaas\"," +
		"\"controller-uuid\":\"deadbeef-1bad-500d-9000-4b1d0d06f00d\"," +
		"\"controller-name\":\"testing\"," +
		"\"is-controller\":false," +
		"\"cloud\":\"altostratus\"," +
		"\"region\":\"mid-level\"," +
		"\"type\":\"aws\"," +
		"\"life\":\"dead\"}}\n"
	s.assertShowOutput(c, "json")
}

func (s *ShowCommandSuite) TestShowBasicWithUsersIncompleteModelsYaml(c *tc.C) {
	basicAndUsersInfo := createBasicModelInfo()
	basicAndUsersInfo.Users = []params.ModelUserInfo{{
		UserName:    "admin",
		DisplayName: "display name",
		Access:      "admin",
	}}
	s.fake.infos = []params.ModelInfoResult{
		{Result: basicAndUsersInfo},
	}
	s.expectedDisplay = `
basic-model:
  name: prod/basic-model
  short-name: basic-model
  model-uuid: deadbeef-0bad-400d-8000-4b1d0d06f00d
  model-type: iaas
  controller-uuid: deadbeef-1bad-500d-9000-4b1d0d06f00d
  controller-name: testing
  is-controller: false
  cloud: altostratus
  region: mid-level
  life: dead
  users:
    admin:
      display-name: display name
      access: admin
      last-connection: never connected
`[1:]
	s.assertShowOutput(c, "yaml")
}

func (s *ShowCommandSuite) TestShowBasicWithUsersIncompleteModelsJson(c *tc.C) {
	basicAndUsersInfo := createBasicModelInfo()
	basicAndUsersInfo.Users = []params.ModelUserInfo{{
		UserName:    "admin",
		DisplayName: "display name",
		Access:      "admin",
	}}

	s.fake.infos = []params.ModelInfoResult{
		{Result: basicAndUsersInfo},
	}
	s.expectedDisplay = "{\"basic-model\":" +
		"{\"name\":\"prod/basic-model\"," +
		"\"short-name\":\"basic-model\"," +
		"\"model-uuid\":\"deadbeef-0bad-400d-8000-4b1d0d06f00d\"," +
		"\"model-type\":\"iaas\"," +
		"\"controller-uuid\":\"deadbeef-1bad-500d-9000-4b1d0d06f00d\"," +
		"\"controller-name\":\"testing\"," +
		"\"is-controller\":false," +
		"\"cloud\":\"altostratus\"," +
		"\"region\":\"mid-level\"," +
		"\"life\":\"dead\"," +
		"\"users\":{\"admin\":{\"display-name\":\"display name\",\"access\":\"admin\",\"last-connection\":\"never connected\"}}}}\n"
	s.assertShowOutput(c, "json")
}

func (s *ShowCommandSuite) TestShowBasicWithMachinesIncompleteModelsYaml(c *tc.C) {
	basicAndMachinesInfo := createBasicModelInfo()
	basicAndMachinesInfo.Machines = []params.ModelMachineInfo{
		{Id: "2"}, {Id: "12"},
	}
	s.fake.infos = []params.ModelInfoResult{
		{Result: basicAndMachinesInfo},
	}
	s.expectedDisplay = `
basic-model:
  name: prod/basic-model
  short-name: basic-model
  model-uuid: deadbeef-0bad-400d-8000-4b1d0d06f00d
  model-type: iaas
  controller-uuid: deadbeef-1bad-500d-9000-4b1d0d06f00d
  controller-name: testing
  is-controller: false
  cloud: altostratus
  region: mid-level
  life: dead
  machines:
    "2":
      cores: 0
    "12":
      cores: 0
`[1:]
	s.assertShowOutput(c, "yaml")
}

func (s *ShowCommandSuite) TestShowBasicWithMachinesIncompleteModelsJson(c *tc.C) {
	basicAndMachinesInfo := createBasicModelInfo()
	basicAndMachinesInfo.Machines = []params.ModelMachineInfo{
		{Id: "2"}, {Id: "12"},
	}
	s.fake.infos = []params.ModelInfoResult{
		{Result: basicAndMachinesInfo},
	}
	s.expectedDisplay = "{\"basic-model\":" +
		"{\"name\":\"prod/basic-model\"," +
		"\"short-name\":\"basic-model\"," +
		"\"model-uuid\":\"deadbeef-0bad-400d-8000-4b1d0d06f00d\"," +
		"\"model-type\":\"iaas\"," +
		"\"controller-uuid\":\"deadbeef-1bad-500d-9000-4b1d0d06f00d\"," +
		"\"controller-name\":\"testing\"," +
		"\"is-controller\":false," +
		"\"cloud\":\"altostratus\"," +
		"\"region\":\"mid-level\"," +
		"\"life\":\"dead\"," +
		"\"machines\":{\"12\":{\"cores\":0},\"2\":{\"cores\":0}}}}\n"
	s.assertShowOutput(c, "json")
}

func (s *ShowCommandSuite) TestShowModelWithAgentVersionInJson(c *tc.C) {
	s.expectedDisplay = "{\"basic-model\":" +
		"{\"name\":\"prod/basic-model\"," +
		"\"short-name\":\"basic-model\"," +
		"\"model-uuid\":\"deadbeef-0bad-400d-8000-4b1d0d06f00d\"," +
		"\"model-type\":\"iaas\"," +
		"\"controller-uuid\":\"deadbeef-1bad-500d-9000-4b1d0d06f00d\"," +
		"\"controller-name\":\"testing\"," +
		"\"is-controller\":false," +
		"\"cloud\":\"altostratus\"," +
		"\"region\":\"mid-level\"," +
		"\"life\":\"dead\"," +
		"\"agent-version\":\"2.55.5\"}}\n"
	s.assertShowModelWithAgent(c, "json")
}

func (s *ShowCommandSuite) TestShowModelWithAgentVersionInYaml(c *tc.C) {
	s.expectedDisplay = `
basic-model:
  name: prod/basic-model
  short-name: basic-model
  model-uuid: deadbeef-0bad-400d-8000-4b1d0d06f00d
  model-type: iaas
  controller-uuid: deadbeef-1bad-500d-9000-4b1d0d06f00d
  controller-name: testing
  is-controller: false
  cloud: altostratus
  region: mid-level
  life: dead
  agent-version: 2.55.5
`[1:]
	s.assertShowModelWithAgent(c, "yaml")
}

func (s *ShowCommandSuite) assertShowModelWithAgent(c *tc.C, format string) {
	// Since most of the tests in this suite already test model infos without
	// agent version, all we need to do here is to test one with it.
	agentVersion, err := semversion.Parse("2.55.5")
	c.Assert(err, tc.ErrorIsNil)
	basicTestInfo := createBasicModelInfo()
	basicTestInfo.AgentVersion = &agentVersion
	s.fake.infos = []params.ModelInfoResult{
		{Result: basicTestInfo},
	}
	s.assertShowOutput(c, format)
}

func (s *ShowCommandSuite) newShowCommand() cmd.Command {
	return model.NewShowCommandForTest(&s.fake, noOpRefresh, s.store)
}

func (s *ShowCommandSuite) assertShowOutput(c *tc.C, format string) {
	ctx, err := cmdtesting.RunCommand(c, s.newShowCommand(), "--format", format)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), tc.Equals, s.expectedDisplay)
}

func (s *ShowCommandSuite) TestHandleRedirectError(c *tc.C) {
	nhp, _ := network.ParseMachineHostPort("1.2.3.4:5555")
	caFingerprint, _, _ := pki.Fingerprint([]byte(testing.CACert))
	s.fake.SetErrors(
		&api.RedirectError{
			Servers:         []network.MachineHostPorts{{*nhp}},
			CACert:          testing.CACert,
			ControllerAlias: "target",
		},
	)
	_, err := cmdtesting.RunCommand(c, model.NewShowCommandForTest(&s.fake, nil, s.store))
	c.Assert(err, tc.Not(tc.IsNil))
	c.Assert(err.Error(), tc.Equals, `Model "admin/mymodel" has been migrated to another controller.
To access it run one of the following commands (you can replace the -c argument with your own preferred controller name):
  'juju login 1.2.3.4:5555 -c target'

New controller fingerprint [`+caFingerprint+`]`)
}

func createBasicModelInfo() *params.ModelInfo {
	return &params.ModelInfo{
		Name:           "basic-model",
		UUID:           testing.ModelTag.Id(),
		ControllerUUID: testing.ControllerTag.Id(),
		IsController:   false,
		Type:           "iaas",
		Qualifier:      "prod",
		Life:           life.Dead,
		CloudTag:       names.NewCloudTag("altostratus").String(),
		CloudRegion:    "mid-level",
	}
}

func createBasicModelInfoWithStatus() *params.ModelInfo {
	basicAndStatusInfo := createBasicModelInfo()
	basicAndStatusInfo.Status = params.EntityStatus{
		Status: status.Busy,
	}
	return basicAndStatusInfo
}

func addMigrationStatusStatus(existingInfo *params.ModelInfo) {
	now := time.Now()
	existingInfo.Migration = &params.ModelMigrationStatus{
		Status: "importing",
		Start:  &now,
	}
}

func noOpRefresh(_ context.Context, _ jujuclient.ClientStore, _ string) error {
	return nil
}

type attrs map[string]interface{}

type fakeModelShowClient struct {
	testhelpers.Stub
	info  params.ModelInfo
	err   *params.Error
	infos []params.ModelInfoResult
}

func (f *fakeModelShowClient) Close() error {
	f.MethodCall(f, "Close")
	return f.NextErr()
}

func (f *fakeModelShowClient) ModelInfo(ctx context.Context, tags []names.ModelTag) ([]params.ModelInfoResult, error) {
	f.MethodCall(f, "ModelInfo", tags)
	if f.infos != nil {
		return f.infos, nil
	}
	if len(tags) != 1 {
		return nil, errors.Errorf("expected 1 tag, got %d", len(tags))
	}
	if tags[0] != testing.ModelTag {
		return nil, errors.Errorf("expected %s, got %s", testing.ModelTag.Id(), tags[0].Id())
	}
	return []params.ModelInfoResult{{Result: &f.info, Error: f.err}}, f.NextErr()
}
