// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller_test

import (
	"context"
	"regexp"
	"testing"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/api/base"
	apicontroller "github.com/juju/juju/api/controller/controller"
	"github.com/juju/juju/cmd/juju/controller"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
)

type ShowControllerSuite struct {
	baseControllerSuite
	fakeController *fakeController
	api            func(string) controller.ControllerAccessAPI
	setAccess      func(permission.Access)
	modelConfigAPI func(controllerName string) controller.ModelConfigAPI
}

func TestShowControllerSuite(t *testing.T) {
	tc.Run(t, &ShowControllerSuite{})
}

func (s *ShowControllerSuite) SetUpTest(c *tc.C) {
	s.baseControllerSuite.SetUpTest(c)
	s.fakeController = &fakeController{
		machines: map[string][]base.Machine{
			"ghi": {
				{Id: "0", InstanceId: "id-0", HasVote: false, WantsVote: true, Status: "active"},
				{Id: "1", InstanceId: "id-1", HasVote: false, WantsVote: true, Status: "down"},
				{Id: "2", InstanceId: "id-2", HasVote: true, WantsVote: true, Status: "active"},
				{Id: "3", InstanceId: "id-3", HasVote: false, WantsVote: false, Status: "active"},
			},
			"xyz": {
				{Id: "0", InstanceId: "id-0", HasVote: false, WantsVote: true, Status: "active"},
			},
		},
		modelTypes: map[string]model.ModelType{"def": model.CAAS, "xyz": model.CAAS},
		units:      map[string]int{"def": 4},
		access:     permission.SuperuserAccess,
		controllerVersion: apicontroller.ControllerVersion{
			Version:   "999.99.99",
			GitCommit: "badf00d0badf00d0badf00d0badf00d0badf00d0",
		},
	}
	s.api = func(controllerName string) controller.ControllerAccessAPI {
		s.fakeController.controllerName = controllerName
		return s.fakeController
	}
	s.setAccess = func(access permission.Access) {
		s.fakeController.access = access
	}
	s.modelConfigAPI = func(controllerName string) controller.ModelConfigAPI {
		return &fakeModelConfig{}
	}
}

func (s *ShowControllerSuite) TestShowOneControllerOneInStore(c *tc.C) {
	s.controllersYaml = `controllers:
  mallards:
    uuid: this-is-another-uuid
    api-endpoints: [this-is-another-of-many-api-endpoints, this-is-one-more-of-many-api-endpoints]
    cloud: mallards
    agent-version: 999.99.99
    mongo-version: 3.5.12
    ca-cert: this-is-another-ca-cert
`
	s.createTestClientStore(c)

	s.expectedOutput = `
mallards:
  details:
    controller-uuid: this-is-another-uuid
    api-endpoints: [this-is-another-of-many-api-endpoints, this-is-one-more-of-many-api-endpoints]
    cloud: mallards
    agent-version: 999.99.99
    agent-git-commit: badf00d0badf00d0badf00d0badf00d0badf00d0
    controller-model-version: 999.99.99
    mongo-version: 3.5.12
    ca-cert: this-is-another-ca-cert
  models:
    controller:
      model-uuid: abc
      machine-count: 2
      core-count: 4
    my-model:
      model-uuid: def
      machine-count: 2
      core-count: 4
  current-model: admin/my-model
  account:
    user: admin
    access: superuser
`[1:]

	s.assertShowController(c, "mallards")
}

func (s *ShowControllerSuite) TestShowK8sController(c *tc.C) {
	s.createTestClientStore(c)
	s.expectedOutput = `
k8s-controller:
  details:
    controller-uuid: this-is-a-k8s-uuid
    api-endpoints: [this-is-one-of-many-k8s-api-endpoints]
    cloud: microk8s
    region: localhost
    agent-version: 999.99.99
    agent-git-commit: badf00d0badf00d0badf00d0badf00d0badf00d0
    controller-model-version: 999.99.99
    mongo-version: 3.5.12
    ca-cert: this-is-a-k8s-ca-cert
  controller-nodes:
    "0":
      instance-id: id-0
  models:
    controller:
      model-uuid: xyz
    my-k8s-model:
      model-uuid: def
      unit-count: 4
  current-model: admin/my-k8s-model
  account:
    user: admin
    access: superuser
`[1:]

	s.assertShowController(c, "k8s-controller")
}

func (s *ShowControllerSuite) TestShowControllerWithPasswords(c *tc.C) {
	s.controllersYaml = `controllers:
  mallards:
    uuid: this-is-another-uuid
    api-endpoints: [this-is-another-of-many-api-endpoints, this-is-one-more-of-many-api-endpoints]
    cloud: mallards
    agent-version: 999.99.99
    mongo-version: 3.5.12
    ca-cert: this-is-another-ca-cert
`
	s.createTestClientStore(c)

	s.expectedOutput = `
mallards:
  details:
    controller-uuid: this-is-another-uuid
    api-endpoints: [this-is-another-of-many-api-endpoints, this-is-one-more-of-many-api-endpoints]
    cloud: mallards
    agent-version: 999.99.99
    agent-git-commit: badf00d0badf00d0badf00d0badf00d0badf00d0
    controller-model-version: 999.99.99
    mongo-version: 3.5.12
    ca-cert: this-is-another-ca-cert
  models:
    controller:
      model-uuid: abc
      machine-count: 2
      core-count: 4
    my-model:
      model-uuid: def
      machine-count: 2
      core-count: 4
  current-model: admin/my-model
  account:
    user: admin
    access: superuser
    password: hunter2
`[1:]

	s.assertShowController(c, "mallards", "--show-password")
}

func (s *ShowControllerSuite) TestShowControllerWithBootstrapConfig(c *tc.C) {
	s.controllersYaml = `controllers:
  mallards:
    uuid: this-is-another-uuid
    api-endpoints: [this-is-another-of-many-api-endpoints, this-is-one-more-of-many-api-endpoints]
    cloud: mallards
    region: mallards1
    agent-version: 999.99.99
    mongo-version: 3.5.12
    ca-cert: this-is-another-ca-cert
`
	store := s.createTestClientStore(c)
	store.BootstrapConfig["mallards"] = jujuclient.BootstrapConfig{
		Config: map[string]interface{}{
			"name":  "controller",
			"type":  "maas",
			"extra": "value",
		},
		Credential:    "my-credential",
		CloudType:     "maas",
		Cloud:         "mallards",
		CloudRegion:   "mallards1",
		CloudEndpoint: "http://mallards.local/MAAS",
	}

	s.expectedOutput = `
mallards:
  details:
    controller-uuid: this-is-another-uuid
    api-endpoints: [this-is-another-of-many-api-endpoints, this-is-one-more-of-many-api-endpoints]
    cloud: mallards
    region: mallards1
    agent-version: 999.99.99
    agent-git-commit: badf00d0badf00d0badf00d0badf00d0badf00d0
    controller-model-version: 999.99.99
    mongo-version: 3.5.12
    ca-cert: this-is-another-ca-cert
  models:
    controller:
      model-uuid: abc
      machine-count: 2
      core-count: 4
    my-model:
      model-uuid: def
      machine-count: 2
      core-count: 4
  current-model: admin/my-model
  account:
    user: admin
    access: superuser
`[1:]

	s.assertShowController(c, "mallards")
}

func (s *ShowControllerSuite) TestShowOneControllerManyInStore(c *tc.C) {
	s.createTestClientStore(c)

	s.expectedOutput = `
aws-test:
  details:
    controller-uuid: this-is-the-aws-test-uuid
    api-endpoints: [this-is-aws-test-of-many-api-endpoints]
    cloud: aws
    region: us-east-1
    agent-version: 999.99.99
    agent-git-commit: badf00d0badf00d0badf00d0badf00d0badf00d0
    controller-model-version: 999.99.99
    mongo-version: 3.5.12
    ca-cert: this-is-aws-test-ca-cert
  controller-machines:
    "0":
      instance-id: id-0
      ha-status: ha-pending
    "1":
      instance-id: id-1
      ha-status: down, lost connection
    "2":
      instance-id: id-2
      ha-status: ha-enabled
  models:
    controller:
      model-uuid: ghi
      machine-count: 2
      core-count: 4
  current-model: admin/controller
  account:
    user: admin
    access: superuser
`[1:]
	s.assertShowController(c, "aws-test")
}

func (s *ShowControllerSuite) TestShowSomeControllerMoreInStore(c *tc.C) {
	s.createTestClientStore(c)
	s.expectedOutput = `
aws-test:
  details:
    controller-uuid: this-is-the-aws-test-uuid
    api-endpoints: [this-is-aws-test-of-many-api-endpoints]
    cloud: aws
    region: us-east-1
    agent-version: 999.99.99
    agent-git-commit: badf00d0badf00d0badf00d0badf00d0badf00d0
    controller-model-version: 999.99.99
    mongo-version: 3.5.12
    ca-cert: this-is-aws-test-ca-cert
  controller-machines:
    "0":
      instance-id: id-0
      ha-status: ha-pending
    "1":
      instance-id: id-1
      ha-status: down, lost connection
    "2":
      instance-id: id-2
      ha-status: ha-enabled
  models:
    controller:
      model-uuid: ghi
      machine-count: 2
      core-count: 4
  current-model: admin/controller
  account:
    user: admin
    access: superuser
mark-test-prodstack:
  details:
    controller-uuid: this-is-a-uuid
    api-endpoints: [this-is-one-of-many-api-endpoints]
    cloud: prodstack
    agent-version: 999.99.99
    agent-git-commit: badf00d0badf00d0badf00d0badf00d0badf00d0
    controller-model-version: 999.99.99
    mongo-version: 3.5.12
    ca-cert: this-is-a-ca-cert
  account:
    user: admin
    access: superuser
`[1:]

	s.assertShowController(c, "aws-test", "mark-test-prodstack")
}

func (s *ShowControllerSuite) TestShowControllerJsonOne(c *tc.C) {
	s.createTestClientStore(c)

	s.expectedOutput = `
{"aws-test":{"details":{"uuid":"this-is-the-aws-test-uuid","api-endpoints":["this-is-aws-test-of-many-api-endpoints"],"cloud":"aws","region":"us-east-1","agent-version":"999.99.99","agent-git-commit":"badf00d0badf00d0badf00d0badf00d0badf00d0","controller-model-version":"999.99.99","mongo-version":"3.5.12","ca-cert":"this-is-aws-test-ca-cert"},"controller-machines":{"0":{"instance-id":"id-0","ha-status":"ha-pending"},"1":{"instance-id":"id-1","ha-status":"down, lost connection"},"2":{"instance-id":"id-2","ha-status":"ha-enabled"}},"models":{"controller":{"uuid":"ghi","machine-count":2,"core-count":4}},"current-model":"admin/controller","account":{"user":"admin","access":"superuser"}}}
`[1:]

	s.assertShowController(c, "--format", "json", "aws-test")
}

func (s *ShowControllerSuite) TestShowControllerJsonMany(c *tc.C) {
	s.createTestClientStore(c)
	s.expectedOutput = `
{"aws-test":{"details":{"uuid":"this-is-the-aws-test-uuid","api-endpoints":["this-is-aws-test-of-many-api-endpoints"],"cloud":"aws","region":"us-east-1","agent-version":"999.99.99","agent-git-commit":"badf00d0badf00d0badf00d0badf00d0badf00d0","controller-model-version":"999.99.99","mongo-version":"3.5.12","ca-cert":"this-is-aws-test-ca-cert"},"controller-machines":{"0":{"instance-id":"id-0","ha-status":"ha-pending"},"1":{"instance-id":"id-1","ha-status":"down, lost connection"},"2":{"instance-id":"id-2","ha-status":"ha-enabled"}},"models":{"controller":{"uuid":"ghi","machine-count":2,"core-count":4}},"current-model":"admin/controller","account":{"user":"admin","access":"superuser"}},"mark-test-prodstack":{"details":{"uuid":"this-is-a-uuid","api-endpoints":["this-is-one-of-many-api-endpoints"],"cloud":"prodstack","agent-version":"999.99.99","agent-git-commit":"badf00d0badf00d0badf00d0badf00d0badf00d0","controller-model-version":"999.99.99","mongo-version":"3.5.12","ca-cert":"this-is-a-ca-cert"},"account":{"user":"admin","access":"superuser"}}}
`[1:]
	s.assertShowController(c, "--format", "json", "aws-test", "mark-test-prodstack")
}

func (s *ShowControllerSuite) TestShowControllerReadFromStoreErr(c *tc.C) {
	s.createTestClientStore(c)

	msg := "fail getting controller"
	errStore := jujuclienttesting.NewStubStore()
	errStore.SetErrors(errors.New(msg))
	s.store = errStore
	s.expectedErr = msg

	s.assertShowControllerFailed(c, "test1")
	errStore.CheckCallNames(c, "ControllerByName")
}

func (s *ShowControllerSuite) TestShowControllerNoArgs(c *tc.C) {
	store := s.createTestClientStore(c)
	store.CurrentControllerName = "aws-test"

	s.expectedOutput = `
{"aws-test":{"details":{"uuid":"this-is-the-aws-test-uuid","api-endpoints":["this-is-aws-test-of-many-api-endpoints"],"cloud":"aws","region":"us-east-1","agent-version":"999.99.99","agent-git-commit":"badf00d0badf00d0badf00d0badf00d0badf00d0","controller-model-version":"999.99.99","mongo-version":"3.5.12","ca-cert":"this-is-aws-test-ca-cert"},"controller-machines":{"0":{"instance-id":"id-0","ha-status":"ha-pending"},"1":{"instance-id":"id-1","ha-status":"down, lost connection"},"2":{"instance-id":"id-2","ha-status":"ha-enabled"}},"models":{"controller":{"uuid":"ghi","machine-count":2,"core-count":4}},"current-model":"admin/controller","account":{"user":"admin","access":"superuser"}}}
`[1:]
	s.assertShowController(c, "--format", "json")
}

func (s *ShowControllerSuite) TestShowControllerNoArgsNoCurrent(c *tc.C) {
	store := s.createTestClientStore(c)
	store.CurrentControllerName = ""
	s.expectedErr = regexp.QuoteMeta(`there is no active controller`)
	s.assertShowControllerFailed(c)
}

func (s *ShowControllerSuite) TestShowControllerNotFound(c *tc.C) {
	s.createTestClientStore(c)

	s.expectedErr = `controller whoops not found`
	s.assertShowControllerFailed(c, "whoops")
}

func (s *ShowControllerSuite) TestShowControllerUnrecognizedFlag(c *tc.C) {
	s.expectedErr = `option provided but not defined: -m`
	s.assertShowControllerFailed(c, "-m", "my.world")
}

func (s *ShowControllerSuite) TestShowControllerUnrecognizedOptionFlag(c *tc.C) {
	s.expectedErr = `option provided but not defined: --model`
	s.assertShowControllerFailed(c, "--model", "still.my.world")
}

func (s *ShowControllerSuite) TestShowControllerRefreshesStore(c *tc.C) {
	store := s.createTestClientStore(c)
	_, err := s.runShowController(c, "aws-test")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(store.Controllers["aws-test"].ActiveControllerMachineCount, tc.Equals, 1)
	s.fakeController.machines["ghi"][0].HasVote = true
	_, err = s.runShowController(c, "aws-test")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(store.Controllers["aws-test"].ControllerMachineCount, tc.Equals, 3)
	c.Check(store.Controllers["aws-test"].ActiveControllerMachineCount, tc.Equals, 2)
}

func (s *ShowControllerSuite) TestShowControllerRefreshesStoreModels(c *tc.C) {
	store := s.createTestClientStore(c)
	c.Assert(store.Models["mallards"], tc.DeepEquals, &jujuclient.ControllerModels{
		CurrentModel: "admin/my-model",
		Models: map[string]jujuclient.ModelDetails{
			"model0":   {ModelUUID: "abc", ModelType: model.IAAS},
			"my-model": {ModelUUID: "def", ModelType: model.IAAS},
		},
	})
	_, err := s.runShowController(c, "mallards")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(store.Models["mallards"], tc.DeepEquals, &jujuclient.ControllerModels{
		CurrentModel: "admin/my-model",
		Models: map[string]jujuclient.ModelDetails{
			"admin/controller": {ModelUUID: "abc", ModelType: model.IAAS},
			"admin/my-model":   {ModelUUID: "def", ModelType: model.IAAS},
		},
	})
}

func (s *ShowControllerSuite) TestShowControllerForUserWithLoginAccess(c *tc.C) {
	s.controllersYaml = `controllers:
  mallards:
    uuid: this-is-another-uuid
    api-endpoints: [this-is-another-of-many-api-endpoints, this-is-one-more-of-many-api-endpoints]
    cloud: mallards
    agent-version: 999.99.99
    ca-cert: this-is-another-ca-cert
`
	s.expectedOutput = `
mallards:
  details:
    controller-uuid: this-is-another-uuid
    api-endpoints: [this-is-another-of-many-api-endpoints, this-is-one-more-of-many-api-endpoints]
    cloud: mallards
    agent-version: 999.99.99
    agent-git-commit: badf00d0badf00d0badf00d0badf00d0badf00d0
    controller-model-version: 999.99.99
    ca-cert: this-is-another-ca-cert
  current-model: admin/my-model
  account:
    user: admin
    access: login
`[1:]

	store := s.createTestClientStore(c)
	c.Assert(store.Models["mallards"].Models, tc.HasLen, 2)
	s.setAccess(permission.LoginAccess)
	s.assertShowController(c, "mallards")
}

func (s *ShowControllerSuite) TestShowControllerWithIdentityProvider(c *tc.C) {
	_ = s.createTestClientStore(c)
	ctx, err := s.runShowController(c, "aws-test")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), tc.Not(tc.Contains), "identity-url")

	expURL := "https://api.jujucharms.com/identity"
	s.fakeController.identityURL = expURL
	ctx, err = s.runShowController(c, "aws-test")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), tc.Contains, "identity-url: "+expURL)
}

func (s *ShowControllerSuite) TestShowControllerWithCAFingerprint(c *tc.C) {
	s.controllersYaml = `controllers:
  mallards:
    uuid: this-is-another-uuid
    api-endpoints: [this-is-another-of-many-api-endpoints, this-is-one-more-of-many-api-endpoints]
    cloud: mallards
    agent-version: 999.99.99
    mongo-version: 3.5.12
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
`
	s.createTestClientStore(c)

	s.expectedOutput = `
mallards:
  details:
    controller-uuid: this-is-another-uuid
    api-endpoints: [this-is-another-of-many-api-endpoints, this-is-one-more-of-many-api-endpoints]
    cloud: mallards
    agent-version: 999.99.99
    agent-git-commit: badf00d0badf00d0badf00d0badf00d0badf00d0
    controller-model-version: 999.99.99
    mongo-version: 3.5.12
    ca-fingerprint: 93:D9:8E:B8:99:36:E8:8E:23:D5:95:5E:81:29:80:B2:D2:89:A7:38:20:7B:1B:BD:96:C8:D9:C1:03:88:55:70
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
  models:
    controller:
      model-uuid: abc
      machine-count: 2
      core-count: 4
    my-model:
      model-uuid: def
      machine-count: 2
      core-count: 4
  current-model: admin/my-model
  account:
    user: admin
    access: superuser
    password: hunter2
`[1:]

	s.assertShowController(c, "mallards", "--show-password")
}
func (s *ShowControllerSuite) runShowController(c *tc.C, args ...string) (*cmd.Context, error) {
	return cmdtesting.RunCommand(c, controller.NewShowControllerCommandForTest(
		s.store, s.api, s.modelConfigAPI), args...)
}

func (s *ShowControllerSuite) assertShowControllerFailed(c *tc.C, args ...string) {
	_, err := s.runShowController(c, args...)
	c.Assert(err, tc.ErrorMatches, s.expectedErr)
}

func (s *ShowControllerSuite) assertShowController(c *tc.C, args ...string) {
	context, err := s.runShowController(c, args...)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(context), tc.Equals, s.expectedOutput)
}

func (s *ShowControllerSuite) TestShowControllerPrimary(c *tc.C) {
	_ = s.createTestClientStore(c)
	s.expectedOutput = `
aws-test:
  details:
    controller-uuid: this-is-the-aws-test-uuid
    api-endpoints: [this-is-aws-test-of-many-api-endpoints]
    cloud: aws
    region: us-east-1
    agent-version: 999.99.99
    agent-git-commit: badf00d0badf00d0badf00d0badf00d0badf00d0
    controller-model-version: 999.99.99
    mongo-version: 3.5.12
    ca-cert: this-is-aws-test-ca-cert
  controller-machines:
    "0":
      instance-id: id-0
      ha-status: ha-pending
    "1":
      instance-id: id-1
      ha-status: down, lost connection
    "2":
      instance-id: id-2
      ha-status: ha-enabled
      ha-primary: true
  models:
    controller:
      model-uuid: ghi
      machine-count: 2
      core-count: 4
  current-model: admin/controller
  account:
    user: admin
    access: superuser
`[1:]

	_true := true
	s.fakeController.machines["ghi"][2].HAPrimary = &_true

	s.assertShowController(c, "aws-test")
}

func (s *ShowControllerSuite) TestShowControllerPrimaryModelStatusFail(c *tc.C) {
	_ = s.createTestClientStore(c)
	s.expectedOutput = `
aws-test:
  details:
    controller-uuid: this-is-the-aws-test-uuid
    api-endpoints: [this-is-aws-test-of-many-api-endpoints]
    cloud: aws
    region: us-east-1
    agent-version: 999.99.99
    agent-git-commit: badf00d0badf00d0badf00d0badf00d0badf00d0
    controller-model-version: 999.99.99
    mongo-version: 3.5.12
    ca-cert: this-is-aws-test-ca-cert
  current-model: admin/controller
  account:
    user: admin
    access: superuser
  errors:
  - model status incomplete
`[1:]

	_true := true
	s.fakeController.machines["ghi"][2].HAPrimary = &_true
	s.fakeController.emptyModelStatus = true

	s.assertShowController(c, "aws-test")
}

type fakeController struct {
	controllerName    string
	machines          map[string][]base.Machine
	units             map[string]int
	modelTypes        map[string]model.ModelType
	access            permission.Access
	identityURL       string
	controllerVersion apicontroller.ControllerVersion
	emptyModelStatus  bool
}

func (c *fakeController) GetControllerAccess(ctx context.Context, user string) (permission.Access, error) {
	return c.access, nil
}

func (c *fakeController) ModelStatus(_ context.Context, models ...names.ModelTag) (result []base.ModelStatus, _ error) {
	if c.emptyModelStatus {
		return result, nil
	}
	for _, mtag := range models {
		result = append(result, base.ModelStatus{
			UUID:              mtag.Id(),
			TotalMachineCount: 2,
			CoreCount:         4,
			Machines:          c.machines[mtag.Id()],
			UnitCount:         c.units[mtag.Id()],
			ModelType:         c.modelTypes[mtag.Id()],
		})
	}
	return result, nil
}

func (c *fakeController) MongoVersion(ctx context.Context) (string, error) {
	return "3.5.12", nil
}

func (c *fakeController) AllModels(ctx context.Context) (result []base.UserModel, _ error) {
	models := map[string][]base.UserModel{
		"aws-test": {
			{Name: "controller", UUID: "ghi", Owner: "admin", Type: model.IAAS},
		},
		"mallards": {
			{Name: "controller", UUID: "abc", Owner: "admin", Type: model.IAAS},
			{Name: "my-model", UUID: "def", Owner: "admin", Type: model.IAAS},
		},
		"k8s-controller": {
			{Name: "controller", UUID: "xyz", Owner: "admin", Type: model.CAAS},
			{Name: "my-k8s-model", UUID: "def", Owner: "admin", Type: model.CAAS},
		},
	}
	all, exists := models[c.controllerName]
	if !exists {
		return result, nil
	}
	return all, nil
}

func (c *fakeController) IdentityProviderURL(ctx context.Context) (string, error) {
	return c.identityURL, nil
}

func (c *fakeController) ControllerVersion(ctx context.Context) (apicontroller.ControllerVersion, error) {
	return c.controllerVersion, nil
}

func (*fakeController) Close() error {
	return nil
}

type fakeModelConfig struct{}

func (*fakeModelConfig) ModelGet(ctx context.Context) (map[string]interface{}, error) {
	return map[string]interface{}{"agent-version": "999.99.99"}, nil
}

func (*fakeModelConfig) Close() error {
	return nil
}
