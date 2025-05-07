// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller_test

import (
	"testing"

	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
)

func TestPackage(t *testing.T) {
	tc.TestingT(t)
}

type baseControllerSuite struct {
	coretesting.FakeJujuXDGDataHomeSuite
	store                                     jujuclient.ClientStore
	controllersYaml, modelsYaml, accountsYaml string
	expectedOutput, expectedErr               string
}

func (s *baseControllerSuite) SetUpTest(c *tc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.controllersYaml = testControllersYaml
	s.modelsYaml = testModelsYaml
	s.accountsYaml = testAccountsYaml
	s.store = jujuclienttesting.MinimalStore()
}

func (s *baseControllerSuite) createTestClientStore(c *tc.C) *jujuclient.MemStore {
	controllers, err := jujuclient.ParseControllers([]byte(s.controllersYaml))
	c.Assert(err, jc.ErrorIsNil)

	models, err := jujuclient.ParseModels([]byte(s.modelsYaml))
	c.Assert(err, jc.ErrorIsNil)

	accounts, err := jujuclient.ParseAccounts([]byte(s.accountsYaml))
	c.Assert(err, jc.ErrorIsNil)

	store := jujuclient.NewMemStore()
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
    cloud: aws
    region: us-east-1
    model-count: 2
    machine-count: 5
    agent-version: 2.0.1
    ca-cert: this-is-aws-test-ca-cert
  mallards:
    uuid: deadbeef-1bad-500d-9000-4b1d0d06f00d
    api-endpoints: [this-is-another-of-many-api-endpoints, this-is-one-more-of-many-api-endpoints]
    cloud: mallards
    region: mallards1
    ca-cert: |-
      -----BEGIN CERTIFICATE-----
      MIICHDCCAcagAwIBAgIUfzWn5ktGMxD6OiTgfiZyvKdM+ZYwDQYJKoZIhvcNAQEL
      BQAwazENMAsGA1UEChMEanVqdTEzMDEGA1UEAwwqanVqdS1nZW5lcmF0ZWQgQ0Eg
      Zm9yIG1vZGVsICJqdWp1IHRlc3RpbmciMSUwIwYDVQQFExwxMjM0LUFCQ0QtSVMt
      Tk9ULUEtUkVBTC1VVUlEMB4XDTE2MDkyMTEwNDgyN1oXDTI2MDkyODEwNDgyN1ow
      azENMAsGA1UEChMEanVqdTEzMDEGA1UEAwwqanVqdS1nZW5lcmF0ZWQgQ0EgZm9y
      IG1vZGVsICJqdWp1IHRlc3RpbmciMSUwIwYDVQQFExwxMjM0LUFCQ0QtSVMtTk9U
      LUEtUkVBTC1VVUlEMFwwDQYJKoZIhvcNAQEBBQADSwAwSAJBAL+0X+1zl2vt1wI4
      1Q+RnlltJyaJmtwCbHRhREXVGU7t0kTMMNERxqLnuNUyWRz90Rg8s9XvOtCqNYW7
      mypGrFECAwEAAaNCMEAwDgYDVR0PAQH/BAQDAgKkMA8GA1UdEwEB/wQFMAMBAf8w
      HQYDVR0OBBYEFHueMLZ1QJ/2sKiPIJ28TzjIMRENMA0GCSqGSIb3DQEBCwUAA0EA
      ovZN0RbUHrO8q9Eazh0qPO4mwW9jbGTDz126uNrLoz1g3TyWxIas1wRJ8IbCgxLy
      XUrBZO5UPZab66lJWXyseA==
      -----END CERTIFICATE-----
  mark-test-prodstack:
    uuid: this-is-a-uuid
    api-endpoints: [this-is-one-of-many-api-endpoints]
    cloud: prodstack
    ca-cert: this-is-a-ca-cert
  k8s-controller:
    uuid: this-is-a-k8s-uuid
    api-endpoints: [this-is-one-of-many-k8s-api-endpoints]
    cloud: microk8s
    region: localhost
    type: kubernetes
    ca-cert: this-is-a-k8s-ca-cert
    machine-count: 3
    agent-version: 6.6.6
current-controller: mallards
`

const testModelsYaml = `
controllers:
  aws-test:
    models:
      controller:
        uuid: ghi
        type: iaas
    current-model: admin/controller
  mallards:
    models:
      model0:
        uuid: abc
        type: iaas
      my-model:
        uuid: def
        type: iaas
    current-model: admin/my-model
  k8s-controller:
    models:
      controller:
        uuid: xyz
        type: caas
      my-k8s-model:
        uuid: def
        type: caas
    current-model: admin/my-k8s-model
`

const testAccountsYaml = `
controllers:
  aws-test:
    user: admin
    password: hun+er2
  mark-test-prodstack:
    user: admin
    password: hunter2
  mallards:
    user: admin
    password: hunter2
    last-known-access: superuser
  k8s-controller:
    user: admin
    password: hunter2
    last-known-access: superuser
`
