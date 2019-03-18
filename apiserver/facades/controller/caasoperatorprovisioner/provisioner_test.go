// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperatorprovisioner_test

import (
	"fmt"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facades/controller/caasoperatorprovisioner"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/version"
)

var _ = gc.Suite(&CAASProvisionerSuite{})

type CAASProvisionerSuite struct {
	coretesting.BaseSuite

	resources          *common.Resources
	authorizer         *apiservertesting.FakeAuthorizer
	api                *caasoperatorprovisioner.API
	st                 *mockState
	storagePoolManager *mockStoragePoolManager
}

func (s *CAASProvisionerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.resources = common.NewResources()
	s.AddCleanup(func(_ *gc.C) { s.resources.StopAll() })

	s.authorizer = &apiservertesting.FakeAuthorizer{
		Tag:        names.NewMachineTag("0"),
		Controller: true,
	}

	s.st = newMockState()
	s.storagePoolManager = &mockStoragePoolManager{}
	api, err := caasoperatorprovisioner.NewCAASOperatorProvisionerAPI(s.resources, s.authorizer, s.st, s.storagePoolManager)
	c.Assert(err, jc.ErrorIsNil)
	s.api = api
}

func (s *CAASProvisionerSuite) TestPermission(c *gc.C) {
	s.authorizer = &apiservertesting.FakeAuthorizer{
		Tag: names.NewMachineTag("0"),
	}
	_, err := caasoperatorprovisioner.NewCAASOperatorProvisionerAPI(s.resources, s.authorizer, s.st, s.storagePoolManager)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *CAASProvisionerSuite) TestWatchApplications(c *gc.C) {
	applicationNames := []string{"db2", "hadoop"}
	s.st.applicationWatcher.changes <- applicationNames
	result, err := s.api.WatchApplications()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)
	c.Assert(result.StringsWatcherId, gc.Equals, "1")
	c.Assert(result.Changes, jc.DeepEquals, applicationNames)

	resource := s.resources.Get("1")
	c.Assert(resource, gc.NotNil)
	c.Assert(resource, gc.Implements, new(state.StringsWatcher))
}

func (s *CAASProvisionerSuite) TestSetPasswords(c *gc.C) {
	s.st.app = &mockApplication{
		tag: names.NewApplicationTag("app"),
	}

	args := params.EntityPasswords{
		Changes: []params.EntityPassword{
			{Tag: "application-app", Password: "xxx-12345678901234567890"},
			{Tag: "application-another", Password: "yyy-12345678901234567890"},
			{Tag: "machine-0", Password: "zzz-12345678901234567890"},
		},
	}
	results, err := s.api.SetPasswords(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{nil},
			{&params.Error{Message: "entity application-another not found", Code: "not found"}},
			{&params.Error{Message: "permission denied", Code: "unauthorized access"}},
		},
	})
	c.Assert(s.st.app.password, gc.Equals, "xxx-12345678901234567890")
}

func (s *CAASProvisionerSuite) TestLife(c *gc.C) {
	s.st.app = &mockApplication{
		tag: names.NewApplicationTag("app"),
	}
	results, err := s.api.Life(params.Entities{
		Entities: []params.Entity{
			{Tag: "application-app"},
			{Tag: "machine-0"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.LifeResults{
		Results: []params.LifeResult{{
			Life: params.Alive,
		}, {
			Error: &params.Error{
				Code:    "unauthorized access",
				Message: "permission denied",
			},
		}},
	})
}

func (s *CAASProvisionerSuite) TestOperatorProvisioningInfoDefault(c *gc.C) {
	result, err := s.api.OperatorProvisioningInfo()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.OperatorProvisioningInfo{
		ImagePath:    fmt.Sprintf("jujusolutions/jujud-operator:%s", version.Current.String()),
		Version:      version.Current,
		APIAddresses: []string{"10.0.0.1:1"},
		Tags: map[string]string{
			"juju-model-uuid":      coretesting.ModelTag.Id(),
			"juju-controller-uuid": coretesting.ControllerTag.Id()},
		CharmStorage: params.KubernetesFilesystemParams{
			StorageName: "charm",
			Size:        uint64(1024),
			Provider:    "kubernetes",
			Attributes: map[string]interface{}{
				"storage-class": "k8s-storage",
				"foo":           "bar",
			},
			Tags: map[string]string{
				"juju-model-uuid":      coretesting.ModelTag.Id(),
				"juju-controller-uuid": coretesting.ControllerTag.Id()},
		},
	})
}

func (s *CAASProvisionerSuite) TestOperatorProvisioningInfo(c *gc.C) {
	s.st.operatorRepo = "somerepo"
	result, err := s.api.OperatorProvisioningInfo()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.OperatorProvisioningInfo{
		ImagePath:    s.st.operatorRepo + "/jujud-operator:" + version.Current.String(),
		Version:      version.Current,
		APIAddresses: []string{"10.0.0.1:1"},
		Tags: map[string]string{
			"juju-model-uuid":      coretesting.ModelTag.Id(),
			"juju-controller-uuid": coretesting.ControllerTag.Id()},
		CharmStorage: params.KubernetesFilesystemParams{
			StorageName: "charm",
			Size:        uint64(1024),
			Provider:    "kubernetes",
			Attributes: map[string]interface{}{
				"storage-class": "k8s-storage",
				"foo":           "bar",
			},
			Tags: map[string]string{
				"juju-model-uuid":      coretesting.ModelTag.Id(),
				"juju-controller-uuid": coretesting.ControllerTag.Id()},
		},
	})
}

func (s *CAASProvisionerSuite) TestOperatorProvisioningInfoNoStoragePool(c *gc.C) {
	s.storagePoolManager.SetErrors(errors.NotFoundf("pool"))
	s.st.operatorRepo = "somerepo"
	result, err := s.api.OperatorProvisioningInfo()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.OperatorProvisioningInfo{
		ImagePath:    s.st.operatorRepo + "/jujud-operator:" + version.Current.String(),
		Version:      version.Current,
		APIAddresses: []string{"10.0.0.1:1"},
		Tags: map[string]string{
			"juju-model-uuid":      coretesting.ModelTag.Id(),
			"juju-controller-uuid": coretesting.ControllerTag.Id()},
		CharmStorage: params.KubernetesFilesystemParams{
			StorageName: "charm",
			Size:        uint64(1024),
			Provider:    "kubernetes",
			Attributes: map[string]interface{}{
				"storage-class": "k8s-storage",
			},
			Tags: map[string]string{
				"juju-model-uuid":      coretesting.ModelTag.Id(),
				"juju-controller-uuid": coretesting.ControllerTag.Id()},
		},
	})
}

func (s *CAASProvisionerSuite) TestAddresses(c *gc.C) {
	_, err := s.api.APIAddresses()
	c.Assert(err, jc.ErrorIsNil)
	s.st.CheckCallNames(c, "APIHostPortsForAgents")
}
