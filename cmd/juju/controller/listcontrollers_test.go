// Copyright 2015,2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/cmd/juju/controller"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
)

type ListControllersSuite struct {
	baseControllerSuite
	api func(string) controller.ControllerAccessAPI
}

func TestListControllersSuite(t *testing.T) {
	tc.Run(t, &ListControllersSuite{})
}

func (s *ListControllersSuite) TestListControllersEmptyStore(c *tc.C) {
	s.store = jujuclient.NewMemStore()
	_, err := s.runListControllers(c)
	c.Check(errors.Cause(err), tc.Equals, modelcmd.ErrNoControllersDefined)
}

func (s *ListControllersSuite) TestListControllers(c *tc.C) {
	store := s.createTestClientStore(c)
	delete(store.Accounts, "aws-test")
	originallyInStore := &jujuclient.ControllerModels{
		CurrentModel: "prod/my-model",
		Models: map[string]jujuclient.ModelDetails{
			"model0":   {ModelUUID: "abc", ModelType: model.IAAS},
			"my-model": {ModelUUID: "def", ModelType: model.IAAS},
		},
	}
	c.Assert(store.Models["mallards"], tc.DeepEquals, originallyInStore)

	s.expectedOutput = `
Use --refresh option with this command to see the latest information.

Controller           Model              User   Access     Cloud/Region        Models  Nodes  HA  Version
aws-test             prod/controller    -      -          aws/us-east-1            1      5   -  2.0.1
k8s-controller       prod/my-k8s-model  admin  superuser  microk8s/localhost       2      3   -  6.6.6
mallards*            prod/my-model      admin  superuser  mallards/mallards1       2      -   -  (unknown)
mark-test-prodstack  -                  admin  (unknown)  prodstack                -      -   -  (unknown)
`[1:]

	s.assertListControllers(c)
	// Check store was not updated.
	c.Assert(store.Models["mallards"], tc.DeepEquals, originallyInStore)
}

func (s *ListControllersSuite) TestListControllersRefresh(c *tc.C) {
	store := s.createTestClientStore(c)
	originallyInStore := &jujuclient.ControllerModels{
		CurrentModel: "prod/my-model",
		Models: map[string]jujuclient.ModelDetails{
			"model0":   {ModelUUID: "abc", ModelType: model.IAAS},
			"my-model": {ModelUUID: "def", ModelType: model.IAAS},
		},
	}
	c.Assert(store.Models["mallards"], tc.DeepEquals, originallyInStore)
	s.api = func(controllerName string) controller.ControllerAccessAPI {
		fakeController := &fakeController{controllerName: controllerName}
		return fakeController
	}
	s.expectedOutput = `
Controller           Model              User   Access     Cloud/Region        Models  Nodes  HA  Version
aws-test             prod/controller    admin  (unknown)  aws/us-east-1            1      2   -  2.0.1
k8s-controller       prod/my-k8s-model  admin  superuser  microk8s/localhost       2      4   -  6.6.6
mallards*            prod/my-model      admin  superuser  mallards/mallards1       2      4   -  (unknown)
mark-test-prodstack  -                  admin  (unknown)  prodstack                -      -   -  (unknown)
`[1:]
	s.assertListControllers(c, "--refresh")
	// Check store was updated.
	c.Assert(store.Models["mallards"], tc.DeepEquals, &jujuclient.ControllerModels{
		CurrentModel: "prod/my-model",
		Models: map[string]jujuclient.ModelDetails{
			"prod/controller": {ModelUUID: "abc", ModelType: model.IAAS},
			"prod/my-model":   {ModelUUID: "def", ModelType: model.IAAS},
		},
	})
}

func (s *ListControllersSuite) setupAPIForControllerMachines() {
	s.api = func(controllerName string) controller.ControllerAccessAPI {
		fakeController := &fakeController{controllerName: controllerName}
		switch controllerName {
		case "aws-test":
			fakeController.machines = map[string][]base.Machine{
				"ghi": {
					{Id: "1", Status: "active"},
					{Id: "2", Status: "down"},
					{Id: "3", Status: "active"},
				},
			}
		case "mallards":
			fakeController.machines = map[string][]base.Machine{
				"abc": {
					{Id: "1", Status: "active"},
				},
				"def": {
					{Id: "2", Status: "active"},
				},
			}
		case "k8s-controller":
			fakeController.machines = map[string][]base.Machine{
				"xyz": {
					{Id: "1", Status: "active"},
				},
				"def": {
					{Id: "2", Status: "active"},
				},
			}
		}
		return fakeController
	}
}

func (s *ListControllersSuite) TestListControllersKnownHAStatus(c *tc.C) {
	s.createTestClientStore(c)
	s.setupAPIForControllerMachines()
	s.expectedOutput = `
Controller           Model              User   Access     Cloud/Region        Models  Nodes    HA  Version
aws-test             prod/controller    admin  (unknown)  aws/us-east-1            1      2   2/3  2.0.1
k8s-controller       prod/my-k8s-model  admin  superuser  microk8s/localhost       2      4     -  6.6.6
mallards*            prod/my-model      admin  superuser  mallards/mallards1       2      4  none  (unknown)
mark-test-prodstack  -                  admin  (unknown)  prodstack                -      -     -  (unknown)
`[1:]
	s.assertListControllers(c, "--refresh")
}

func (s *ListControllersSuite) TestListControllersYaml(c *tc.C) {
	s.expectedOutput = `
controllers:
  aws-test:
    current-model: prod/controller
    user: admin
    recent-server: this-is-aws-test-of-many-api-endpoints
    controller-uuid: this-is-the-aws-test-uuid
    api-endpoints: [this-is-aws-test-of-many-api-endpoints]
    ca-cert: this-is-aws-test-ca-cert
    cloud: aws
    region: us-east-1
    agent-version: 2.0.1
    model-count: 1
    machine-count: 2
    controller-machines:
      active: 2
      total: 3
  k8s-controller:
    current-model: prod/my-k8s-model
    user: admin
    access: superuser
    recent-server: this-is-one-of-many-k8s-api-endpoints
    controller-uuid: this-is-a-k8s-uuid
    api-endpoints: [this-is-one-of-many-k8s-api-endpoints]
    ca-cert: this-is-a-k8s-ca-cert
    cloud: microk8s
    region: localhost
    agent-version: 6.6.6
    model-count: 2
    node-count: 4
    controller-nodes:
      active: 1
      total: 1
  mallards:
    current-model: prod/my-model
    user: admin
    access: superuser
    recent-server: this-is-another-of-many-api-endpoints
    controller-uuid: deadbeef-1bad-500d-9000-4b1d0d06f00d
    api-endpoints: [this-is-another-of-many-api-endpoints, this-is-one-more-of-many-api-endpoints]
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
    cloud: mallards
    region: mallards1
    model-count: 2
    machine-count: 4
    controller-machines:
      active: 1
      total: 1
  mark-test-prodstack:
    user: admin
    recent-server: this-is-one-of-many-api-endpoints
    controller-uuid: this-is-a-uuid
    api-endpoints: [this-is-one-of-many-api-endpoints]
    ca-cert: this-is-a-ca-cert
    cloud: prodstack
current-controller: mallards
`[1:]

	s.createTestClientStore(c)
	s.setupAPIForControllerMachines()
	s.assertListControllers(c, "--format", "yaml", "--refresh")
}

func intPtr(i int) *int {
	return &i
}

const testCert = `-----BEGIN CERTIFICATE-----
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
-----END CERTIFICATE-----`

func (s *ListControllersSuite) TestListControllersJson(c *tc.C) {
	s.expectedOutput = ""
	s.createTestClientStore(c)
	jsonOut := s.assertListControllers(c, "--format", "json")
	var result controller.ControllerSet
	err := json.Unmarshal([]byte(jsonOut), &result)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, controller.ControllerSet{
		Controllers: map[string]controller.ControllerItem{
			"aws-test": {
				ControllerUUID: "this-is-the-aws-test-uuid",
				ModelName:      "prod/controller",
				User:           "admin",
				Server:         "this-is-aws-test-of-many-api-endpoints",
				APIEndpoints:   []string{"this-is-aws-test-of-many-api-endpoints"},
				CACert:         "this-is-aws-test-ca-cert",
				Cloud:          "aws",
				CloudRegion:    "us-east-1",
				AgentVersion:   "2.0.1",
				ModelCount:     intPtr(1),
				MachineCount:   intPtr(5),
			},
			"mallards": {
				ControllerUUID: "deadbeef-1bad-500d-9000-4b1d0d06f00d",
				ModelName:      "prod/my-model",
				User:           "admin",
				Access:         "superuser",
				Server:         "this-is-another-of-many-api-endpoints",
				APIEndpoints:   []string{"this-is-another-of-many-api-endpoints", "this-is-one-more-of-many-api-endpoints"},
				CACert:         testCert,
				Cloud:          "mallards",
				CloudRegion:    "mallards1",
				ModelCount:     intPtr(2),
			},
			"k8s-controller": {
				ControllerUUID: "this-is-a-k8s-uuid",
				ModelName:      "prod/my-k8s-model",
				User:           "admin",
				Access:         "superuser",
				Server:         "this-is-one-of-many-k8s-api-endpoints",
				APIEndpoints:   []string{"this-is-one-of-many-k8s-api-endpoints"},
				CACert:         "this-is-a-k8s-ca-cert",
				Cloud:          "microk8s",
				CloudRegion:    "localhost",
				AgentVersion:   "6.6.6",
				NodeCount:      intPtr(3),
				ModelCount:     intPtr(2),
			},
			"mark-test-prodstack": {
				ControllerUUID: "this-is-a-uuid",
				User:           "admin",
				Server:         "this-is-one-of-many-api-endpoints",
				APIEndpoints:   []string{"this-is-one-of-many-api-endpoints"},
				CACert:         "this-is-a-ca-cert",
				Cloud:          "prodstack",
			},
		},
		CurrentController: "mallards",
	})
}

func (s *ListControllersSuite) TestListControllersReadFromStoreErr(c *tc.C) {
	msg := "fail getting all controllers"
	errStore := jujuclienttesting.NewStubStore()
	errStore.SetErrors(errors.New(msg))
	s.store = errStore
	s.expectedErr = fmt.Sprintf("failed to list controllers: %v", msg)
	s.assertListControllersFailed(c)
	errStore.CheckCallNames(c, "AllControllers")
}

func (s *ListControllersSuite) TestListControllersUnrecognizedArg(c *tc.C) {
	s.createTestClientStore(c)
	s.expectedErr = `unrecognized args: \["whoops"\]`
	s.assertListControllersFailed(c, "whoops")
}

func (s *ListControllersSuite) TestListControllersUnrecognizedFlag(c *tc.C) {
	s.createTestClientStore(c)
	s.expectedErr = `option provided but not defined: -m`
	s.assertListControllersFailed(c, "-m", "my.world")
}

func (s *ListControllersSuite) TestListControllersUnrecognizedOptionFlag(c *tc.C) {
	s.createTestClientStore(c)
	s.expectedErr = `option provided but not defined: --model`
	s.assertListControllersFailed(c, "--model", "still.my.world")
}

func (s *ListControllersSuite) TestListControllersNoControllers(c *tc.C) {
	store := s.createTestClientStore(c)
	store.Controllers = map[string]jujuclient.ControllerDetails{}
	s.expectedErr = `No controllers registered.

Please either create a new controller using "juju bootstrap" or connect to
another controller that you have been given access to using "juju register".
`
	s.assertListControllersFailed(c)
}

func (s *ListControllersSuite) TestListControllersManagedFlag(c *tc.C) {
	s.createTestClientStore(c)
	_, err := s.runListControllers(c, "--managed")
	c.Assert(err, tc.ErrorIs, cmd.ErrCommandMissing)
}

func (s *ListControllersSuite) runListControllers(c *tc.C, args ...string) (*cmd.Context, error) {
	return cmdtesting.RunCommand(c, controller.NewListControllersCommandForTest(s.store, s.api), args...)
}

func (s *ListControllersSuite) assertListControllersFailed(c *tc.C, args ...string) {
	_, err := s.runListControllers(c, args...)
	c.Assert(err, tc.ErrorMatches, s.expectedErr)
}

func (s *ListControllersSuite) assertListControllers(c *tc.C, args ...string) string {
	context, err := s.runListControllers(c, args...)
	c.Assert(err, tc.ErrorIsNil)
	output := cmdtesting.Stdout(context)
	if s.expectedOutput != "" {
		c.Assert(output, tc.Equals, s.expectedOutput)
	}
	return output
}
