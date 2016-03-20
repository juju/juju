// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller_test

import (
	"testing"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	coretesting "github.com/juju/juju/testing"
)

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type baseControllerSuite struct {
	coretesting.FakeJujuXDGDataHomeSuite
	store                                     jujuclient.ClientStore
	controllersYaml, modelsYaml, accountsYaml string
	expectedOutput, expectedErr               string
}

func (s *baseControllerSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.controllersYaml = testControllersYaml
	s.modelsYaml = testModelsYaml
	s.accountsYaml = testAccountsYaml
	s.store = nil

	err := modelcmd.WriteCurrentController("local.mallards")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *baseControllerSuite) createTestClientStore(c *gc.C) {
	controllers, err := jujuclient.ParseControllers([]byte(s.controllersYaml))
	c.Assert(err, jc.ErrorIsNil)

	models, err := jujuclient.ParseModels([]byte(s.modelsYaml))
	c.Assert(err, jc.ErrorIsNil)

	accounts, err := jujuclient.ParseAccounts([]byte(s.accountsYaml))
	c.Assert(err, jc.ErrorIsNil)

	s.store = &jujuclienttesting.MemStore{
		Controllers: controllers,
		Models:      models,
		Accounts:    accounts,
	}
}

const testControllersYaml = `
controllers:
  local.aws-test:
    servers: [instance-1-2-4.useast.aws.com]
    uuid: this-is-the-aws-test-uuid
    api-endpoints: [this-is-aws-test-of-many-api-endpoints]
    ca-cert: this-is-aws-test-ca-cert
  local.mallards:
    servers: [maas-1-05.cluster.mallards]
    uuid: this-is-another-uuid
    api-endpoints: [this-is-another-of-many-api-endpoints, this-is-one-more-of-many-api-endpoints]
    ca-cert: this-is-another-ca-cert
  local.mark-test-prodstack:
    servers: [vm-23532.prodstack.canonical.com, great.test.server.hostname.co.nz]
    uuid: this-is-a-uuid
    api-endpoints: [this-is-one-of-many-api-endpoints]
    ca-cert: this-is-a-ca-cert
`

const testModelsYaml = `
controllers:
  local.aws-test:
    accounts:
      admin@local:
        models:
          admin:
            uuid: ghi
        current-model: admin
  local.mallards:
    accounts:
      admin@local:
        models:
          admin:
            uuid: abc
          my-model:
            uuid: def
        current-model: my-model
`

const testAccountsYaml = `
controllers:
  local.aws-test:
    accounts:
      admin@local:
        user: admin@local
        password: hun+er2
  local.mark-test-prodstack:
    accounts:
      admin@local:
        user: admin@local
        password: hunter2
  local.mallards:
    accounts:
      admin@local:
        user: admin@local
        password: hunter2
      bob@local:
        user: bob@local
        password: huntert00
      bob@remote:
        user: bob@remote
    current-account: admin@local
`
