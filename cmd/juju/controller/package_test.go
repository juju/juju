// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller_test

import (
	"testing"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/jujuclient"
	coretesting "github.com/juju/juju/testing"
)

func TestPackage(t *testing.T) {
	coretesting.MgoTestPackage(t)
}

type baseControllerSuite struct {
	coretesting.FakeJujuXDGDataHomeSuite

	store                       jujuclient.ClientStore
	expectedOutput, expectedErr string
}

func (s *baseControllerSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
}

func (s *baseControllerSuite) TearDownTest(c *gc.C) {
	s.store = nil
	s.FakeJujuXDGDataHomeSuite.TearDownTest(c)
}

func (s *baseControllerSuite) createTestClientStore(c *gc.C) {
	s.store = jujuclient.NewFileClientStore()

	// Load controllers.
	controllers, err := jujuclient.ParseControllers([]byte(testControllersYAML))
	c.Assert(err, jc.ErrorIsNil)
	err = jujuclient.WriteControllersFile(controllers)
	c.Assert(err, jc.ErrorIsNil)

	// Load models.
	models, err := jujuclient.ParseModels([]byte(testModelsYAML))
	c.Assert(err, jc.ErrorIsNil)
	err = jujuclient.WriteModelsFile(models)
	c.Assert(err, jc.ErrorIsNil)

	// Load accounts.
	accounts, err := jujuclient.ParseAccounts([]byte(testAccountsYAML))
	c.Assert(err, jc.ErrorIsNil)
	err = jujuclient.WriteAccountsFile(accounts)
	c.Assert(err, jc.ErrorIsNil)
}

const testControllersYAML = `
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

const testModelsYAML = `
controllers:
  local.aws-test:
    models:
      admin:
        uuid: ghi
    current-model: admin
  local.mallards:
    models:
      admin:
        uuid: abc
      my-model:
        uuid: def
    current-model: my-model
`

const testAccountsYAML = `
controllers:
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
