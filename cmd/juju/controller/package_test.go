// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller_test

import (
	"testing"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

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
}

func (s *baseControllerSuite) createTestClientStore(c *gc.C) *jujuclienttesting.MemStore {
	controllers, err := jujuclient.ParseControllers([]byte(s.controllersYaml))
	c.Assert(err, jc.ErrorIsNil)

	models, err := jujuclient.ParseModels([]byte(s.modelsYaml))
	c.Assert(err, jc.ErrorIsNil)

	accounts, err := jujuclient.ParseAccounts([]byte(s.accountsYaml))
	c.Assert(err, jc.ErrorIsNil)

	store := jujuclienttesting.NewMemStore()
	store.Controllers = controllers.Controllers
	store.CurrentControllerName = controllers.CurrentController
	store.Models = models
	store.Accounts = accounts
	s.store = store
	return store
}

const testControllersYaml = `
controllers:
  aws-test:
    uuid: this-is-the-aws-test-uuid
    api-endpoints: [this-is-aws-test-of-many-api-endpoints]
    ca-cert: this-is-aws-test-ca-cert
    cloud: aws
    region: us-east-1
    model-count: 2
    machine-count: 5
    agent-version: 2.0.1
  mallards:
    uuid: this-is-another-uuid
    api-endpoints: [this-is-another-of-many-api-endpoints, this-is-one-more-of-many-api-endpoints]
    ca-cert: this-is-another-ca-cert
    cloud: mallards
    region: mallards1
  mark-test-prodstack:
    uuid: this-is-a-uuid
    api-endpoints: [this-is-one-of-many-api-endpoints]
    ca-cert: this-is-a-ca-cert
    cloud: prodstack
current-controller: mallards
`

const testModelsYaml = `
controllers:
  aws-test:
    models:
      controller:
        uuid: ghi
    current-model: controller
  mallards:
    models:
      controller:
        uuid: abc
      my-model:
        uuid: def
    current-model: my-model
`

const testAccountsYaml = `
controllers:
  aws-test:
    user: admin@local
    password: hun+er2
  mark-test-prodstack:
    user: admin@local
    password: hunter2
  mallards:
    user: admin@local
    password: hunter2
    last-known-access: superuser
`
